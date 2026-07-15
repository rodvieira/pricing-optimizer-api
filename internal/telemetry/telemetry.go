// Package telemetry wires the OpenTelemetry SDK: a TracerProvider exporting
// spans via OTLP/HTTP, and a slog.Handler that stamps every log line with
// the active span's trace_id/span_id. See ADR-0007 for why exporting is
// gated behind an explicit endpoint rather than always attempting one.
package telemetry

import (
	"context"
	"fmt"

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
	if cfg.Endpoint == "" {
		otel.SetTracerProvider(noop.NewTracerProvider())
		return func(context.Context) error { return nil }, nil
	}

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
