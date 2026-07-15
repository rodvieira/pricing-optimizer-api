package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerationInput_Validate(t *testing.T) {
	t.Parallel()

	valid := func() GenerationInput {
		return GenerationInput{
			SiteProfile: SiteProfile{URL: "https://example.com"},
			Strategy:    StrategyFreemium,
			Currency:    "USD",
		}
	}

	tests := []struct {
		name       string
		mutate     func(in *GenerationInput)
		wantErrMsg string
	}{
		{
			name:   "valid input passes",
			mutate: func(in *GenerationInput) {},
		},
		{
			name: "invalid strategy is rejected",
			mutate: func(in *GenerationInput) {
				in.Strategy = "bogo"
			},
			wantErrMsg: "invalid strategy",
		},
		{
			name: "missing site url is rejected",
			mutate: func(in *GenerationInput) {
				in.SiteProfile.URL = ""
			},
			wantErrMsg: "site profile url is required",
		},
		{
			name: "lowercase currency is rejected",
			mutate: func(in *GenerationInput) {
				in.Currency = "usd"
			},
			wantErrMsg: "invalid currency",
		},
		{
			name: "empty currency is rejected",
			mutate: func(in *GenerationInput) {
				in.Currency = ""
			},
			wantErrMsg: "invalid currency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := valid()
			tt.mutate(&in)

			err := in.Validate()

			if tt.wantErrMsg != "" {
				require.ErrorIs(t, err, ErrInvalidGenerationInput)
				assert.Contains(t, err.Error(), tt.wantErrMsg)
				return
			}
			assert.NoError(t, err)
		})
	}
}
