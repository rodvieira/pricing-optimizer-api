package telemetry

import (
	"fmt"
	"log/slog"

	"github.com/getsentry/sentry-go"
)

// InitSentry installs the global Sentry client per dsn, gated the same way
// Init gates OTel export (see ADR-0007): empty means a documented no-op —
// every sentry.CaptureException/CaptureMessage call still works, it just
// has nothing configured to send to, so it silently does nothing rather
// than erroring.
//
// Uses sentry.NewHTTPSyncTransport(), not the SDK's default background
// worker: ADR-0014 found the hard way that Cloud Run only allocates CPU
// while a request is in flight (min-instances=0), so anything relying on a
// background goroutine to flush after the handler returns can silently
// never send. The synchronous transport sends each event inline, still
// inside the request's own CPU-allocated window — the same fix already
// applied to OTel span export, applied here proactively instead of waiting
// to rediscover the same bug class.
func InitSentry(dsn, environment, release string) error {
	if dsn == "" {
		slog.Info("sentry disabled: no DSN configured")
		return nil
	}
	slog.Info("sentry enabled: reporting errors")

	err := sentry.Init(sentry.ClientOptions{
		Dsn:         dsn,
		Environment: environment,
		Release:     release,
		Transport:   sentry.NewHTTPSyncTransport(),
		// Tracing is OTel/Grafana Cloud's job (ADR-0007) — Sentry here is
		// error tracking only, not a second, overlapping tracing backend.
		EnableTracing: false,
	})
	if err != nil {
		return fmt.Errorf("telemetry: init sentry: %w", err)
	}
	return nil
}
