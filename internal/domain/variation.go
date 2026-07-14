package domain

import "fmt"

// Variation is one generated pricing-page variation.
//
// ID is assigned by the LLMProvider adapter after a successful, validated
// response — never requested from the model, which cannot be trusted to
// produce a valid, unique identifier.
type Variation struct {
	ID          string
	Strategy    PricingStrategy
	Headline    string
	Subheadline string
	Tiers       []PricingTier
	Rationale   string
}

const (
	minTiers = 1
	maxTiers = 5
)

// Validate enforces the structural invariants a decoded LLM tool call must
// satisfy. The JSON schema handed to the model constrains its shape; this
// catches the cases models still get wrong (missing fields, empty text,
// too many/few tiers) so adapters can fail fast with ErrInvalidLLMResponse
// instead of returning a variation the API contract would reject anyway.
func (v Variation) Validate() error {
	if !v.Strategy.Valid() {
		return fmt.Errorf("%w: invalid strategy %q", ErrInvalidLLMResponse, v.Strategy)
	}
	if v.Headline == "" {
		return fmt.Errorf("%w: missing headline", ErrInvalidLLMResponse)
	}
	if len(v.Tiers) < minTiers || len(v.Tiers) > maxTiers {
		return fmt.Errorf("%w: must have %d-%d tiers, got %d",
			ErrInvalidLLMResponse, minTiers, maxTiers, len(v.Tiers))
	}
	for i, t := range v.Tiers {
		if err := t.validate(); err != nil {
			return fmt.Errorf("tier %d: %w", i, err)
		}
	}
	return nil
}

func (t PricingTier) validate() error {
	if t.Name == "" {
		return fmt.Errorf("%w: missing tier name", ErrInvalidLLMResponse)
	}
	if t.Price.CustomLabel == "" {
		if t.Price.Currency == "" {
			return fmt.Errorf("%w: tier %q missing price", ErrInvalidLLMResponse, t.Name)
		}
		if !t.Price.Interval.Valid() {
			return fmt.Errorf("%w: tier %q has invalid interval %q",
				ErrInvalidLLMResponse, t.Name, t.Price.Interval)
		}
		if t.Price.AmountMinorUnits < 0 {
			return fmt.Errorf("%w: tier %q has a negative price", ErrInvalidLLMResponse, t.Name)
		}
	}
	return nil
}
