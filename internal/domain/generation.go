package domain

import (
	"errors"
	"fmt"
	"regexp"
)

var currencyPattern = regexp.MustCompile(`^[A-Z]{3}$`)

// GenerationInput is a single LLMProvider call: one site profile, one
// strategy. It is deliberately narrower than the multi-strategy /v1/generate
// API request — the use case that orchestrates parallel generation fans that
// request out into one GenerationInput per requested strategy.
type GenerationInput struct {
	SiteProfile SiteProfile
	Strategy    PricingStrategy
	Currency    string
}

// Validate checks the invariants an LLMProvider is entitled to assume about
// its input before spending an API call on it.
func (in GenerationInput) Validate() error {
	if !in.Strategy.Valid() {
		return fmt.Errorf("invalid strategy %q", in.Strategy)
	}
	if in.SiteProfile.URL == "" {
		return errors.New("site profile url is required")
	}
	if !currencyPattern.MatchString(in.Currency) {
		return fmt.Errorf("invalid currency %q: must be a 3-letter ISO 4217 code", in.Currency)
	}
	return nil
}
