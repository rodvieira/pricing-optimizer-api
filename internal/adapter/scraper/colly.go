// Package scraper implements domain.Scraper: a fast static-HTML fetch
// (colly) that a browser-backed fetch (chromedp) can fall back to when the
// static result looks too thin to be a real page, e.g. an SPA whose initial
// HTML is just an empty root element.
package scraper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

const collyUserAgent = "PricingOptimizerBot/1.0 (+https://github.com/rodvieira/pricing-optimizer-api)"

// CollyScraper fetches a URL over plain HTTP and parses the returned HTML.
// It cannot see content injected by client-side JavaScript.
type CollyScraper struct {
	timeout time.Duration
}

// NewCollyScraper creates a static-HTML scraper with the given per-request
// timeout.
func NewCollyScraper(timeout time.Duration) *CollyScraper {
	return &CollyScraper{timeout: timeout}
}

// Scrape implements domain.Scraper.
func (s *CollyScraper) Scrape(ctx context.Context, rawURL string) (*domain.ScrapedPage, error) {
	c := colly.NewCollector(colly.StdlibContext(ctx), colly.UserAgent(collyUserAgent))
	c.SetRequestTimeout(s.timeout)

	page := &domain.ScrapedPage{URL: rawURL, SourceType: domain.SourceTypeStatic}

	c.OnHTML("title", func(e *colly.HTMLElement) {
		if page.Title == "" {
			page.Title = strings.TrimSpace(e.Text)
		}
	})
	c.OnHTML("body", func(e *colly.HTMLElement) {
		// script/style content is not part of the page's visible text and
		// would otherwise pollute what gets fed to LLM classification.
		e.DOM.Find("script, style, noscript").Remove()
		page.Text = strings.TrimSpace(e.DOM.Text())
	})

	if err := c.Visit(rawURL); err != nil {
		return nil, fmt.Errorf("colly: visit %s: %w", rawURL, err)
	}
	if err := page.Validate(); err != nil {
		return nil, fmt.Errorf("colly: %w", err)
	}
	return page, nil
}
