package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	"github.com/rodvieira/pricing-optimizer-api/internal/usecase"
	mockdomain "github.com/rodvieira/pricing-optimizer-api/test/mocks/domain"
)

func fixtureSiteProfile() domain.SiteProfile {
	return domain.SiteProfile{
		URL:              "https://example.com",
		Title:            "Acme Analytics",
		ValueProposition: "Real-time analytics for indie SaaS founders",
		Industry:         "developer-tools",
		Audience: domain.Audience{
			Segment:        "SaaS founders",
			Sophistication: domain.SophisticationMedium,
			PricePosition:  domain.PricePositionMidMarket,
		},
	}
}

func fixtureVariation(strategy domain.PricingStrategy) *domain.Variation {
	return &domain.Variation{
		ID:       "fixture-id",
		Strategy: strategy,
		Headline: "Simple, transparent pricing",
		Tiers: []domain.PricingTier{
			{
				Name:     "Pro",
				Price:    domain.Price{AmountMinorUnits: 2900, Currency: "USD", Interval: domain.IntervalMonthly},
				Features: []string{"Feature A"},
			},
		},
	}
}

func validGenerateVariationsInput() usecase.GenerateVariationsInput {
	return usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyFreemium},
		Currency:    "USD",
	}
}

// chunkChannel returns a closed, pre-filled channel of chunks, mimicking
// what a real LLMProvider.StreamStructured call returns.
func chunkChannel(chunks ...domain.StreamChunk) <-chan domain.StreamChunk {
	ch := make(chan domain.StreamChunk, len(chunks))
	for _, c := range chunks {
		ch <- c
	}
	close(ch)
	return ch
}

func successChunks(strategy domain.PricingStrategy) []domain.StreamChunk {
	v := fixtureVariation(strategy)
	return []domain.StreamChunk{
		{Type: domain.StreamChunkToken, Delta: "Simple"},
		{Type: domain.StreamChunkToken, Delta: ", transparent pricing"},
		{Type: domain.StreamChunkVariationCompleted, Variation: v},
	}
}

// collect drains ch with a timeout, failing the test if it doesn't close in
// time (guards against a goroutine leak or deadlock in the use case).
func collect(t *testing.T, ch <-chan domain.GenerationEvent) []domain.GenerationEvent {
	t.Helper()

	var events []domain.GenerationEvent
	deadline := time.After(2 * time.Second)
	for {
		select {
		case ev, ok := <-ch:
			if !ok {
				return events
			}
			events = append(events, ev)
		case <-deadline:
			t.Fatal("channel did not close before the test deadline; possible goroutine leak")
			return nil
		}
	}
}

func eventTypes(events []domain.GenerationEvent) []domain.GenerationEventType {
	types := make([]domain.GenerationEventType, len(events))
	for i, ev := range events {
		types[i] = ev.Type
	}
	return types
}

func TestGenerateVariationsInput_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   usecase.GenerateVariationsInput
	}{
		{
			name: "missing site profile url is rejected",
			in: usecase.GenerateVariationsInput{
				SiteProfile: domain.SiteProfile{},
				Strategies:  []domain.PricingStrategy{domain.StrategyAnchor},
				Currency:    "USD",
			},
		},
		{
			name: "no strategies is rejected",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies:  nil,
				Currency:    "USD",
			},
		},
		{
			name: "more than three strategies is rejected",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies: []domain.PricingStrategy{
					domain.StrategyAnchor, domain.StrategyFreemium, domain.StrategyValueBased, domain.StrategyAnchor,
				},
				Currency: "USD",
			},
		},
		{
			name: "duplicate strategy is rejected",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies:  []domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyAnchor},
				Currency:    "USD",
			},
		},
		{
			name: "unknown strategy is rejected",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies:  []domain.PricingStrategy{"bogus"},
				Currency:    "USD",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			err := tt.in.Validate()

			require.ErrorIs(t, err, domain.ErrInvalidInput)
		})
	}
}

func TestGenerateVariations_Execute_RejectsInvalidInputBeforeAnyCall(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	uc := usecase.NewGenerateVariations(provider, repo)
	in := validGenerateVariationsInput()
	in.Strategies = nil

	_, err := uc.Execute(context.Background(), in)

	require.ErrorIs(t, err, domain.ErrInvalidInput)
}

func TestGenerateVariations_Execute_InitialSaveFailureShortCircuits(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	saveErr := errors.New("db down")
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(saveErr)

	uc := usecase.NewGenerateVariations(provider, repo)
	_, err := uc.Execute(context.Background(), validGenerateVariationsInput())

	require.ErrorIs(t, err, saveErr)
}

func TestGenerateVariations_Execute_SingleStrategySuccess(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	in := usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor},
		Currency:    "USD",
	}

	var savedStatuses []domain.GenerationStatus
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(_ context.Context, g domain.Generation) error {
			savedStatuses = append(savedStatuses, g.Status)
			return nil
		})
	provider.EXPECT().
		StreamStructured(gomock.Any(), gomock.Any()).
		Return(chunkChannel(successChunks(domain.StrategyAnchor)...), nil)

	uc := usecase.NewGenerateVariations(provider, repo)
	ch, err := uc.Execute(context.Background(), in)
	require.NoError(t, err)

	events := collect(t, ch)

	assert.Equal(t, []domain.GenerationEventType{
		domain.GenerationEventStarted,
		domain.GenerationEventVariationStarted,
		domain.GenerationEventToken,
		domain.GenerationEventToken,
		domain.GenerationEventVariationCompleted,
		domain.GenerationEventDone,
	}, eventTypes(events))

	done := events[len(events)-1]
	require.NotNil(t, done.Generation)
	assert.Equal(t, domain.GenerationStatusCompleted, done.Generation.Status)
	require.Len(t, done.Generation.Variations, 1)
	assert.Equal(t, domain.StrategyAnchor, done.Generation.Variations[0].Strategy)
	assert.Equal(t, in.SiteProfile.URL, done.Generation.SourceURL)

	assert.Equal(t, []domain.GenerationStatus{domain.GenerationStatusStreaming, domain.GenerationStatusCompleted}, savedStatuses)
}

