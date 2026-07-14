package usecase

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"strings"

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

// Execute scrapes rawURL and classifies the result into a SiteProfile.
func (uc *AnalyzeSite) Execute(ctx context.Context, rawURL string) (*domain.SiteProfile, error) {
	if err := validateAnalyzeURL(rawURL); err != nil {
		return nil, fmt.Errorf("analyze site: %w", err)
	}

	page, err := uc.scraper.Scrape(ctx, rawURL)
	if err != nil {
		return nil, fmt.Errorf("analyze site: scrape %s: %w: %w", rawURL, domain.ErrSiteUnreachable, err)
	}

	profile, err := uc.provider.ClassifySite(ctx, *page)
	if err != nil {
		return nil, fmt.Errorf("analyze site: classify %s: %w", rawURL, err)
	}
	return profile, nil
}

// validateAnalyzeURL rejects requests that would make the Scraper fetch a
// URL that a public "analyze this URL" endpoint must never be able to reach:
// non-HTTP(S) schemes, and IP-literal or "localhost" hosts pointing at the
// loopback interface, private ranges, or link-local addresses — the last of
// which includes 169.254.169.254, the address cloud metadata-endpoint SSRF
// classically targets.
//
// This deliberately does not resolve arbitrary hostnames: doing so here
// would only check the DNS answer at validation time, not the one the
// scraper's own HTTP client resolves moments later (a DNS-rebinding TOCTOU
// gap). Closing that gap requires pinning the resolved IP at the transport
// level inside the scraper adapters themselves, which is out of scope for
// this use case.
func validateAnalyzeURL(rawURL string) error {
	if rawURL == "" {
		return fmt.Errorf("%w: url is required", domain.ErrInvalidInput)
	}

	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("%w: %q is not a valid url", domain.ErrInvalidInput, rawURL)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("%w: url scheme must be http or https, got %q", domain.ErrInvalidInput, u.Scheme)
	}

	host := u.Hostname()
	if host == "" {
		return fmt.Errorf("%w: url is missing a host", domain.ErrInvalidInput)
	}
	if strings.EqualFold(host, "localhost") || strings.HasSuffix(strings.ToLower(host), ".localhost") {
		return fmt.Errorf("%w: localhost is not an analyzable host", domain.ErrInvalidInput)
	}
	if ip := net.ParseIP(host); ip != nil && isDisallowedIP(ip) {
		return fmt.Errorf("%w: url host %s is not a publicly analyzable address", domain.ErrInvalidInput, host)
	}
	return nil
}

// isDisallowedIP reports whether ip is a loopback, private, link-local, or
// unspecified address that a public "analyze this URL" endpoint must never
// reach.
func isDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
