package usecase

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

const (
	minStrategies = 1
	maxStrategies = 3
)

// GenerateVariationsInput is the multi-strategy request the GenerateVariations
// use case fans out into one domain.GenerationInput per strategy.
type GenerateVariationsInput struct {
	SiteProfile domain.SiteProfile
	Strategies  []domain.PricingStrategy
	Currency    string
}

// Validate checks the invariants this use case needs before spending any LLM
// calls: 1-3 known strategies, no duplicates.
func (in GenerateVariationsInput) Validate() error {
	if len(in.Strategies) < minStrategies || len(in.Strategies) > maxStrategies {
		return fmt.Errorf("%w: must request %d-%d strategies, got %d",
			ErrInvalidInput, minStrategies, maxStrategies, len(in.Strategies))
	}
	seen := make(map[domain.PricingStrategy]struct{}, len(in.Strategies))
	for _, s := range in.Strategies {
		if !s.Valid() {
			return fmt.Errorf("%w: invalid strategy %q", ErrInvalidInput, s)
		}
		if _, dup := seen[s]; dup {
			return fmt.Errorf("%w: duplicate strategy %q", ErrInvalidInput, s)
		}
		seen[s] = struct{}{}
	}
	return nil
}

// GenerateVariations orchestrates parallel structured generation across the
// requested pricing strategies via the configured LLMProvider.
type GenerateVariations struct {
	provider domain.LLMProvider
}

// NewGenerateVariations creates the use case bound to provider. No handler or
// other use case is aware of which concrete LLMProvider this is.
func NewGenerateVariations(provider domain.LLMProvider) *GenerateVariations {
	return &GenerateVariations{provider: provider}
}

// Execute generates one Variation per requested strategy in parallel. If any
// call fails, sibling calls are canceled via the shared context and the first
// error is returned; no partial results are returned on failure.
func (uc *GenerateVariations) Execute(
	ctx context.Context, in GenerateVariationsInput,
) ([]domain.Variation, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("generate variations: %w", err)
	}

	variations := make([]domain.Variation, len(in.Strategies))

	g, gctx := errgroup.WithContext(ctx)
	for i, strategy := range in.Strategies {
		g.Go(func() error {
			input := domain.GenerationInput{
				SiteProfile: in.SiteProfile,
				Strategy:    strategy,
				Currency:    in.Currency,
			}
			variation, err := uc.provider.GenerateStructured(gctx, input)
			if err != nil {
				return fmt.Errorf("generate %s variation: %w", strategy, err)
			}
			variations[i] = *variation
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, fmt.Errorf("generate variations: %w", err)
	}
	return variations, nil
}
