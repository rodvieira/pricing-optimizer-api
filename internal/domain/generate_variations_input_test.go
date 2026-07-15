package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateVariationsInput_Validate(t *testing.T) {
	t.Parallel()

	valid := func() GenerateVariationsInput {
		return GenerateVariationsInput{
			SiteProfile: SiteProfile{URL: "https://example.com"},
			Strategies:  []PricingStrategy{StrategyAnchor, StrategyFreemium},
			Currency:    "USD",
		}
	}

	tests := []struct {
		name    string
		mutate  func(in *GenerateVariationsInput)
		wantErr error
	}{
		{
			name:   "valid input passes",
			mutate: func(in *GenerateVariationsInput) {},
		},
		{
			name: "missing site profile url is rejected",
			mutate: func(in *GenerateVariationsInput) {
				in.SiteProfile.URL = ""
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "no strategies is rejected",
			mutate: func(in *GenerateVariationsInput) {
				in.Strategies = nil
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "more than three strategies is rejected",
			mutate: func(in *GenerateVariationsInput) {
				in.Strategies = []PricingStrategy{StrategyAnchor, StrategyFreemium, StrategyValueBased, StrategyAnchor}
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "duplicate strategy is rejected",
			mutate: func(in *GenerateVariationsInput) {
				in.Strategies = []PricingStrategy{StrategyAnchor, StrategyAnchor}
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "unknown strategy is rejected",
			mutate: func(in *GenerateVariationsInput) {
				in.Strategies = []PricingStrategy{"bogus"}
			},
			wantErr: ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := valid()
			tt.mutate(&in)

			err := in.Validate()

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
