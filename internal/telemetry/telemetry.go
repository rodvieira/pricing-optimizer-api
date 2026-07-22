// Package telemetry wires the OpenTelemetry SDK: a TracerProvider exporting
// spans via OTLP/HTTP, and a slog.Handler that stamps every log line with
// the active span's trace_id/span_id. See ADR-0007 for why exporting is
// gated behind an explicit endpoint rather than always attempting one, and
// ADR-0014 for why spans are exported synchronously (WithSyncer) rather
// than batched.
package telemetry

import (
	"context"
	"fmt"
	"log/slog"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracehttp"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace/noop"
)

// Config configures the SDK. Endpoint is deliberately just a presence
// check: when set, otlptracehttp.New reads the full OTEL_EXPORTER_OTLP_*
// env var family itself (headers, compression, TLS, ...), so nothing here
// duplicates that parsing. When empty (no collector/Grafana Cloud
// provisioned yet — that's Sprint 7), tracing is a genuine no-op rather
// than a batch exporter perpetually failing to reach localhost:4318.
type Config struct {
	ServiceName    string
	ServiceVersion string
	Endpoint       string
}

// Init installs the global TracerProvider per cfg and returns a shutdown
// func to flush pending spans and release exporter resources on exit.
func Init(ctx context.Context, cfg Config) (shutdown func(context.Context) error, err error) {
	// The signal-specific OTEL_EXPORTER_OTLP_TRACES_ENDPOINT is a legitimate
	// way to configure the exporter without setting the general
	// OTEL_EXPORTER_OTLP_ENDPOINT cfg.Endpoint reads; check it too so the
	// on/off gate agrees with what otlptracehttp itself considers "configured".
	if cfg.Endpoint == "" && os.Getenv("OTEL_EXPORTER_OTLP_TRACES_ENDPOINT") == "" {
		slog.Info("telemetry disabled: no OTLP endpoint configured")
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}
	slog.Info("telemetry enabled: exporting via OTLP/HTTP")

	res, err := resource.New(ctx,
		resource.WithAttributes(
			semconv.ServiceName(cfg.ServiceName),
			semconv.ServiceVersion(cfg.ServiceVersion),
		),
		resource.WithFromEnv(),
		resource.WithTelemetrySDK(),
	)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build resource: %w", err)
	}

	exporter, err := otlptracehttp.New(ctx)
	if err != nil {
		return nil, fmt.Errorf("telemetry: build otlp exporter: %w", err)
	}

	// WithSyncer, not WithBatcher: Cloud Run only allocates CPU while a
	// request is in flight (min-instances=0, CPU not always allocated —
	// see ADR-0009). A BatchSpanProcessor's periodic background flush can
	// easily never get scheduled before the instance freezes right after
	// the response is written, silently dropping every span. Confirmed for
	// real, not just theorized: after wiring Grafana Cloud (ADR/issue #39),
	// a live request against production returned 200 and the service's own
	// "telemetry enabled" log line confirmed the exporter was active, but
	// no trace ever reached Grafana. WithSyncer's per-export latency is a
	// non-issue at this project's traffic volume, and every request already
	// takes seconds (LLM calls dominate), so a synchronous HTTP POST to the
	// OTLP endpoint is not a meaningfully worse user-facing cost. See
	// ADR-0014 for the fix's own verification status.
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithSyncer(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
