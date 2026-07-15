package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

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
	r.Use(renameSpanToRoutePattern)

	// otelhttp wraps everything, including chi's own middleware, so the span
	// covers the whole request lifecycle (a panic Recoverer catches still
	// lands inside it). When telemetry.Init installed the noop
	// TracerProvider (no OTLP endpoint configured), otel.Tracer() returns a
	// tracer that produces invalid span contexts — this handler still runs
	// exactly the same, just without a real trace_id downstream. The initial
	// name here is only a placeholder for the two requests renameSpanToRoutePattern
	// can't rename (no route matched at all): everything else gets the low-
	// cardinality route-pattern name instead of this raw, per-id path.
	return otelhttp.NewHandler(
		api.HandlerFromMux(srv, r),
		"http.server",
		otelhttp.WithSpanNameFormatter(func(_ string, r *http.Request) string {
			return r.Method + " " + r.URL.Path
		}),
	)
}

// renameSpanToRoutePattern renames the otelhttp span from the raw,
// per-request path (e.g. "GET /v1/generations/4f9e...") to the route
// template chi actually matched (e.g. "GET /v1/generations/{id}") once
// routing has happened. It must run this way round — call next first, then
// read the pattern — because chi.RouteContext only finishes accumulating
// RoutePattern() once the tree walk to the terminal handler completes;
// reading it beforehand (e.g. from code that runs before next.ServeHTTP)
// would see an empty pattern. The span itself was created by otelhttp
// wrapping the whole chain, so it's already the active span in r's context
// by the time this (chi-registered, so inherently inside otelhttp) middleware
// runs.
func renameSpanToRoutePattern(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		next.ServeHTTP(w, r)
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			if pattern := rctx.RoutePattern(); pattern != "" {
				trace.SpanFromContext(r.Context()).SetName(r.Method + " " + pattern)
			}
		}
	})
}
