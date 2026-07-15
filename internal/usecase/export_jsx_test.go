package usecase

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func fixtureExportVariation() domain.Variation {
	return domain.Variation{
		Headline:    "Simple, transparent pricing",
		Subheadline: "No hidden fees",
		Tiers: []domain.PricingTier{
			{
				Name:        "Pro",
				Tagline:     "For growing teams",
				Price:       domain.Price{AmountMinorUnits: 2900, Currency: "USD", Interval: domain.IntervalMonthly},
				Features:    []string{"Feature A", "Feature B"},
				CTA:         "Get started",
				Badge:       "Most popular",
				Highlighted: true,
			},
		},
	}
}

func TestRenderJSX(t *testing.T) {
	t.Parallel()

	v := fixtureExportVariation()
	result := renderJSX(v)

	assert.Equal(t, domain.ExportFormatJSX, result.Format)
	assert.Equal(t, "PricingSection.tsx", result.Filename)
	assert.Equal(t, "text/plain", result.ContentType)

	content := result.Content
	assert.Contains(t, content, "export default function PricingSection()")
	assert.Contains(t, content, `{"No hidden fees"}`)
	assert.Contains(t, content, `{"Pro"}`)
	assert.Contains(t, content, `{"$29/month"}`)
	assert.Contains(t, content, `{"For growing teams"}`)
	assert.Contains(t, content, `{"Feature A"}`)
	assert.Contains(t, content, `{"Feature B"}`)
	assert.Contains(t, content, `{"Get started"}`)
	assert.Contains(t, content, `{"Most popular"}`)
	assert.Contains(t, content, "tier tier--highlighted")
}

func TestRenderJSX_EscapesDangerousHeadline(t *testing.T) {
	t.Parallel()

	v := fixtureExportVariation()
	v.Headline = `"}); alert("xss"); ({"`
	result := renderJSX(v)

	// Extract the JSON string literal embedded between <h2>{ and }</h2> and
	// decode it back: if escaping worked, this round-trips to the exact
	// malicious input rather than letting any of it break out of the
	// literal or be parsed as a second JSX expression.
	start := strings.Index(result.Content, "<h2>{") + len("<h2>{")
	end := strings.Index(result.Content, "}</h2>")
	require.Greater(t, end, start)

	var decoded string
	require.NoError(t, json.Unmarshal([]byte(result.Content[start:end]), &decoded))
	assert.Equal(t, v.Headline, decoded)
}

func TestRenderJSX_OmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	v := domain.Variation{
		Headline: "Simple, transparent pricing",
		Tiers: []domain.PricingTier{
			{Name: "Free", Price: domain.Price{CustomLabel: "Free forever"}},
		},
	}
	result := renderJSX(v)

	assert.NotContains(t, result.Content, "subheadline")
	assert.NotContains(t, result.Content, "tagline")
	assert.NotContains(t, result.Content, "badge")
	assert.NotContains(t, result.Content, "<button")
	assert.Contains(t, result.Content, `{"Free forever"}`)
}
