package telemetry_test

import (
	"testing"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/telemetry"
)

func TestInitSentry_NoDSNIsANoop(t *testing.T) {
	// Not t.Parallel(): sentry.Init mutates the process-global CurrentHub,
	// same global-state reason as telemetry.Init's own tests.
	err := telemetry.InitSentry("", "test", "test")
	require.NoError(t, err)

	// A safe no-op: capturing without a configured client must not panic
	// or block, and (with no client at all) has nothing to report to, so
	// there is no event id.
	assert.Nil(t, sentry.CaptureException(assertErr{}))
}

func TestInitSentry_WithDSNInstallsARealClient(t *testing.T) {
	// Not t.Parallel(): same global-state reason as above.
	err := telemetry.InitSentry("https://public@example.com/1", "test", "v-test")
	require.NoError(t, err)
	t.Cleanup(func() {
		sentry.CurrentHub().BindClient(nil)
	})

	client := sentry.CurrentHub().Client()
	require.NotNil(t, client, "a configured DSN must install a real client")
	assert.Equal(t, "test", client.Options().Environment)
	assert.Equal(t, "v-test", client.Options().Release)
	// EnableTracing: false — Grafana Cloud/OTel (ADR-0007) owns tracing;
	// Sentry here is error tracking only, not a second overlapping tracer.
	assert.False(t, client.Options().EnableTracing)
}

func TestInitSentry_UsesASynchronousTransport(t *testing.T) {
	// Not t.Parallel(): same global-state reason as above.
	err := telemetry.InitSentry("https://public@example.com/1", "test", "v-test")
	require.NoError(t, err)
	t.Cleanup(func() {
		sentry.CurrentHub().BindClient(nil)
	})

	// ADR-0014: Cloud Run only allocates CPU while a request is in flight,
	// so anything relying on a background flush can silently never send.
	// HTTPSyncTransport.Flush is a documented no-op that always returns
	// true immediately — the only way to actually confirm the sync
	// transport is wired is checking the concrete type, since behaviorally
	// a fast Flush(0) would look the same for either transport in a test
	// this short.
	transport := sentry.CurrentHub().Client().Options().Transport
	_, isSync := transport.(*sentry.HTTPSyncTransport)
	assert.True(t, isSync, "must use HTTPSyncTransport, not the default background HTTPTransport")
	assert.True(t, transport.Flush(time.Millisecond))
}

type assertErr struct{}

func (assertErr) Error() string { return "assert-only test error" }
