package domain

import "net"

// IsDisallowedIP reports whether ip is a loopback, private, link-local, or
// unspecified address that a public "analyze this URL" endpoint — and
// anything it hands off to a Scraper — must never connect to. Link-local
// includes 169.254.169.254, the address cloud metadata endpoints classically
// live at (the target of the SSRF pattern this exists to block).
//
// This is the single source of truth for that policy: usecase's
// validateAnalyzeURL uses it for the cheap up-front rejection of an
// IP-literal URL, and the scraper adapters use it again at actual connection
// time (see adapter/scraper's resolveAllowedIP) to close the DNS-rebinding
// gap a hostname-only check at request-validation time can't close on its
// own — a hostname that resolves to a public address when validated can
// legitimately resolve to a different, disallowed one moments later when
// the scraper actually connects.
func IsDisallowedIP(ip net.IP) bool {
	return ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() ||
		ip.IsLinkLocalMulticast() || ip.IsUnspecified()
}
