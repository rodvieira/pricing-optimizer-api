package scraper

import (
	"context"
	"fmt"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// minStaticTextLength is the extracted-text length below which a static
// fetch is treated as too thin to classify — typically an SPA whose initial
// HTML is just an empty root element — triggering a browser-backed re-scrape.
const minStaticTextLength = 200

// FallbackScraper tries a fast static fetch first and only pays for a real
// browser render when the static result looks too thin to classify.
type FallbackScraper struct {
	static  domain.Scraper
	browser domain.Scraper
}

// NewFallbackScraper composes a static and a browser-backed domain.Scraper.
func NewFallbackScraper(static, browser domain.Scraper) *FallbackScraper {
	return &FallbackScraper{static: static, browser: browser}
}

// Scrape implements domain.Scraper.
func (s *FallbackScraper) Scrape(ctx context.Context, rawURL string) (*domain.ScrapedPage, error) {
	page, staticErr := s.static.Scrape(ctx, rawURL)
	if staticErr == nil && len(page.Text) >= minStaticTextLength {
		return page, nil
	}

	browserPage, browserErr := s.browser.Scrape(ctx, rawURL)
	if browserErr != nil {
		if staticErr != nil {
			return nil, fmt.Errorf("static scrape failed (%w) and browser fallback failed: %w", staticErr, browserErr)
		}
		return nil, fmt.Errorf("browser fallback failed after thin static result: %w", browserErr)
	}
	return browserPage, nil
}
