package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// GenerateVariationsInput is domain.GenerateVariationsInput. Aliased (not
// redefined) so every existing caller/test can keep writing
// usecase.GenerateVariationsInput, while the real type lives in domain: the
// POST /v1/generate HTTP handler needs to construct one and reference it in
// its own consumer-defined interface, and adapters may never import usecase.
type GenerateVariationsInput = domain.GenerateVariationsInput

// GenerateVariations orchestrates streaming generation across the requested
// pricing strategies: it creates and persists a Generation, fans out one
// LLMProvider.StreamStructured call per strategy in parallel, and multiplexes
// their per-strategy StreamChunk channels into a single ordered channel of
// GenerationEvent for the HTTP layer to translate into SSE frames.
type GenerateVariations struct {
	provider domain.LLMProvider
	repo     domain.GenerationRepo
}

// NewGenerateVariations creates the use case bound to provider and repo. No
// handler is aware of which concrete LLMProvider or GenerationRepo these are.
func NewGenerateVariations(provider domain.LLMProvider, repo domain.GenerationRepo) *GenerateVariations {
	return &GenerateVariations{provider: provider, repo: repo}
}

// Execute validates in, creates and saves a Generation in the streaming
// state, then returns a channel of GenerationEvent that emits progress as
// each requested strategy's variation is generated. The channel closes after
// at most one terminal event: GenerationEventDone (every strategy
// succeeded) or GenerationEventError (at least one strategy failed, ctx was
// canceled before every strategy finished, or the terminal Generation could
// not be saved).
//
// Canceling ctx stops event delivery to the caller (the returned channel
// closes, typically without a terminal event reaching an already-gone
// consumer), but the Generation is still saved as failed through a context
// that outlives ctx — an early-disconnecting SSE client must not leave the
// record stuck in "streaming" or, worse, incorrectly marked completed with
// partial variations.
func (uc *GenerateVariations) Execute(
	ctx context.Context, in GenerateVariationsInput,
) (<-chan domain.GenerationEvent, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("generate variations: %w", err)
	}

	gen := domain.Generation{
		ID:          uuid.NewString(),
		SourceURL:   in.SiteProfile.URL,
		SiteProfile: in.SiteProfile,
		Status:      domain.GenerationStatusStreaming,
		CreatedAt:   time.Now(),
	}
	if err := uc.repo.Save(ctx, gen); err != nil {
		return nil, fmt.Errorf("generate variations: save generation: %w", err)
	}

	out := make(chan domain.GenerationEvent)
	go uc.run(ctx, in, gen, out)
	return out, nil
}

// run fans out one StreamStructured call per strategy, relays every chunk as
// a GenerationEvent on out, then saves and emits the terminal Generation
// state. It always closes out before returning.
func (uc *GenerateVariations) run(
	ctx context.Context, in GenerateVariationsInput, gen domain.Generation, out chan<- domain.GenerationEvent,
) {
	defer close(out)

	send := func(ev domain.GenerationEvent) bool {
		select {
		case out <- ev:
			return true
		case <-ctx.Done():
			return false
		}
	}

	started := gen
	if !send(domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &started}) {
		return
	}

	variations := make([]domain.Variation, len(in.Strategies))

	var (
		mu       sync.Mutex
		firstErr error
	)
	recordErr := func(err error) {
		mu.Lock()
		defer mu.Unlock()
		if firstErr == nil {
			firstErr = err
		}
	}
	setVariation := func(i int, v domain.Variation) {
		mu.Lock()
		defer mu.Unlock()
		variations[i] = v
	}

	var wg sync.WaitGroup
	for i, strategy := range in.Strategies {
		wg.Add(1)
		go func(i int, strategy domain.PricingStrategy) {
			defer wg.Done()
			uc.streamStrategy(ctx, in, i, strategy, send, recordErr, setVariation)
		}(i, strategy)
	}
	wg.Wait()

	// A canceled ctx (the client disconnected) must count as a failure even
	// though no strategy goroutine necessarily recorded one: the adapters'
	// StreamStructured implementations stop silently on ctx.Done() rather
	// than emitting a StreamChunkError, so firstErr alone cannot be trusted
	// to reflect "every strategy actually completed."
	switch {
	case ctx.Err() != nil:
		if firstErr == nil {
			firstErr = ctx.Err()
		}
		gen.Status = domain.GenerationStatusFailed
	case firstErr != nil:
		gen.Status = domain.GenerationStatusFailed
	default:
		gen.Status = domain.GenerationStatusCompleted
		gen.Variations = variations
	}

	// Persist the terminal state through a context that outlives ctx: a
	// disconnected client must not prevent the correct failed/completed
	// status from being saved, or the record is left stuck in "streaming"
	// forever (or, worse, silently marked completed with partial data).
	saveCtx := context.WithoutCancel(ctx)
	if err := uc.repo.Save(saveCtx, gen); err != nil {
		send(domain.GenerationEvent{
			Type: domain.GenerationEventError,
			Err:  fmt.Errorf("save %s generation: %w", gen.Status, err),
		})
		return
	}

	if firstErr != nil {
		failed := gen
		send(domain.GenerationEvent{Type: domain.GenerationEventError, Err: firstErr, Generation: &failed})
		return
	}
	done := gen
	send(domain.GenerationEvent{Type: domain.GenerationEventDone, Generation: &done})
}

// streamStrategy runs one strategy's StreamStructured call and relays its
// chunks as GenerationEvents via send, stopping early if send reports the
// consumer is gone (ctx canceled).
func (uc *GenerateVariations) streamStrategy(
	ctx context.Context,
	in GenerateVariationsInput,
	i int,
	strategy domain.PricingStrategy,
	send func(domain.GenerationEvent) bool,
	recordErr func(error),
	setVariation func(int, domain.Variation),
) {
	if !send(domain.GenerationEvent{Type: domain.GenerationEventVariationStarted, Strategy: strategy}) {
		return
	}

	chunks, err := uc.provider.StreamStructured(ctx, domain.GenerationInput{
		SiteProfile: in.SiteProfile,
		Strategy:    strategy,
		Currency:    in.Currency,
	})
	if err != nil {
		recordErr(fmt.Errorf("stream %s variation: %w", strategy, err))
		send(domain.GenerationEvent{Type: domain.GenerationEventError, Strategy: strategy, Err: err})
		return
	}

	for chunk := range chunks {
		var ev domain.GenerationEvent
		switch chunk.Type {
		case domain.StreamChunkToken:
			ev = domain.GenerationEvent{Type: domain.GenerationEventToken, Strategy: strategy, Delta: chunk.Delta}
		case domain.StreamChunkVariationCompleted:
			setVariation(i, *chunk.Variation)
			ev = domain.GenerationEvent{
				Type: domain.GenerationEventVariationCompleted, Strategy: strategy, Variation: chunk.Variation,
			}
		case domain.StreamChunkError:
			recordErr(fmt.Errorf("stream %s variation: %w", strategy, chunk.Err))
			ev = domain.GenerationEvent{Type: domain.GenerationEventError, Strategy: strategy, Err: chunk.Err}
		default:
			continue
		}
		if !send(ev) {
			return
		}
	}
}
