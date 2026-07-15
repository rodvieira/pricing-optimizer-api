package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func TestRenderHTML(t *testing.T) {
	t.Parallel()

	v := fixtureExportVariation()
	result := renderHTML(v)

	assert.Equal(t, domain.ExportFormatHTML, result.Format)
	assert.Equal(t, "pricing.html", result.Filename)
	assert.Equal(t, "text/html", result.ContentType)

	content := result.Content
	assert.Contains(t, content, "<!doctype html>")
	assert.Contains(t, content, "<h2>Simple, transparent pricing</h2>")
	assert.Contains(t, content, "No hidden fees")
	assert.Contains(t, content, "$29/month")
	assert.Contains(t, content, "Feature A")
	assert.Contains(t, content, "tier--highlighted")
	assert.Contains(t, content, "Get started")
}

func TestRenderHTML_EscapesMarkupInjection(t *testing.T) {
	t.Parallel()

	v := fixtureExportVariation()
	v.Headline = `<script>alert("xss")</script>`
	result := renderHTML(v)

	assert.NotContains(t, result.Content, "<script>alert")
	assert.Contains(t, result.Content, "&lt;script&gt;")
}

func TestRenderHTML_OmitsEmptyOptionalFields(t *testing.T) {
	t.Parallel()

	v := domain.Variation{
		Headline: "Simple, transparent pricing",
		Tiers: []domain.PricingTier{
			{Name: "Free", Price: domain.Price{CustomLabel: "Free forever"}},
		},
	}
	result := renderHTML(v)

	assert.NotContains(t, result.Content, `class="subheadline"`)
	assert.NotContains(t, result.Content, `class="tagline"`)
	assert.NotContains(t, result.Content, `class="badge"`)
	assert.NotContains(t, result.Content, "<button")
	assert.Contains(t, result.Content, "Free forever")
}
