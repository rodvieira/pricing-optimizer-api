package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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
