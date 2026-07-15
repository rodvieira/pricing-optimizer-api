package usecase

import (
	"context"
	"fmt"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// ExportVariation looks up a previously generated Variation within a
// Generation and renders it into the requested export format. Unlike
// GenerateVariations, it makes no LLMProvider call: it is a pure
// transformation over data the GenerationRepo already has.
type ExportVariation struct {
	repo domain.GenerationRepo
}

// NewExportVariation creates the use case bound to repo, the same
// GenerationRepo GenerateVariations saves through.
func NewExportVariation(repo domain.GenerationRepo) *ExportVariation {
	return &ExportVariation{repo: repo}
}

// Execute validates in, fetches the Generation at in.GenerationID, finds the
// Variation at in.VariationID within it, and renders it per in.Format.
// Returns domain.ErrGenerationNotFound or domain.ErrVariationNotFound
// (wrapped) when either lookup fails.
func (uc *ExportVariation) Execute(ctx context.Context, in domain.ExportVariationInput) (*domain.ExportResult, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("export variation: %w", err)
	}

	gen, err := uc.repo.Get(ctx, in.GenerationID)
	if err != nil {
		return nil, fmt.Errorf("export variation: %w", err)
	}

	variation, ok := findVariation(gen.Variations, in.VariationID)
	if !ok {
		return nil, fmt.Errorf("export variation: %w: %s", domain.ErrVariationNotFound, in.VariationID)
	}

	switch in.Format {
	case domain.ExportFormatJSX:
		return renderJSX(variation), nil
	case domain.ExportFormatHTML:
		return renderHTML(variation), nil
	case domain.ExportFormatStripe:
		return renderStripe(variation), nil
	default:
		// Unreachable: in.Validate() already rejected any format that isn't
		// one of the three ExportFormat.Valid() knows about.
		return nil, fmt.Errorf("export variation: %w: unsupported format %q", domain.ErrInvalidInput, in.Format)
	}
}

// findVariation returns the Variation in vs whose ID matches id.
func findVariation(vs []domain.Variation, id string) (domain.Variation, bool) {
	for _, v := range vs {
		if v.ID == id {
			return v, true
		}
	}
	return domain.Variation{}, false
}
