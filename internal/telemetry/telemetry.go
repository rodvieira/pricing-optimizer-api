// Package telemetry wires the OpenTelemetry SDK: a TracerProvider exporting
// spans via OTLP/HTTP, and a slog.Handler that stamps every log line with
// the active span's trace_id/span_id. See ADR-0007 for why exporting is
// gated behind an explicit endpoint rather than always attempting one.
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

	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(exporter),
		sdktrace.WithResource(res),
	)
	otel.SetTracerProvider(tp)

	return tp.Shutdown, nil
}
