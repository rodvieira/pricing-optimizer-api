package llm

import (
	"time"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

const (
	classifySiteToolName        = "emit_site_profile"
	classifySiteToolDescription = "Classify the scraped page content into a SiteProfile: value " +
		"proposition, industry, and target audience."
)

// siteProfileToolOutput mirrors domain.SiteProfile, minus the fields the
// model must never set: URL, Title, and SourceType come from the scrape
// itself, and AnalyzedAt is stamped by the adapter, not requested from the
// model.
type siteProfileToolOutput struct {
	ValueProposition string             `json:"valueProposition"`
	Industry         string             `json:"industry"`
	Audience         audienceToolOutput `json:"audience"`
	Keywords         []string           `json:"keywords"`
}

type audienceToolOutput struct {
	Segment        string `json:"segment"`
	Sophistication string `json:"sophistication"`
	PricePosition  string `json:"pricePosition"`
}

// siteProfileToolProperties and classifySiteToolRequired describe
// siteProfileToolOutput as a JSON schema. Both the Anthropic and
// OpenAI-compatible (Groq) tool-calling APIs accept a plain map[string]any
// for a tool's parameters, so the same values feed both.
func siteProfileToolProperties() map[string]any {
	audience := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"segment": map[string]any{
				"type":        "string",
				"description": "Target audience segment, e.g. \"SaaS founders\", \"enterprise IT\".",
			},
			"sophistication": map[string]any{
				"type": "string",
				"enum": []string{"low", "medium", "high"},
			},
			"pricePosition": map[string]any{
				"type": "string",
				"enum": []string{"budget", "mid_market", "premium"},
			},
		},
		"required": []string{"segment", "sophistication", "pricePosition"},
	}

	return map[string]any{
		"valueProposition": map[string]any{
			"type":        "string",
			"description": "One-to-two sentence summary of what the product offers.",
		},
		"industry": map[string]any{
			"type":        "string",
			"description": "Classified industry, e.g. \"developer-tools\", \"e-commerce\".",
		},
		"audience": audience,
		"keywords": map[string]any{
			"type":        "array",
			"items":       map[string]any{"type": "string"},
			"description": "Salient terms extracted from the page.",
		},
	}
}

var classifySiteToolRequired = []string{"valueProposition", "industry", "audience"}

// classifySiteToolJSONSchema is the full JSON Schema object, used by the
// OpenAI-compatible (Groq) function-calling API, which expects
// type/properties/required together rather than as separate fields.
func classifySiteToolJSONSchema() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": siteProfileToolProperties(),
		"required":   classifySiteToolRequired,
	}
}

// toDomainSiteProfile maps a decoded tool call into the domain type, filling
// in the fields the model was never asked for: URL, Title, and SourceType
// come from the scrape, and AnalyzedAt is stamped now.
func toDomainSiteProfile(page domain.ScrapedPage, out siteProfileToolOutput) domain.SiteProfile {
	return domain.SiteProfile{
		URL:              page.URL,
		Title:            page.Title,
		ValueProposition: out.ValueProposition,
		Industry:         out.Industry,
		Audience: domain.Audience{
			Segment:        out.Audience.Segment,
			Sophistication: domain.Sophistication(out.Audience.Sophistication),
			PricePosition:  domain.PricePosition(out.Audience.PricePosition),
		},
		Keywords:   out.Keywords,
		SourceType: page.SourceType,
		AnalyzedAt: time.Now(),
	}
}
