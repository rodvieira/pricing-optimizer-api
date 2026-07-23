package scraper

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/chromedp/chromedp"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// hostGuardFunc validates rawURL's host before it is handed to a real
// browser process. Narrowed so tests can inject a no-op guard to reach an
// httptest server on 127.0.0.1 — which guardHostResolvesToPublicAddress,
// the production default, deliberately refuses.
type hostGuardFunc func(ctx context.Context, rawURL string) error

// ChromedpScraper renders a page in a real headless browser before
// extracting content, seeing client-side (SPA) rendering that a static HTTP
// fetch cannot.
type ChromedpScraper struct {
	execPath  string
	timeout   time.Duration
	hostGuard hostGuardFunc
}

// NewChromedpScraper creates a browser-backed scraper. execPath is the path
// to (or name of, resolved via PATH) the Chrome/Chromium binary to drive;
// pass "" to let chromedp discover it via its own well-known-path search.
func NewChromedpScraper(execPath string, timeout time.Duration) *ChromedpScraper {
	return &ChromedpScraper{execPath: execPath, timeout: timeout, hostGuard: guardHostResolvesToPublicAddress}
}

// guardHostResolvesToPublicAddress re-validates rawURL's host against the
// same SSRF policy usecase.validateAnalyzeURL applies at request time (see
// resolveAllowedIP's doc comment for why a second check is needed at all),
// specifically for this scraper: chromedp.Navigate hands rawURL to a real
// Chrome process, which does its own DNS resolution internally — this
// package cannot intercept or pin that the way safeDialer pins the colly
// path's dial address. This closes most of the same window (rejecting up
// front if the hostname resolves only to a disallowed address right now,
// including the redirect case for the initial navigation) but leaves a
// smaller residual TOCTOU race than the colly path's: an attacker serving a
// zero-TTL DNS answer that changes between this check and Chrome's own
// resolution moments later, or a redirect Chrome itself follows after
// navigation starts. Fully closing that needs network-level isolation for
// the browser process (issue #31, parked) or CDP-level request
// interception; this is the pragmatic mitigation available without either.
func guardHostResolvesToPublicAddress(ctx context.Context, rawURL string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("parse url: %w", err)
	}
	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("url %q is missing a host", rawURL)
	}
	_, err = resolveAllowedIP(ctx, host, net.DefaultResolver.LookupIPAddr)
	return err
}

// Scrape implements domain.Scraper.
func (s *ChromedpScraper) Scrape(ctx context.Context, rawURL string) (*domain.ScrapedPage, error) {
	if err := s.hostGuard(ctx, rawURL); err != nil {
		return nil, fmt.Errorf("chromedp: %w", err)
	}

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

	page := &domain.ScrapedPage{
		URL:        rawURL,
		Title:      strings.TrimSpace(title),
		Text:       strings.TrimSpace(text),
		SourceType: domain.SourceTypeSPA,
	}
	if err := page.Validate(); err != nil {
		return nil, fmt.Errorf("chromedp: %w", err)
	}
	return page, nil
}
