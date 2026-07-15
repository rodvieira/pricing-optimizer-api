package telemetry_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"

	"github.com/rodvieira/pricing-optimizer-api/internal/telemetry"
)

func TestSlogHandler_AddsTraceIDWhenSpanIsActive(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(telemetry.NewSlogHandler(slog.NewJSONHandler(&buf, nil)))

	// A TracerProvider with no exporter configured still produces real,
	// non-zero span contexts (spans are just dropped on End, not
	// suppressed at creation) — enough to exercise the handler without
	// needing a live OTLP collector.
	tracer := sdktrace.NewTracerProvider().Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	logger.InfoContext(ctx, "hello")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	assert.Equal(t, span.SpanContext().TraceID().String(), record["trace_id"])
	assert.Equal(t, span.SpanContext().SpanID().String(), record["span_id"])
	assert.Equal(t, "hello", record["msg"])
}

func TestSlogHandler_OmitsTraceIDWithoutASpan(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(telemetry.NewSlogHandler(slog.NewJSONHandler(&buf, nil)))

	logger.InfoContext(context.Background(), "hello")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	assert.NotContains(t, record, "trace_id")
	assert.NotContains(t, record, "span_id")
}

func TestSlogHandler_WithAttrsAndWithGroupDelegate(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(telemetry.NewSlogHandler(slog.NewJSONHandler(&buf, nil)))
	logger = logger.With("service", "api").WithGroup("request")

	logger.InfoContext(context.Background(), "hello", "path", "/v1/healthz")

	var record map[string]any
	require.NoError(t, json.Unmarshal(buf.Bytes(), &record))
	assert.Equal(t, "api", record["service"])
	group, ok := record["request"].(map[string]any)
	require.True(t, ok, "grouped attribute must nest under \"request\"")
	assert.Equal(t, "/v1/healthz", group["path"])
}
