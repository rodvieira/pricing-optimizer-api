package scraper

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func staticHTMLServer(t *testing.T, html string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		if _, err := w.Write([]byte(html)); err != nil {
			t.Logf("write response: %v", err)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

func TestCollyScraper_Scrape(t *testing.T) {
	t.Parallel()

	html := `<html><head><title>  Acme Analytics  </title></head>
<body>
	<script>var secretScriptContent = "must-not-appear";</script>
	<style>.hidden { display: none; } /* must-not-appear-style */</style>
	<h1>Real-time analytics</h1>
	<p>For indie SaaS founders.</p>
</body></html>`

	srv := staticHTMLServer(t, html)
	s := NewCollyScraper(5 * time.Second)

	page, err := s.Scrape(context.Background(), srv.URL)

	require.NoError(t, err)
	assert.Equal(t, srv.URL, page.URL)
	assert.Equal(t, "Acme Analytics", page.Title)
	assert.Contains(t, page.Text, "Real-time analytics")
	assert.Contains(t, page.Text, "For indie SaaS founders.")
	assert.NotContains(t, page.Text, "must-not-appear")
	assert.Equal(t, domain.SourceTypeStatic, page.SourceType)
}

func TestCollyScraper_Scrape_UnreachableHost(t *testing.T) {
	t.Parallel()

	s := NewCollyScraper(2 * time.Second)

	// Port 1 is never listening.
	_, err := s.Scrape(context.Background(), "http://127.0.0.1:1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "colly")
}

func TestCollyScraper_Scrape_EmptyBody(t *testing.T) {
	t.Parallel()

	srv := staticHTMLServer(t, `<html><head><title>Empty</title></head><body></body></html>`)
	s := NewCollyScraper(5 * time.Second)

	page, err := s.Scrape(context.Background(), srv.URL)

	require.NoError(t, err)
	assert.Equal(t, "Empty", page.Title)
	assert.Empty(t, page.Text)
}
