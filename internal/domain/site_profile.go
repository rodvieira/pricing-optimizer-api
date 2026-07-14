package domain

import (
	"fmt"
	"time"
)

// SourceType records which scraper produced a SiteProfile.
type SourceType string

const (
	SourceTypeSPA    SourceType = "spa"
	SourceTypeStatic SourceType = "static"
)

// Valid reports whether t is one of the known source types.
func (t SourceType) Valid() bool {
	switch t {
	case SourceTypeSPA, SourceTypeStatic:
		return true
	default:
		return false
	}
}

// SiteProfile is the result of analyzing a product URL: its value
// proposition, classification, and target audience.
type SiteProfile struct {
	URL              string
	Title            string
	ValueProposition string
	Industry         string
	Audience         Audience
	Keywords         []string
	SourceType       SourceType
	AnalyzedAt       time.Time
}

// Validate enforces the structural invariants a decoded LLM classification
// must satisfy, mirroring Variation.Validate: the JSON schema constrains the
// model's shape, this catches the cases it still gets wrong. It does not
// check Title, which comes from the scrape itself and may legitimately be
// empty (a page with no <title>), not from the model.
func (p SiteProfile) Validate() error {
	if p.URL == "" {
		return fmt.Errorf("%w: missing url", ErrInvalidLLMResponse)
	}
	if p.ValueProposition == "" {
		return fmt.Errorf("%w: missing value proposition", ErrInvalidLLMResponse)
	}
	if p.Industry == "" {
		return fmt.Errorf("%w: missing industry", ErrInvalidLLMResponse)
	}
	if p.Audience.Segment == "" {
		return fmt.Errorf("%w: missing audience segment", ErrInvalidLLMResponse)
	}
	if !p.Audience.Sophistication.Valid() {
		return fmt.Errorf("%w: invalid audience sophistication %q", ErrInvalidLLMResponse, p.Audience.Sophistication)
	}
	if !p.Audience.PricePosition.Valid() {
		return fmt.Errorf("%w: invalid audience price position %q", ErrInvalidLLMResponse, p.Audience.PricePosition)
	}
	if !p.SourceType.Valid() {
		return fmt.Errorf("%w: invalid source type %q", ErrInvalidLLMResponse, p.SourceType)
	}
	return nil
}
