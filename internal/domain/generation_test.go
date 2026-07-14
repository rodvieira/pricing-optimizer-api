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
		name    string
		mutate  func(in *GenerationInput)
		wantErr string
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
			wantErr: "invalid strategy",
		},
		{
			name: "missing site url is rejected",
			mutate: func(in *GenerationInput) {
				in.SiteProfile.URL = ""
			},
			wantErr: "site profile url is required",
		},
		{
			name: "lowercase currency is rejected",
			mutate: func(in *GenerationInput) {
				in.Currency = "usd"
			},
			wantErr: "invalid currency",
		},
		{
			name: "empty currency is rejected",
			mutate: func(in *GenerationInput) {
				in.Currency = ""
			},
			wantErr: "invalid currency",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := valid()
			tt.mutate(&in)

			err := in.Validate()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
