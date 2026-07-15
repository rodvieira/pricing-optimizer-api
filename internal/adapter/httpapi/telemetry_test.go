package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.opentelemetry.io/otel/sdk/trace/tracetest"
	"go.opentelemetry.io/otel/trace/noop"
)

// withRecordingTracerProvider installs a real TracerProvider backed by an
// in-memory span recorder for the duration of the test, restoring the noop
// default (this package's tests otherwise run with no tracing configured,
// same as production when OTEL_EXPORTER_OTLP_ENDPOINT is unset) afterward.
func withRecordingTracerProvider(t *testing.T) *tracetest.SpanRecorder {
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

func TestNewRouter_CreatesOneSpanPerRequest(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	recorder := withRecordingTracerProvider(t)

	router := NewRouter(NewServer(nil, nil, nil, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	spans := recorder.Ended()
	require.Len(t, spans, 1, "one request must produce exactly one otelhttp span")
	assert.Equal(t, "GET /v1/healthz", spans[0].Name())
	assert.True(t, spans[0].SpanContext().IsValid())
}

func TestWriteProblem_UsesRealTraceIDWhenASpanIsActive(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	withRecordingTracerProvider(t)

	tracer := otel.Tracer("test")
	ctx, span := tracer.Start(context.Background(), "test-span")
	defer span.End()

	req := httptest.NewRequestWithContext(ctx, http.MethodGet, "/v1/analyze", nil)
	rec := httptest.NewRecorder()

	writeProblem(rec, req, http.StatusBadRequest, "malformed request body", "")

	var problem map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	assert.Equal(t, span.SpanContext().TraceID().String(), problem["traceId"],
		"traceId must be the real OTel trace id when a span is active")
}

func TestWriteProblem_FallsBackToChiRequestIDWithoutASpan(t *testing.T) {
	t.Parallel()

	// No tracer installed beyond this package's default (noop), so
	// SpanContextFromContext(r.Context()) is invalid and writeProblem must
	// fall back to chi's request id — exercised through the real router so
	// the RequestID middleware actually runs, rather than faking its value.
	router := NewRouter(NewServer(nil, nil, nil, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString("not json"))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusBadRequest, rec.Code)
	var problem map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
	traceID, ok := problem["traceId"].(string)
	require.True(t, ok, "traceId must still be set, from chi's request id, without a real span")
	assert.NotEmpty(t, traceID)
}
