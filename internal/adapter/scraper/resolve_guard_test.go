package scraper

import (
	"context"
	"errors"
	"net"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func fakeLookup(table map[string][]net.IPAddr, err error) lookupIPAddrFunc {
	return func(_ context.Context, host string) ([]net.IPAddr, error) {
		if err != nil {
			return nil, err
		}
		return table[host], nil
	}
}

func ipAddrs(ips ...string) []net.IPAddr {
	out := make([]net.IPAddr, len(ips))
	for i, s := range ips {
		out[i] = net.IPAddr{IP: net.ParseIP(s)}
	}
	return out
}

func TestResolveAllowedIP(t *testing.T) {
	t.Parallel()

	errLookup := errors.New("dns down")

	tests := []struct {
		name    string
		host    string
		lookup  lookupIPAddrFunc
		want    string
		wantErr bool
	}{
		{
			name: "a public IP literal is allowed without any lookup",
			host: "8.8.8.8",
			lookup: func(context.Context, string) ([]net.IPAddr, error) {
				t.Fatal("must not resolve an IP literal")
				return nil, nil
			},
			want: "8.8.8.8",
		},
		{
			name:    "a loopback IP literal is rejected",
			host:    "127.0.0.1",
			lookup:  fakeLookup(nil, nil),
			wantErr: true,
		},
		{
			name:    "a private-range IP literal is rejected",
			host:    "10.0.0.5",
			lookup:  fakeLookup(nil, nil),
			wantErr: true,
		},
		{
			name:    "the cloud metadata link-local IP literal is rejected",
			host:    "169.254.169.254",
			lookup:  fakeLookup(nil, nil),
			wantErr: true,
		},
		{
			name:   "a hostname resolving only to a public address is allowed",
			host:   "public.example.com",
			lookup: fakeLookup(map[string][]net.IPAddr{"public.example.com": ipAddrs("93.184.216.34")}, nil),
			want:   "93.184.216.34",
		},
		{
			name: "a hostname resolving only to a private address is rejected (DNS rebinding)",
			host: "rebind.example.com",
			lookup: fakeLookup(
				map[string][]net.IPAddr{"rebind.example.com": ipAddrs("169.254.169.254")}, nil,
			),
			wantErr: true,
		},
		{
			name: "a hostname resolving to a mix of addresses returns the first publicly reachable one",
			host: "mixed.example.com",
			lookup: fakeLookup(
				map[string][]net.IPAddr{"mixed.example.com": ipAddrs("10.0.0.1", "93.184.216.34")}, nil,
			),
			want: "93.184.216.34",
		},
		{
			name:    "a resolution failure is wrapped and reported",
			host:    "broken.example.com",
			lookup:  fakeLookup(nil, errLookup),
			wantErr: true,
		},
		{
			name:    "a hostname with no addresses at all is rejected",
			host:    "empty.example.com",
			lookup:  fakeLookup(map[string][]net.IPAddr{}, nil),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := resolveAllowedIP(context.Background(), tt.host, tt.lookup)

			if tt.wantErr {
				require.Error(t, err)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.want, got.String())
		})
	}
}
