package domain_test

import (
	"net"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func TestIsDisallowedIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		ip   string
		want bool
	}{
		{name: "IPv4 loopback is disallowed", ip: "127.0.0.1", want: true},
		{name: "IPv6 loopback is disallowed", ip: "::1", want: true},
		{name: "IPv4 private range 10.0.0.0/8 is disallowed", ip: "10.0.0.5", want: true},
		{name: "IPv4 private range 172.16.0.0/12 is disallowed", ip: "172.16.4.4", want: true},
		{name: "IPv4 private range 192.168.0.0/16 is disallowed", ip: "192.168.1.1", want: true},
		{name: "IPv4 link-local (cloud metadata endpoint) is disallowed", ip: "169.254.169.254", want: true},
		{name: "IPv6 unique local (fc00::/7) is disallowed", ip: "fd00::1", want: true},
		{name: "IPv6 link-local (fe80::/10) is disallowed", ip: "fe80::1", want: true},
		{name: "IPv4 unspecified is disallowed", ip: "0.0.0.0", want: true},
		{name: "IPv6 unspecified is disallowed", ip: "::", want: true},
		{name: "IPv4 link-local multicast is disallowed", ip: "224.0.0.1", want: true},
		{name: "a public IPv4 address is allowed", ip: "8.8.8.8", want: false},
		{name: "a public IPv6 address is allowed", ip: "2001:4860:4860::8888", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ip := net.ParseIP(tt.ip)
			assert.Equal(t, tt.want, domain.IsDisallowedIP(ip), "IsDisallowedIP(%s)", tt.ip)
		})
	}
}
