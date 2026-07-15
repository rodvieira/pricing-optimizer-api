// Package httpapi contains the Chi HTTP adapter: router, middleware, and handlers.
package httpapi

import (
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5/middleware"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
)

// writeJSON writes v as a 200 OK JSON response. Every handler that succeeds
// synchronously (the SSE stream in generate.go is the one exception) reports
// success this way, so there is no separate status parameter to get wrong.
func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		slog.Error("encode json response", "error", err)
	}
}

// writeProblem writes an RFC 7807 problem+json response. traceId is taken
// from chi's request id middleware as a stand-in correlation id until real
// OpenTelemetry trace_id propagation lands (issue #4, Sprint 6).
func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	problem := api.Problem{
		Status: status,
		Title:  title,
	}
	if detail != "" {
		problem.Detail = &detail
	}
	if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		problem.TraceId = &reqID
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		slog.Error("encode problem response", "error", err)
	}
}
