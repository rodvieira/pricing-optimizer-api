package scraper

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// chromePath resolves a real Chrome/Chromium binary for tests to drive, or
// skips the test if none is installed. chromedp needs a real browser
// process; there is no meaningful fake for it.
func chromePath(t *testing.T) string {
	t.Helper()
	for _, name := range []string{"google-chrome-stable", "google-chrome", "chromium", "chromium-browser"} {
		if path, err := exec.LookPath(name); err == nil {
			return path
		}
	}
	t.Skip("no Chrome/Chromium binary found; install one to run chromedp tests")
	return ""
}

func TestChromedpScraper_Scrape(t *testing.T) {
	t.Parallel()

	execPath := chromePath(t)

	html := `<html><head><title>  Acme Analytics  </title></head>
<body><h1>Real-time analytics</h1><p>For indie SaaS founders.</p></body></html>`
	srv := staticHTMLServer(t, html)

	s := NewChromedpScraper(execPath, 20*time.Second)
	page, err := s.Scrape(context.Background(), srv.URL)

	require.NoError(t, err)
	assert.Equal(t, srv.URL, page.URL)
	assert.Equal(t, "Acme Analytics", page.Title)
	assert.Contains(t, page.Text, "Real-time analytics")
	assert.Contains(t, page.Text, "For indie SaaS founders.")
	assert.Equal(t, domain.SourceTypeSPA, page.SourceType)
}

func TestChromedpScraper_Scrape_RendersClientSideContent(t *testing.T) {
	t.Parallel()

	execPath := chromePath(t)

	// The initial HTML is empty; content is injected by JavaScript after
	// load, as in a client-rendered SPA. A static fetch (colly) would only
	// ever see the empty <div id="root">. This is chromedp's entire reason
	// for existing in this adapter, so it is asserted directly, not assumed.
	html := `<html><head><title>SPA Shell</title></head>
<body>
	<div id="root"></div>
	<script>
		document.getElementById('root').innerText = 'Rendered by client-side JavaScript';
	</script>
</body></html>`
	srv := staticHTMLServer(t, html)

	s := NewChromedpScraper(execPath, 20*time.Second)
	page, err := s.Scrape(context.Background(), srv.URL)

	require.NoError(t, err)
	assert.Contains(t, page.Text, "Rendered by client-side JavaScript")
}

func TestChromedpScraper_Scrape_InvalidExecPath(t *testing.T) {
	t.Parallel()

	s := NewChromedpScraper("/nonexistent/chrome-binary", 5*time.Second)
	_, err := s.Scrape(context.Background(), "http://127.0.0.1:1")

	require.Error(t, err)
	assert.Contains(t, err.Error(), "chromedp")
}
