package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func validTier() PricingTier {
	return PricingTier{
		Name:     "Pro",
		Price:    Price{AmountMinorUnits: 2900, Currency: "USD", Interval: IntervalMonthly},
		Features: []string{"Feature A", "Feature B"},
	}
}

func TestVariation_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(v *Variation)
		wantErr error
	}{
		{
			name:   "valid variation passes",
			mutate: func(v *Variation) {},
		},
		{
			name: "invalid strategy is rejected",
			mutate: func(v *Variation) {
				v.Strategy = "discount_bomb"
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "empty headline is rejected",
			mutate: func(v *Variation) {
				v.Headline = ""
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "zero tiers is rejected",
			mutate: func(v *Variation) {
				v.Tiers = nil
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "more than five tiers is rejected",
			mutate: func(v *Variation) {
				tiers := make([]PricingTier, 6)
				for i := range tiers {
					tiers[i] = validTier()
				}
				v.Tiers = tiers
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "tier with no name is rejected",
			mutate: func(v *Variation) {
				v.Tiers[0].Name = ""
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "tier with neither price nor custom label is rejected",
			mutate: func(v *Variation) {
				v.Tiers[0].Price = Price{}
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "tier with a custom label but no currency is accepted",
			mutate: func(v *Variation) {
				v.Tiers[0].Price = Price{CustomLabel: "Contact us"}
			},
		},
		{
			name: "tier with an invalid interval is rejected",
			mutate: func(v *Variation) {
				v.Tiers[0].Price.Interval = "annual"
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "tier with a negative price is rejected",
			mutate: func(v *Variation) {
				v.Tiers[0].Price.AmountMinorUnits = -100
			},
			wantErr: ErrInvalidLLMResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			v := Variation{
				Strategy:  StrategyAnchor,
				Headline:  "Simple, transparent pricing",
				Tiers:     []PricingTier{validTier()},
				Rationale: "Anchors the Pro tier against a higher-priced decoy.",
			}
			tt.mutate(&v)

			err := v.Validate()

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
