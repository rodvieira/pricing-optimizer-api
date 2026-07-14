package scraper

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// ChromedpScraper renders a page in a real headless browser before
// extracting content, seeing client-side (SPA) rendering that a static HTTP
// fetch cannot.
type ChromedpScraper struct {
	execPath string
	timeout  time.Duration
}

// NewChromedpScraper creates a browser-backed scraper. execPath is the path
// to (or name of, resolved via PATH) the Chrome/Chromium binary to drive;
// pass "" to let chromedp discover it via its own well-known-path search.
func NewChromedpScraper(execPath string, timeout time.Duration) *ChromedpScraper {
	return &ChromedpScraper{execPath: execPath, timeout: timeout}
}

// Scrape implements domain.Scraper.
func (s *ChromedpScraper) Scrape(ctx context.Context, rawURL string) (*domain.ScrapedPage, error) {
	allocOpts := append(chromedp.DefaultExecAllocatorOptions[:], chromedp.NoSandbox)
	if s.execPath != "" {
		allocOpts = append(allocOpts, chromedp.ExecPath(s.execPath))
	}

	allocCtx, cancelAlloc := chromedp.NewExecAllocator(ctx, allocOpts...)
	defer cancelAlloc()

	browserCtx, cancelBrowser := chromedp.NewContext(allocCtx)
	defer cancelBrowser()

	runCtx, cancelTimeout := context.WithTimeout(browserCtx, s.timeout)
	defer cancelTimeout()

	var title, text string
	err := chromedp.Run(runCtx,
		chromedp.Navigate(rawURL),
		chromedp.Title(&title),
		chromedp.Text("body", &text, chromedp.ByQuery),
	)
	if err != nil {
		return nil, fmt.Errorf("chromedp: navigate %s: %w", rawURL, err)
	}

	return &domain.ScrapedPage{
		URL:        rawURL,
		Title:      strings.TrimSpace(title),
		Text:       strings.TrimSpace(text),
		SourceType: domain.SourceTypeSPA,
	}, nil
}
