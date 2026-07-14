// Package llm implements domain.LLMProvider against Anthropic and Groq using
// structured tool calling: the model must return a typed tool call matching
// a fixed JSON schema, never free-form text.
package llm

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

const (
	toolName        = "emit_pricing_variation"
	toolDescription = "Emit one complete pricing-page variation for the requested strategy."
)

// toolOutput mirrors domain.Variation, minus the two fields the model must
// never set: ID (untrustworthy) and Strategy (already known from the
// request).
type toolOutput struct {
	Headline    string       `json:"headline"`
	Subheadline string       `json:"subheadline"`
	Tiers       []tierOutput `json:"tiers"`
	Rationale   string       `json:"rationale"`
}

type tierOutput struct {
	Name        string      `json:"name"`
	Price       priceOutput `json:"price"`
	Tagline     string      `json:"tagline"`
	Features    []string    `json:"features"`
	CTA         string      `json:"cta"`
	Highlighted bool        `json:"highlighted"`
	Badge       string      `json:"badge"`
}

type priceOutput struct {
	AmountMinorUnits int    `json:"amountMinorUnits"`
	Currency         string `json:"currency"`
	Interval         string `json:"interval"`
	CustomLabel      string `json:"customLabel"`
}

// toolProperties and toolRequired describe toolOutput as a JSON schema.
// Both the Anthropic and OpenAI-compatible (Groq) tool-calling APIs accept a
// plain map[string]any for a tool's parameters, so the same values feed both.
func toolProperties() map[string]any {
	price := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"amountMinorUnits": map[string]any{
				"type":        "integer",
				"description": "Amount in the currency's minor unit (e.g. cents). 0 means free.",
			},
			"currency": map[string]any{
				"type":        "string",
				"description": "ISO 4217 currency code, e.g. USD.",
			},
			"interval": map[string]any{
				"type": "string",
				"enum": []string{"one_time", "monthly", "yearly"},
			},
			"customLabel": map[string]any{
				"type":        "string",
				"description": "Overrides the rendered price, e.g. \"Contact us\". Empty otherwise.",
			},
		},
		"required": []string{"amountMinorUnits", "currency", "interval"},
	}

	tier := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":        map[string]any{"type": "string"},
			"price":       price,
			"tagline":     map[string]any{"type": "string"},
			"features":    map[string]any{"type": "array", "items": map[string]any{"type": "string"}},
			"cta":         map[string]any{"type": "string"},
			"highlighted": map[string]any{"type": "boolean"},
			"badge":       map[string]any{"type": "string"},
		},
		"required": []string{"name", "price", "features"},
	}

	return map[string]any{
		"headline":    map[string]any{"type": "string"},
		"subheadline": map[string]any{"type": "string"},
		"tiers": map[string]any{
			"type":     "array",
			"items":    tier,
			"minItems": 1,
			"maxItems": 5,
		},
		"rationale": map[string]any{"type": "string"},
	}
}

var toolRequired = []string{"headline", "tiers", "rationale"}

// toolJSONSchema is the full JSON Schema object, used by the OpenAI-compatible
// (Groq) function-calling API, which expects type/properties/required
// together rather than as separate fields.
func toolJSONSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": toolProperties(),
		"required":   toolRequired,
	}
}

// toDomainVariation maps a decoded tool call into the domain type, assigning
// the id and strategy the model was never asked for.
func toDomainVariation(id string, strategy domain.PricingStrategy, out toolOutput) domain.Variation {
	tiers := make([]domain.PricingTier, len(out.Tiers))
	for i, t := range out.Tiers {
		tiers[i] = domain.PricingTier{
			Name:    t.Name,
			Tagline: t.Tagline,
			Price: domain.Price{
				AmountMinorUnits: t.Price.AmountMinorUnits,
				Currency:         t.Price.Currency,
				Interval:         domain.Interval(t.Price.Interval),
				CustomLabel:      t.Price.CustomLabel,
			},
			Features:    t.Features,
			CTA:         t.CTA,
			Highlighted: t.Highlighted,
			Badge:       t.Badge,
		}
	}
	return domain.Variation{
		ID:          id,
		Strategy:    strategy,
		Headline:    out.Headline,
		Subheadline: out.Subheadline,
		Tiers:       tiers,
		Rationale:   out.Rationale,
	}
}

// decodeStreamedVariation decodes and validates the tool-input JSON
// accumulated from a StreamStructured call, shared by every provider's
// streaming implementation.
func decodeStreamedVariation(strategy domain.PricingStrategy, accumulatedJSON string) (*domain.Variation, error) {
	var decoded toolOutput
	if err := json.Unmarshal([]byte(accumulatedJSON), &decoded); err != nil {
		return nil, fmt.Errorf("%w: %w", domain.ErrInvalidLLMResponse, err)
	}
	variation := toDomainVariation(uuid.NewString(), strategy, decoded)
	if err := variation.Validate(); err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	return &variation, nil
}
