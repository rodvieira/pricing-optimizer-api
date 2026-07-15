package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
)

// NewRouter builds the HTTP handler: base middleware plus the routes generated
// from the OpenAPI contract, dispatched to srv.
func NewRouter(srv api.ServerInterface) http.Handler {
	r := chi.NewRouter()

	// ClientIPFromRemoteAddr first, so the rate limiter's key is available
	// before anything else runs. Unlike the deprecated middleware.RealIP, it
	// never trusts a client-controlled header (X-Forwarded-For/X-Real-IP):
	// it stores the raw TCP peer address, so it can't be spoofed. This repo
	// has no reverse proxy topology decided yet (deploy is Sprint 7); once
	// one exists, swap in whichever ClientIPFrom* middleware matches it
	// (e.g. a single trusted header the proxy unconditionally overwrites).
	r.Use(middleware.ClientIPFromRemoteAddr)
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	// otelhttp wraps everything, including chi's own middleware, so the span
	// covers the whole request lifecycle (a panic Recoverer catches still
	// lands inside it). When telemetry.Init installed the noop
	// TracerProvider (no OTLP endpoint configured), otel.Tracer() returns a
	// tracer that produces invalid span contexts — this handler still runs
	// exactly the same, just without a real trace_id downstream.
	return otelhttp.NewHandler(
		api.HandlerFromMux(srv, r),
		"http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
}
