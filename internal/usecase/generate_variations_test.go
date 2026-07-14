package usecase_test

import (
	"context"
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

func TestGenerateVariations_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		in      usecase.GenerateVariationsInput
		setup   func(m *mockdomain.MockLLMProvider)
		want    int // number of variations, when no error is expected
		wantErr error
	}{
		{
			name: "generates one variation per requested strategy",
			in:   validGenerateVariationsInput(),
			setup: func(m *mockdomain.MockLLMProvider) {
				m.EXPECT().
					GenerateStructured(gomock.Any(), gomock.Any()).
					DoAndReturn(func(_ context.Context, in domain.GenerationInput) (*domain.Variation, error) {
						return fixtureVariation(in.Strategy), nil
					}).
					Times(2)
			},
			want: 2,
		},
		{
			// Cancellation itself is exercised separately in
			// TestGenerateVariations_Execute_CancelsSiblingsOnFirstError;
			// this case only checks that the aggregate error propagates
			// regardless of how many sibling calls happened to run first.
			name: "returns the first provider error",
			in:   validGenerateVariationsInput(),
			setup: func(m *mockdomain.MockLLMProvider) {
				m.EXPECT().
					GenerateStructured(gomock.Any(), gomock.Any()).
					Return(nil, domain.ErrProviderUnavailable).
					MinTimes(1)
			},
			wantErr: domain.ErrProviderUnavailable,
		},
		{
			name: "no strategies is rejected before any provider call",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies:  nil,
				Currency:    "USD",
			},
			setup:   func(m *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "more than three strategies is rejected before any provider call",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies: []domain.PricingStrategy{
					domain.StrategyAnchor, domain.StrategyFreemium, domain.StrategyValueBased, domain.StrategyAnchor,
				},
				Currency: "USD",
			},
			setup:   func(m *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "duplicate strategy is rejected before any provider call",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies:  []domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyAnchor},
				Currency:    "USD",
			},
			setup:   func(m *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "unknown strategy is rejected before any provider call",
			in: usecase.GenerateVariationsInput{
				SiteProfile: fixtureSiteProfile(),
				Strategies:  []domain.PricingStrategy{"bogus"},
				Currency:    "USD",
			},
			setup:   func(m *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			provider := mockdomain.NewMockLLMProvider(ctrl)
			tt.setup(provider)

			uc := usecase.NewGenerateVariations(provider)
			got, err := uc.Execute(context.Background(), tt.in)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Len(t, got, tt.want)
			for i, strategy := range tt.in.Strategies {
				assert.Equal(t, strategy, got[i].Strategy, "variation at index %d must match its requested strategy", i)
			}
		})
	}
}

func TestGenerateVariations_Execute_PropagatesEachStrategyToItsCall(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)

	in := usecase.GenerateVariationsInput{
		SiteProfile: fixtureSiteProfile(),
		Strategies:  []domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyFreemium, domain.StrategyValueBased},
		Currency:    "EUR",
	}

	provider.EXPECT().
		GenerateStructured(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, got domain.GenerationInput) (*domain.Variation, error) {
			assert.Equal(t, in.SiteProfile, got.SiteProfile)
			assert.Equal(t, in.Currency, got.Currency)
			assert.True(t, got.Strategy.Valid())
			return fixtureVariation(got.Strategy), nil
		}).
		Times(3)

	uc := usecase.NewGenerateVariations(provider)
	got, err := uc.Execute(context.Background(), in)

	require.NoError(t, err)
	require.Len(t, got, 3)
	assert.ElementsMatch(t,
		[]domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyFreemium, domain.StrategyValueBased},
		[]domain.PricingStrategy{got[0].Strategy, got[1].Strategy, got[2].Strategy},
	)
}

func TestGenerateVariations_Execute_CancelsSiblingsOnFirstError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	provider := mockdomain.NewMockLLMProvider(ctrl)

	provider.EXPECT().
		GenerateStructured(gomock.Any(), gomock.Any()).
		DoAndReturn(func(ctx context.Context, in domain.GenerationInput) (*domain.Variation, error) {
			if in.Strategy == domain.StrategyAnchor {
				return nil, domain.ErrProviderUnavailable
			}
			// The sibling call: this only returns because errgroup
			// canceled the shared context in response to the anchor
			// call's error. If Execute stopped propagating ctx to each
			// GenerateStructured call, this would block forever and the
			// timeout below would catch it.
			<-ctx.Done()
			return nil, ctx.Err()
		}).
		Times(2)

	uc := usecase.NewGenerateVariations(provider)

	done := make(chan error, 1)
	go func() {
		_, err := uc.Execute(context.Background(), usecase.GenerateVariationsInput{
			SiteProfile: fixtureSiteProfile(),
			Strategies:  []domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyFreemium},
			Currency:    "USD",
		})
		done <- err
	}()

	select {
	case err := <-done:
		require.ErrorIs(t, err, domain.ErrProviderUnavailable)
	case <-time.After(2 * time.Second):
		t.Fatal("Execute did not return; sibling call was not canceled by the shared context")
	}
}
