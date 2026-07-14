package llm

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
