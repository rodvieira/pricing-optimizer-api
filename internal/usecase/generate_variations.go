package usecase

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

const (
	minStrategies = 1
	maxStrategies = 3
)

// GenerateVariationsInput is the POST /v1/generate request the
// GenerateVariations use case fans out into one LLMProvider.StreamStructured
// call per strategy. Defaulting an omitted Strategies list to all three
// strategies, per the openapi.yaml contract, is the HTTP layer's job, not
// this use case's: Execute always requires an already-resolved 1-3 list.
// There is no separate source-URL field: the Generation's SourceURL is
// SiteProfile.URL, since that's the only URL the GenerateRequest contract
// carries (via the SiteProfile the client got from a prior /v1/analyze call).
type GenerateVariationsInput struct {
	SiteProfile domain.SiteProfile
	Strategies  []domain.PricingStrategy
	Currency    string
}

// Validate checks the invariants this use case needs before spending any LLM
// calls: 1-3 known strategies, no duplicates.
func (in GenerateVariationsInput) Validate() error {
	if in.SiteProfile.URL == "" {
		return fmt.Errorf("%w: site profile url is required", domain.ErrInvalidInput)
	}
	if len(in.Strategies) < minStrategies || len(in.Strategies) > maxStrategies {
		return fmt.Errorf("%w: must request %d-%d strategies, got %d",
			domain.ErrInvalidInput, minStrategies, maxStrategies, len(in.Strategies))
	}
	seen := make(map[domain.PricingStrategy]struct{}, len(in.Strategies))
	for _, s := range in.Strategies {
		if !s.Valid() {
			return fmt.Errorf("%w: invalid strategy %q", domain.ErrInvalidInput, s)
		}
		if _, dup := seen[s]; dup {
			return fmt.Errorf("%w: duplicate strategy %q", domain.ErrInvalidInput, s)
		}
		seen[s] = struct{}{}
	}
	return nil
}

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
// exactly one terminal event: GenerationEventDone (all strategies succeeded)
// or GenerationEventError (at least one failed, or the completed Generation
// could not be saved).
//
// Canceling ctx stops event delivery: the returned channel closes without a
// terminal event once every fan-out goroutine has observed the cancellation,
// same as an early-disconnecting SSE client would expect.
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

	if !send(domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}) {
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

	if firstErr != nil {
		gen.Status = domain.GenerationStatusFailed
	} else {
		gen.Status = domain.GenerationStatusCompleted
		gen.Variations = variations
	}

	if err := uc.repo.Save(ctx, gen); err != nil {
		send(domain.GenerationEvent{
			Type: domain.GenerationEventError,
			Err:  fmt.Errorf("save completed generation: %w", err),
		})
		return
	}

	if firstErr != nil {
		send(domain.GenerationEvent{Type: domain.GenerationEventError, Err: firstErr, Generation: &gen})
		return
	}
	send(domain.GenerationEvent{Type: domain.GenerationEventDone, Generation: &gen})
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
