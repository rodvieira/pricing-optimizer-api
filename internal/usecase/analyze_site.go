package usecase

import (
	"context"
	"fmt"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// AnalyzeSite orchestrates scraping a URL and classifying the result into a
// SiteProfile via the configured LLMProvider.
type AnalyzeSite struct {
	scraper  domain.Scraper
	provider domain.LLMProvider
}

// NewAnalyzeSite creates the use case bound to scraper and provider. No
// handler is aware of which concrete Scraper or LLMProvider these are.
func NewAnalyzeSite(scraper domain.Scraper, provider domain.LLMProvider) *AnalyzeSite {
	return &AnalyzeSite{scraper: scraper, provider: provider}
}

// Execute scrapes url and classifies the result into a SiteProfile.
func (uc *AnalyzeSite) Execute(ctx context.Context, url string) (*domain.SiteProfile, error) {
	if url == "" {
		return nil, fmt.Errorf("analyze site: %w: url is required", ErrInvalidInput)
	}

	page, err := uc.scraper.Scrape(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("analyze site: scrape %s: %w", url, err)
	}

	profile, err := uc.provider.ClassifySite(ctx, *page)
	if err != nil {
		return nil, fmt.Errorf("analyze site: classify %s: %w", url, err)
	}
	return profile, nil
}
