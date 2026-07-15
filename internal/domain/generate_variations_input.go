package domain

import "fmt"

const (
	minStrategies = 1
	maxStrategies = 3
)

// GenerateVariationsInput is the POST /v1/generate request the
// GenerateVariations use case fans out into one LLMProvider.StreamStructured
// call per strategy. Defaulting an omitted Strategies list to all three
// strategies, per the openapi.yaml contract, is the HTTP layer's job, not
// this use case's: Execute always requires an already-resolved 1-3 list.
// There is no separate source-URL field: the Generation's SourceURL is
// SiteProfile.URL, since that's the only URL the GenerateRequest contract
// carries (via the SiteProfile the client got from a prior /v1/analyze call).
//
// Lives in domain, not usecase, so the HTTP handler can reference it in its
// own consumer-defined interface without importing usecase.
type GenerateVariationsInput struct {
	SiteProfile SiteProfile
	Strategies  []PricingStrategy
	Currency    string
}

// Validate checks the invariants GenerateVariations needs before spending
// any LLM calls: 1-3 known strategies, no duplicates.
func (in GenerateVariationsInput) Validate() error {
	if in.SiteProfile.URL == "" {
		return fmt.Errorf("%w: site profile url is required", ErrInvalidInput)
	}
	if len(in.Strategies) < minStrategies || len(in.Strategies) > maxStrategies {
		return fmt.Errorf("%w: must request %d-%d strategies, got %d",
			ErrInvalidInput, minStrategies, maxStrategies, len(in.Strategies))
	}
	seen := make(map[PricingStrategy]struct{}, len(in.Strategies))
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
