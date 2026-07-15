// Package httpapi contains the Chi HTTP adapter: router, middleware, and handlers.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/otel/trace"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
)

// writeJSON writes v as a 200 OK JSON response. Every handler that succeeds
// synchronously (the SSE stream in generate.go is the one exception) reports
// success this way, so there is no separate status parameter to get wrong.
func writeJSON(w http.ResponseWriter, r *http.Request, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.ErrorContext(r.Context(), "encode json response", "error", err)
	}
}

// writeProblem writes an RFC 7807 problem+json response. traceId prefers
// the real OpenTelemetry trace id from r's active span (set by the
// otelhttp.NewHandler wrapping the whole router); when tracing is disabled
// (telemetry.Init installed the noop TracerProvider, so the span context is
// invalid) it falls back to chi's request id middleware as a correlation id
// that's still unique per request, just not an OTel trace_id.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	problem := api.Problem{
		Status: status,
		Title:  title,
	}
	if detail != "" {
		problem.Detail = &detail
	}
	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		traceID := sc.TraceID().String()
		problem.TraceId = &traceID
	} else if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		problem.TraceId = &reqID
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		slog.ErrorContext(r.Context(), "encode problem response", "error", err)
	}
}
