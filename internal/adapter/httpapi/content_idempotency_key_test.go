package httpapi

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func baseGenerateVariationsInput() domain.GenerateVariationsInput {
	return domain.GenerateVariationsInput{
		SiteProfile: domain.SiteProfile{
			URL:              "https://example.com",
			Title:            "Acme Analytics",
			ValueProposition: "Real-time analytics for indie SaaS founders",
			Industry:         "developer-tools",
			Audience:         domain.Audience{Segment: "SaaS founders", Sophistication: domain.SophisticationMedium},
			SourceType:       domain.SourceTypeStatic,
			AnalyzedAt:       time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		},
		Strategies: []domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyFreemium},
		Currency:   "USD",
	}
}

func TestContentIdempotencyKey(t *testing.T) {
	t.Parallel()

	t.Run("identical input produces the same key", func(t *testing.T) {
		t.Parallel()
		in := baseGenerateVariationsInput()
		assert.Equal(t, contentIdempotencyKey(in), contentIdempotencyKey(in))
	})

	t.Run("AnalyzedAt does not affect the key", func(t *testing.T) {
		t.Parallel()
		a := baseGenerateVariationsInput()
		b := baseGenerateVariationsInput()
		b.SiteProfile.AnalyzedAt = time.Date(2030, 1, 1, 0, 0, 0, 0, time.UTC)
		assert.Equal(t, contentIdempotencyKey(a), contentIdempotencyKey(b),
			"AnalyzedAt is a scrape timestamp, not semantic content, and must not fragment the cache")
	})

	t.Run("strategy order does not affect the key", func(t *testing.T) {
		t.Parallel()
		a := baseGenerateVariationsInput()
		b := baseGenerateVariationsInput()
		b.Strategies = []domain.PricingStrategy{domain.StrategyFreemium, domain.StrategyAnchor}
		assert.Equal(t, contentIdempotencyKey(a), contentIdempotencyKey(b))
	})

	t.Run("a different URL produces a different key", func(t *testing.T) {
		t.Parallel()
		a := baseGenerateVariationsInput()
		b := baseGenerateVariationsInput()
		b.SiteProfile.URL = "https://different.example"
		assert.NotEqual(t, contentIdempotencyKey(a), contentIdempotencyKey(b))
	})

	t.Run("a different currency produces a different key", func(t *testing.T) {
		t.Parallel()
		a := baseGenerateVariationsInput()
		b := baseGenerateVariationsInput()
		b.Currency = "EUR"
		assert.NotEqual(t, contentIdempotencyKey(a), contentIdempotencyKey(b))
	})

	t.Run("a different strategy set produces a different key", func(t *testing.T) {
		t.Parallel()
		a := baseGenerateVariationsInput()
		b := baseGenerateVariationsInput()
		b.Strategies = []domain.PricingStrategy{domain.StrategyValueBased}
		assert.NotEqual(t, contentIdempotencyKey(a), contentIdempotencyKey(b))
	})
}
