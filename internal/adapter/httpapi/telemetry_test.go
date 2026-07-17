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
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
	"github.com/rodvieira/pricing-optimizer-api/test/otelrecorder"
)

func TestNewRouter_CreatesOneSpanPerRequest(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	recorder := otelrecorder.WithRecordingTracerProvider(t)

	router := NewRouter(NewServer(nil, nil, nil, nil, nil, nil, nil), []string{"http://localhost:3000"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/healthz", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	spans := recorder.Ended()
	require.Len(t, spans, 1, "one request must produce exactly one otelhttp span")
	assert.Equal(t, "GET /v1/healthz", spans[0].Name())
	assert.True(t, spans[0].SpanContext().IsValid())
}

func TestNewRouter_RenamesSpanToLowCardinalityRoutePattern(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	recorder := otelrecorder.WithRecordingTracerProvider(t)

	ctrl := gomock.NewController(t)
	mockGetter := mockhttpapi.NewMockgenerationGetter(ctrl)
	mockGetter.EXPECT().Get(gomock.Any(), gomock.Any()).Return(nil, domain.ErrGenerationNotFound)

	router := NewRouter(NewServer(nil, nil, mockGetter, nil, nil, nil, nil), []string{"http://localhost:3000"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodGet,
		"/v1/generations/4f9e1c2b-1111-1111-1111-111111111111", nil)
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusNotFound, rec.Code)
	spans := recorder.Ended()
	require.Len(t, spans, 1)
	assert.Equal(t, "GET /v1/generations/{id}", spans[0].Name(),
		"the span must be renamed to the route template, not the raw per-id path")
}

func TestWriteProblem_UsesRealTraceIDWhenASpanIsActive(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	otelrecorder.WithRecordingTracerProvider(t)

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
	router := NewRouter(NewServer(nil, nil, nil, nil, nil, nil, nil), []string{"http://localhost:3000"})

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
