// Package scraper implements domain.Scraper: a fast static-HTML fetch
// (colly) that a browser-backed fetch (chromedp) can fall back to when the
// static result looks too thin to be a real page, e.g. an SPA whose initial
// HTML is just an empty root element.
package scraper

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/gocolly/colly/v2"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

const collyUserAgent = "PricingOptimizerBot/1.0 (+https://github.com/rodvieira/pricing-optimizer-api)"

// dialContextFunc matches net.Dialer.DialContext's signature (and, via that,
// http.Transport.DialContext's), narrowed so tests can inject an unguarded
// dialer to reach an httptest server on 127.0.0.1 — which safeDialer, the
// production default, deliberately refuses to connect to.
type dialContextFunc func(ctx context.Context, network, addr string) (net.Conn, error)

// CollyScraper fetches a URL over plain HTTP and parses the returned HTML.
// It cannot see content injected by client-side JavaScript.
type CollyScraper struct {
	timeout time.Duration
	dial    dialContextFunc
}

// NewCollyScraper creates a static-HTML scraper with the given per-request
// timeout. Every connection it makes — including ones triggered by
// following a redirect to a different host — is dialed through safeDialer,
// which resolves and validates the target address before connecting (see
// resolveAllowedIP's doc comment for why this exists at the transport level
// rather than only checking the requested URL up front).
func NewCollyScraper(timeout time.Duration) *CollyScraper {
	return &CollyScraper{timeout: timeout, dial: safeDialer(timeout)}
}

// safeDialer returns a DialContext that resolves addr's host via
// resolveAllowedIP and connects only to the address that check approves,
// instead of the address net.Dialer would otherwise resolve and connect to
// unchecked. Wired into the collector's http.Transport so it runs on every
// dial the collector makes for the lifetime of one Scrape call, not just the
// first request — an http.Transport following a redirect issues a new
// dial through the same Transport, so a public-looking URL that 302s to an
// internal address is caught here too.
func safeDialer(timeout time.Duration) dialContextFunc {
	dialer := &net.Dialer{Timeout: timeout}
	return func(ctx context.Context, network, addr string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(addr)
		if err != nil {
			return nil, fmt.Errorf("split host/port %q: %w", addr, err)
		}
		ip, err := resolveAllowedIP(ctx, host, net.DefaultResolver.LookupIPAddr)
		if err != nil {
			return nil, err
		}
		return dialer.DialContext(ctx, network, net.JoinHostPort(ip.String(), port))
	}
}

// Scrape implements domain.Scraper.
func (s *CollyScraper) Scrape(ctx context.Context, rawURL string) (*domain.ScrapedPage, error) {
	c := colly.NewCollector(colly.StdlibContext(ctx), colly.UserAgent(collyUserAgent))
	c.SetRequestTimeout(s.timeout)
	// Cloned from http.DefaultTransport, not a bare &http.Transport{}, so
	// pinning DialContext doesn't also silently drop its other defaults
	// (ProxyFromEnvironment, TLSHandshakeTimeout, HTTP/2 negotiation, idle
	// connection tuning) — only the dial behavior itself is overridden. The
	// type assertion is always true for the stdlib's own http.DefaultTransport
	// (it is a *http.Transport literal); the fallback exists only to satisfy
	// golangci-lint's check-type-assertions rule without a bare panic.
	defaultTransport, ok := http.DefaultTransport.(*http.Transport)
	if !ok {
		defaultTransport = &http.Transport{}
	}
	transport := defaultTransport.Clone()
	transport.DialContext = s.dial
	c.WithTransport(transport)

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
