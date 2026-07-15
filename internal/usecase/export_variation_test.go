package usecase_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	"github.com/rodvieira/pricing-optimizer-api/internal/usecase"
	mockdomain "github.com/rodvieira/pricing-optimizer-api/test/mocks/domain"
)

const (
	fixtureGenerationID = "b6f1c6b2-6b8a-4b1a-9f1a-1c2c3c4c5c6c"
	fixtureVariationID  = "a1a1a1a1-1111-1111-1111-111111111111"
)

func fixtureGenerationWithVariation() domain.Generation {
	return domain.Generation{
		ID:     fixtureGenerationID,
		Status: domain.GenerationStatusCompleted,
		Variations: []domain.Variation{
			{
				ID:       fixtureVariationID,
				Strategy: domain.StrategyAnchor,
				Headline: "Simple, transparent pricing",
				Tiers: []domain.PricingTier{
					{
						Name:     "Pro",
						Price:    domain.Price{AmountMinorUnits: 2900, Currency: "USD", Interval: domain.IntervalMonthly},
						Features: []string{"Feature A"},
					},
				},
			},
		},
	}
}

func TestExportVariation_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		in              domain.ExportVariationInput
		setup           func(r *mockdomain.MockGenerationRepo)
		wantErr         error
		wantFilename    string
		wantContentType string
	}{
		{
			name: "jsx format renders a React component",
			in: domain.ExportVariationInput{
				GenerationID: fixtureGenerationID, VariationID: fixtureVariationID, Format: domain.ExportFormatJSX,
			},
			setup: func(r *mockdomain.MockGenerationRepo) {
				gen := fixtureGenerationWithVariation()
				r.EXPECT().Get(gomock.Any(), fixtureGenerationID).Return(&gen, nil)
			},
			wantFilename:    "PricingSection.tsx",
			wantContentType: "text/plain",
		},
		{
			name: "html format renders a standalone document",
			in: domain.ExportVariationInput{
				GenerationID: fixtureGenerationID, VariationID: fixtureVariationID, Format: domain.ExportFormatHTML,
			},
			setup: func(r *mockdomain.MockGenerationRepo) {
				gen := fixtureGenerationWithVariation()
				r.EXPECT().Get(gomock.Any(), fixtureGenerationID).Return(&gen, nil)
			},
			wantFilename:    "pricing.html",
			wantContentType: "text/html",
		},
		{
			name: "stripe format renders a pricing table config",
			in: domain.ExportVariationInput{
				GenerationID: fixtureGenerationID, VariationID: fixtureVariationID, Format: domain.ExportFormatStripe,
			},
			setup: func(r *mockdomain.MockGenerationRepo) {
				gen := fixtureGenerationWithVariation()
				r.EXPECT().Get(gomock.Any(), fixtureGenerationID).Return(&gen, nil)
			},
			wantFilename:    "stripe-pricing-table.json",
			wantContentType: "application/json",
		},
		{
			name: "invalid input never reaches the repo",
			in: domain.ExportVariationInput{
				GenerationID: "", VariationID: fixtureVariationID, Format: domain.ExportFormatJSX,
			},
			setup:   func(r *mockdomain.MockGenerationRepo) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "unknown generation id is not found",
			in: domain.ExportVariationInput{
				GenerationID: "missing", VariationID: fixtureVariationID, Format: domain.ExportFormatJSX,
			},
			setup: func(r *mockdomain.MockGenerationRepo) {
				r.EXPECT().Get(gomock.Any(), "missing").
					Return(nil, fmt.Errorf("%w: missing", domain.ErrGenerationNotFound))
			},
			wantErr: domain.ErrGenerationNotFound,
		},
		{
			name: "unknown variation id within a real generation is not found",
			in: domain.ExportVariationInput{
				GenerationID: fixtureGenerationID, VariationID: "missing", Format: domain.ExportFormatJSX,
			},
			setup: func(r *mockdomain.MockGenerationRepo) {
				gen := fixtureGenerationWithVariation()
				r.EXPECT().Get(gomock.Any(), fixtureGenerationID).Return(&gen, nil)
			},
			wantErr: domain.ErrVariationNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			repo := mockdomain.NewMockGenerationRepo(ctrl)
			tt.setup(repo)

			uc := usecase.NewExportVariation(repo)
			result, err := uc.Execute(context.Background(), tt.in)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, result)
				return
			}
			require.NoError(t, err)
			require.NotNil(t, result)
			assert.Equal(t, tt.wantFilename, result.Filename)
			assert.Equal(t, tt.wantContentType, result.ContentType)
			assert.NotEmpty(t, result.Content)
		})
	}
}
