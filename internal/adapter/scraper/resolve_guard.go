package scraper

import (
	"context"
	"fmt"
	"net"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// lookupIPAddrFunc matches net.Resolver.LookupIPAddr's signature, narrowed to
// what resolveAllowedIP needs so tests can inject a fake resolution table
// instead of hitting real DNS. Production always passes
// net.DefaultResolver.LookupIPAddr.
type lookupIPAddrFunc func(ctx context.Context, host string) ([]net.IPAddr, error)

// resolveAllowedIP resolves host (an IP literal is returned as-is, with no
// lookup) and returns the first address that passes domain.IsDisallowedIP,
// or an error if none does. It is what actually closes the SSRF gap
// usecase.validateAnalyzeURL's doc comment describes: that check only
// inspects the URL string once, at request-validation time, before any
// network activity — it cannot know what a hostname will resolve to the
// moment a scraper later connects. resolveAllowedIP runs at that later
// moment instead, in the two places that matter:
//
//   - safeDialer (colly.go) calls it on every dial the collector's transport
//     makes, including ones triggered by following a redirect to a new host
//     — so a public-looking URL that 302s to an internal address is caught
//     too, not just the originally-requested host.
//   - guardHostResolvesToPublicAddress (chromedp.go) calls it once before
//     handing the URL to a real browser process, which does its own DNS
//     resolution chromedp cannot intercept the way the colly path pins the
//     dial address — see that function's doc comment for the residual gap
//     this leaves there.
//
// Deliberately validates every literal or resolved address the same way,
// including IP literals: a redirect's Location header can itself be an IP
// literal, and this function is the only check standing between that and an
// actual connection (validateAnalyzeURL only ever saw the original URL).
func resolveAllowedIP(ctx context.Context, host string, lookup lookupIPAddrFunc) (net.IP, error) {
	if ip := net.ParseIP(host); ip != nil {
		if domain.IsDisallowedIP(ip) {
			return nil, fmt.Errorf("address %s is not a publicly reachable address", ip)
		}
		return ip, nil
	}

	addrs, err := lookup(ctx, host)
	if err != nil {
		return nil, fmt.Errorf("resolve host %q: %w", host, err)
	}
	for _, addr := range addrs {
		if !domain.IsDisallowedIP(addr.IP) {
			return addr.IP, nil
		}
	}
	return nil, fmt.Errorf("host %q resolves only to non-publicly-reachable addresses", host)
}
