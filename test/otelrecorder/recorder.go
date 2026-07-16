// Package otelrecorder gives tests a real OpenTelemetry TracerProvider
// backed by an in-memory span recorder, plus a small assertion helper for
// span attributes, so span-shape tests across packages (httpapi, llm,
// repository) don't each hand-roll the same setup/teardown.
package otelrecorder

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// WithRecordingTracerProvider installs a real TracerProvider backed by an
// in-memory span recorder for the duration of the test, restoring the noop
// default (production's behavior when OTEL_EXPORTER_OTLP_ENDPOINT is unset)
// afterward. Callers must not run in parallel with other tests using this
// helper: it mutates the process-global TracerProvider.
func WithRecordingTracerProvider(t *testing.T) *tracetest.SpanRecorder {
	t.Helper()
	recorder := tracetest.NewSpanRecorder()
	tp := sdktrace.NewTracerProvider(sdktrace.WithSpanProcessor(recorder))
	otel.SetTracerProvider(tp)
	t.Cleanup(func() {
		require.NoError(t, tp.Shutdown(context.Background()))
		otel.SetTracerProvider(noop.NewTracerProvider())
	})
	return recorder
}

// AssertHasAttribute fails the test if span does not carry a string
// attribute key with value want.
func AssertHasAttribute(t *testing.T, span sdktrace.ReadOnlySpan, key, want string) {
	t.Helper()
	for _, kv := range span.Attributes() {
		if string(kv.Key) == key {
			assert.Equal(t, want, kv.Value.AsString())
			return
		}
	}
	t.Errorf("span %q missing attribute %q", span.Name(), key)
}
