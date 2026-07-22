package httpapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeSentryTransport records events in memory instead of sending them
// anywhere, so tests can assert on what writeProblem reported without a
// real Sentry project.
type fakeSentryTransport struct {
	mu     sync.Mutex
	events []*sentry.Event
}

func (t *fakeSentryTransport) Configure(sentry.ClientOptions)        {}
func (t *fakeSentryTransport) Close()                                {}
func (t *fakeSentryTransport) Flush(time.Duration) bool              { return true }
func (t *fakeSentryTransport) FlushWithContext(context.Context) bool { return true }
func (t *fakeSentryTransport) SendEvent(event *sentry.Event) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.events = append(t.events, event)
}

func (t *fakeSentryTransport) captured() []*sentry.Event {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.events
}

// initFakeSentry installs a real Sentry client backed by fakeSentryTransport
// as the process-global CurrentHub's client, and unbinds it on test
// cleanup. Not safe to run in parallel with other tests that touch the
// global Sentry hub — matches internal/telemetry's own tests for the same
// process-global-state reason.
func initFakeSentry(t *testing.T) *fakeSentryTransport {
	t.Helper()
	transport := &fakeSentryTransport{}
	err := sentry.Init(sentry.ClientOptions{
		Dsn:       "https://public@example.com/1",
		Transport: transport,
	})
	require.NoError(t, err)
	t.Cleanup(func() {
		sentry.CurrentHub().BindClient(nil)
	})
	return transport
}

func TestWriteProblem_ReportsServerErrorsToSentry(t *testing.T) {
	// Not t.Parallel(): mutates the process-global Sentry hub.
	transport := initFakeSentry(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze", nil)
	rec := httptest.NewRecorder()

	writeProblem(rec, req, http.StatusInternalServerError, "could not analyze the site", "scrape timed out")

	events := transport.captured()
	require.Len(t, events, 1, "a 5xx problem response must report exactly one Sentry event")
	event := events[0]
	require.NotEmpty(t, event.Exception, "must be captured as an exception, not a bare message")
	assert.Equal(t, "could not analyze the site: scrape timed out", event.Exception[0].Value)
	assert.Equal(t, "POST", event.Tags["http.method"])
	assert.Equal(t, "/v1/analyze", event.Tags["http.path"])
	assert.Equal(t, "500", event.Tags["http.status"])
}

func TestWriteProblem_DoesNotReportClientErrorsToSentry(t *testing.T) {
	// Not t.Parallel(): mutates the process-global Sentry hub.
	transport := initFakeSentry(t)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze", nil)
	rec := httptest.NewRecorder()

	writeProblem(rec, req, http.StatusUnprocessableEntity, "invalid analyze request", "url is required")

	assert.Empty(t, transport.captured(),
		"a 4xx problem response is expected client traffic, not a bug — must not spam Sentry")
}

func TestWriteProblem_SentryDisabledIsANoop(t *testing.T) {
	// Not t.Parallel(): mutates the process-global Sentry hub.
	sentry.CurrentHub().BindClient(nil)

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze", nil)
	rec := httptest.NewRecorder()

	assert.NotPanics(t, func() {
		writeProblem(rec, req, http.StatusInternalServerError, "could not analyze the site", "")
	})
	assert.Equal(t, http.StatusInternalServerError, rec.Code, "the actual HTTP response must still be written")
}
