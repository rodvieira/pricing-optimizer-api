// Package httpapi contains the Chi HTTP adapter: router, middleware, and handlers.
package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/getsentry/sentry-go"
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

// writeJSONBytes writes an already-encoded JSON body as a 200 OK response.
// Used where the caller needs the encoded bytes for its own purposes too
// (analyze.go caches the same bytes it writes), so encoding via writeJSON
// would mean encoding twice.
func writeJSONBytes(w http.ResponseWriter, r *http.Request, data []byte) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if _, err := w.Write(data); err != nil {
		slog.ErrorContext(r.Context(), "write json response", "error", err)
	}
}

// writeProblem writes an RFC 7807 problem+json response. traceId is
// resolved via traceIDFromRequest.
//
// status >= 500 also reports to Sentry (a safe no-op if telemetry.InitSentry
// was never called): these are server-side bugs worth tracking, unlike 4xx,
// which is a client sending something invalid — expected traffic, not an
// error to triage. Call sites only ever have title/detail strings, not the
// original error value, so this constructs one from them; Sentry still
// captures a real stack trace at this call site regardless, which is what
// actually matters for triage.
func writeProblem(w http.ResponseWriter, r *http.Request, status int, title, detail string) {
	problem := api.Problem{
		Status: status,
		Title:  title,
	}
	if detail != "" {
		problem.Detail = &detail
	}
	problem.TraceId = traceIDFromRequest(r)

	if status >= http.StatusInternalServerError {
		reportToSentry(r, status, title, detail, problem.TraceId)
	}

	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(problem); err != nil {
		slog.ErrorContext(r.Context(), "encode problem response", "error", err)
	}
}

// traceIDFromRequest prefers the real OpenTelemetry trace id from r's active
// span (set by the otelhttp.NewHandler wrapping the whole router); when
// tracing is disabled (telemetry.Init installed the noop TracerProvider, so
// the span context is invalid) it falls back to chi's request id middleware
// as a correlation id that's still unique per request, just not an OTel
// trace_id. Shared by writeProblem and generate.go's in-stream Sentry
// reporting — both need to tie a Sentry event back to the same trace a
// request's OTel spans (in Grafana Cloud) and slog lines already carry.
func traceIDFromRequest(r *http.Request) *string {
	if sc := trace.SpanContextFromContext(r.Context()); sc.IsValid() {
		traceID := sc.TraceID().String()
		return &traceID
	}
	if reqID := middleware.GetReqID(r.Context()); reqID != "" {
		return &reqID
	}
	return nil
}

func reportToSentry(r *http.Request, status int, title, detail string, traceID *string) {
	msg := title
	if detail != "" {
		msg = title + ": " + detail
	}
	hub := sentry.GetHubFromContext(r.Context())
	if hub == nil {
		hub = sentry.CurrentHub()
	}
	hub.WithScope(func(scope *sentry.Scope) {
		scope.SetTag("http.method", r.Method)
		scope.SetTag("http.path", r.URL.Path)
		scope.SetTag("http.status", strconv.Itoa(status))
		if traceID != nil {
			// Ties a Sentry error report back to the matching OTel trace in
			// Grafana Cloud — the two observability tools cover different
			// signals (errors vs. request timing/spans), this is the bridge
			// between them.
			scope.SetTag("trace_id", *traceID)
		}
		hub.CaptureException(errors.New(msg))
	})
}
