package llm

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// readFixture loads a golden fixture from test/fixtures/llm.
func readFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "..", "test", "fixtures", "llm", name))
	require.NoError(t, err)
	return data
}

// jsonServer serves a fixed status code and body for every request.
func jsonServer(t *testing.T, status int, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			t.Logf("write fixture response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// sseServer serves a fixed SSE event stream for every request.
func sseServer(t *testing.T, body []byte) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		if _, err := w.Write(body); err != nil {
			t.Logf("write fixture response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// capturingServer serves body with the given status and records the last
// request body it received, so tests can assert on request construction
// (tool schema, forced tool_choice, model) rather than only on response
// decoding.
func capturingServer(t *testing.T, status int, body []byte) (srv *httptest.Server, captured func() []byte) {
	t.Helper()

	var mu sync.Mutex
	var lastBody []byte

	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, err := io.ReadAll(r.Body)
		require.NoError(t, err)
		mu.Lock()
		lastBody = b
		mu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		if _, err := w.Write(body); err != nil {
			t.Logf("write fixture response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)

	return srv, func() []byte {
		mu.Lock()
		defer mu.Unlock()
		return lastBody
	}
}

// testGenerationInput is a valid GenerationInput shared across adapter tests.
func testGenerationInput() domain.GenerationInput {
	return domain.GenerationInput{
		SiteProfile: domain.SiteProfile{
			URL:              "https://example.com",
			Title:            "Acme Analytics",
			ValueProposition: "Real-time analytics for indie SaaS founders",
			Industry:         "developer-tools",
			Audience: domain.Audience{
				Segment:        "SaaS founders",
				Sophistication: domain.SophisticationMedium,
				PricePosition:  domain.PricePositionMidMarket,
			},
			Keywords: []string{"analytics", "saas"},
		},
		Strategy: domain.StrategyValueBased,
		Currency: "USD",
	}
}
