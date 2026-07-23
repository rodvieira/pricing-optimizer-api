package httpapi

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
)

// Regression: this API has no reverse-proxy topology — the frontend is
// always a separate origin (a separate Next.js repo/deploy) in every
// environment, so a missing CORS header is a real cross-origin failure, not
// just a local-dev inconvenience. Every prior session validated the Studio
// flow only via Playwright route-interception mocks or a deliberately
// unreachable backend, so a live-browser cross-origin request was never
// actually exercised before this was caught.
func TestCORS(t *testing.T) {
	t.Parallel()

	router := NewRouter(NewServer(nil, nil, nil, nil, nil, nil, nil), []string{"http://localhost:3000"})

	t.Run("allowed origin gets Access-Control-Allow-Origin on a simple request", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/healthz", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("disallowed origin gets no Access-Control-Allow-Origin", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/healthz", nil)
		req.Header.Set("Origin", "https://evil.example.com")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		assert.Empty(t, rec.Header().Get("Access-Control-Allow-Origin"))
	})

	t.Run("preflight OPTIONS for POST /v1/generate is allowed for the configured origin", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(context.Background(), http.MethodOptions, "/v1/generate", nil)
		req.Header.Set("Origin", "http://localhost:3000")
		req.Header.Set("Access-Control-Request-Method", http.MethodPost)
		req.Header.Set("Access-Control-Request-Headers", "content-type,idempotency-key")
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
		assert.Equal(t, "http://localhost:3000", rec.Header().Get("Access-Control-Allow-Origin"))
		assert.Contains(t, rec.Header().Get("Access-Control-Allow-Methods"), http.MethodPost)
	})

	t.Run("no Origin header (same-origin or non-browser client) is unaffected", func(t *testing.T) {
		t.Parallel()

		req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/healthz", nil)
		rec := httptest.NewRecorder()

		router.ServeHTTP(rec, req)

		require.Equal(t, http.StatusOK, rec.Code)
	})
}

// rateLimitedRequest builds a POST /v1/analyze request wired to a rate
// limiter mock that expects exactly wantKey as its Allow key (proving the
// client IP actually flowing through router middleware -> checkRateLimit ->
// Allow is wantKey, not just that some key was passed) and always denies —
// analyzer stays nil, so a request that ever reached past the rate limit
// check would panic calling Execute on it, which doubles as proof it didn't.
func rateLimitedRequest(t *testing.T, opts []RouterOption, wantKey, remoteAddr, xff string) *httptest.ResponseRecorder {
	t.Helper()

	ctrl := gomock.NewController(t)
	limiter := mockhttpapi.NewMockrateLimiter(ctrl)
	limiter.EXPECT().Allow(gomock.Any(), wantKey).Return(false, time.Second, nil)

	router := NewRouter(NewServer(nil, nil, nil, nil, limiter, nil, nil), []string{"http://localhost:3000"}, opts...)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	req.RemoteAddr = remoteAddr
	if xff != "" {
		req.Header.Set("X-Forwarded-For", xff)
	}
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	return rec
}

// TestClientIP is a regression test for the rate limiter silently becoming
// a near-global budget instead of a per-caller one once this API sat behind
// Cloud Run's reverse proxy: RemoteAddr there is always the proxy's own
// address, never the caller's, and a naive switch to trusting
// X-Forwarded-For outright would let any caller spoof a different key and
// dodge the limit instead.
func TestClientIP(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		opts       []RouterOption
		remoteAddr string
		xff        string
		wantKey    string
	}{
		{
			name:       "defaults to RemoteAddr and ignores a client-supplied X-Forwarded-For",
			remoteAddr: "203.0.113.7:54321",
			xff:        "198.51.100.9",
			wantKey:    "203.0.113.7",
		},
		{
			name:       "one trusted proxy hop uses the trusted XFF entry, not RemoteAddr",
			opts:       []RouterOption{WithTrustedProxyHops(1)},
			remoteAddr: "10.0.0.1:443",
			xff:        "203.0.113.7",
			wantKey:    "203.0.113.7",
		},
		{
			name:       "one trusted proxy hop ignores a client-spoofed earlier XFF entry",
			opts:       []RouterOption{WithTrustedProxyHops(1)},
			remoteAddr: "10.0.0.1:443",
			xff:        "198.51.100.9, 203.0.113.7",
			wantKey:    "203.0.113.7",
		},
		{
			// A two-trusted-proxy chain: the client (or an untrusted hop)
			// contributes the leftmost entry, the outermost trusted proxy
			// appends the real caller's address next, and the innermost
			// trusted proxy (closest to this server) appends its own
			// address last. WithTrustedProxyHops(2) must select the middle
			// entry — the outermost trusted proxy's observation — not the
			// last one, which is just the other trusted proxy's own address.
			name:       "two trusted proxy hops selects the interior trusted entry, not the last",
			opts:       []RouterOption{WithTrustedProxyHops(2)},
			remoteAddr: "10.0.0.1:443",
			xff:        "198.51.100.9, 203.0.113.7, 10.0.0.5",
			wantKey:    "203.0.113.7",
		},
		{
			name:       "one trusted proxy hop normalizes a v4-mapped IPv6 XFF entry",
			opts:       []RouterOption{WithTrustedProxyHops(1)},
			remoteAddr: "10.0.0.1:443",
			xff:        "::ffff:203.0.113.7",
			wantKey:    "203.0.113.7",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			rec := rateLimitedRequest(t, tt.opts, tt.wantKey, tt.remoteAddr, tt.xff)

			assert.Equal(t, http.StatusTooManyRequests, rec.Code)
		})
	}
}
