package domain

import "fmt"

// ExportFormat is the artifact shape POST /v1/export/{id} can render a
// Variation into.
type ExportFormat string

const (
	ExportFormatJSX    ExportFormat = "jsx"
	ExportFormatHTML   ExportFormat = "html"
	ExportFormatStripe ExportFormat = "stripe"
)

// Valid reports whether f is one of the known export formats.
func (f ExportFormat) Valid() bool {
	switch f {
	case ExportFormatJSX, ExportFormatHTML, ExportFormatStripe:
		return true
	default:
		return false
	}
}

// ExportVariationInput is the POST /v1/export/{id} request the
// ExportVariation use case resolves into an ExportResult: which Generation
// (GenerationID, from the path) and which of its Variations (VariationID,
// from the request body) to render, and in which ExportFormat.
//
// Lives in domain, not usecase, so the HTTP handler can reference it in its
// own consumer-defined interface without importing usecase, the same
// rationale as GenerateVariationsInput.
type ExportVariationInput struct {
	GenerationID string
	VariationID  string
	Format       ExportFormat
}

// Validate checks the invariants ExportVariation needs before looking
// anything up in the GenerationRepo.
func (in ExportVariationInput) Validate() error {
	if in.GenerationID == "" {
		return fmt.Errorf("%w: generation id is required", ErrInvalidInput)
	}
	if in.VariationID == "" {
		return fmt.Errorf("%w: variation id is required", ErrInvalidInput)
	}
	if !in.Format.Valid() {
		return fmt.Errorf("%w: invalid export format %q", ErrInvalidInput, in.Format)
	}
	return nil
}

// ExportResult is one rendered export artifact: Content is the full text of
// the requested Format (a JSX component, a standalone HTML document, or a
// Stripe Pricing Table JSON config), alongside a suggested Filename and the
// MIME type it should be served/saved as.
type ExportResult struct {
	Format      ExportFormat
	Filename    string
	ContentType string
	Content     string
}