func TestGenerateVariations_Execute_MultipleStrategiesPreserveOrder(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	in := usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyFreemium, domain.StrategyValueBased},
		Currency:    "USD",
	}

	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	provider.EXPECT().
		StreamStructured(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, genIn domain.GenerationInput) (<-chan domain.StreamChunk, error) {
			return chunkChannel(successChunks(genIn.Strategy)...), nil
		}).
		Times(3)

	uc := usecase.NewGenerateVariations(provider, repo)
	ch, err := uc.Execute(context.Background(), in)
	require.NoError(t, err)

	events := collect(t, ch)
	done := events[len(events)-1]
	require.Equal(t, domain.GenerationEventDone, done.Type)
	require.NotNil(t, done.Generation)
	require.Len(t, done.Generation.Variations, 3)

	// Variations must line up with the requested strategy order (index i),
	// not with whichever goroutine happened to finish first.
	for i, strategy := range in.Strategies {
		assert.Equal(t, strategy, done.Generation.Variations[i].Strategy)
	}
}

func TestGenerateVariations_Execute_ProviderCallFailureMarksGenerationFailed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	var savedStatuses []domain.GenerationStatus
	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Times(2).DoAndReturn(
		func(_ context.Context, g domain.Generation) error {
			savedStatuses = append(savedStatuses, g.Status)
			return nil
		})
	provider.EXPECT().
		StreamStructured(gomock.Any(), gomock.Any()).
		Return(nil, domain.ErrProviderUnavailable)

	in := usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor},
		Currency:    "USD",
	}

	uc := usecase.NewGenerateVariations(provider, repo)
	ch, err := uc.Execute(context.Background(), in)
	require.NoError(t, err)

	events := collect(t, ch)
	last := events[len(events)-1]
	assert.Equal(t, domain.GenerationEventError, last.Type)
	require.NotNil(t, last.Generation)
	assert.Equal(t, domain.GenerationStatusFailed, last.Generation.Status)
	assert.Equal(t, []domain.GenerationStatus{domain.GenerationStatusStreaming, domain.GenerationStatusFailed}, savedStatuses)
}

func TestGenerateVariations_Execute_MidStreamErrorMarksGenerationFailed(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	repo.EXPECT().Save(gomock.Any(), gomock.Any()).Times(2).Return(nil)
	streamErr := errors.New("stream broke")
	provider.EXPECT().
		StreamStructured(gomock.Any(), gomock.Any()).
		Return(chunkChannel(
			domain.StreamChunk{Type: domain.StreamChunkToken, Delta: "partial"},
			domain.StreamChunk{Type: domain.StreamChunkError, Err: streamErr},
		), nil)

	in := usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor},
		Currency:    "USD",
	}

	uc := usecase.NewGenerateVariations(provider, repo)
	ch, err := uc.Execute(context.Background(), in)
	require.NoError(t, err)

	events := collect(t, ch)
	assert.Contains(t, eventTypes(events), domain.GenerationEventError)
	last := events[len(events)-1]
	require.NotNil(t, last.Generation)
	assert.Equal(t, domain.GenerationStatusFailed, last.Generation.Status)
}

func TestGenerateVariations_Execute_FinalSaveFailureEmitsError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	saveErr := errors.New("db down")
	gomock.InOrder(
		repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(nil),
		repo.EXPECT().Save(gomock.Any(), gomock.Any()).Return(saveErr),
	)
	provider.EXPECT().
		StreamStructured(gomock.Any(), gomock.Any()).
		Return(chunkChannel(successChunks(domain.StrategyAnchor)...), nil)

	in := usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor},
		Currency:    "USD",
	}

	uc := usecase.NewGenerateVariations(provider, repo)
	ch, err := uc.Execute(context.Background(), in)
	require.NoError(t, err)

	events := collect(t, ch)
	last := events[len(events)-1]
	assert.Equal(t, domain.GenerationEventError, last.Type)
	require.ErrorIs(t, last.Err, saveErr)
}

func TestGenerateVariations_Execute_ClosesOnContextCancellation(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)
	repo := mockdomain.NewMockGenerationRepo(ctrl)

	repo.EXPECT().Save(gomock.Any(), gomock.Any()).AnyTimes().Return(nil)

	block := make(chan domain.StreamChunk)
	provider.EXPECT().
		StreamStructured(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, _ domain.GenerationInput) (<-chan domain.StreamChunk, error) {
			// Never sends; the real adapters close their channel once ctx is
			// canceled, so block until then too.
			go func() {
				<-ctx.Done()
				close(block)
			}()
			return block, nil
		})

	in := usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor},
		Currency:    "USD",
	}

	uc := usecase.NewGenerateVariations(provider, repo)
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := uc.Execute(ctx, in)
	require.NoError(t, err)

	// Drain the two events the use case can deliver before it blocks inside
	// StreamStructured's mocked call (generation_started, then
	// variation_started — the latter's successful send happens-before the
	// StreamStructured call in streamStrategy, so by the time this returns
	// the mock is guaranteed to be invoked next), then cancel and confirm
	// the channel still closes promptly instead of leaking.
	<-ch
	<-ch
	cancel()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-ch:
			if !ok {
				return
			}
		case <-deadline:
			t.Fatal("channel did not close after context cancellation; goroutine leaked")
		}
	}
}
