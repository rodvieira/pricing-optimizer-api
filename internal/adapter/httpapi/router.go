package httpapi

import (
	"net/http"

	sentryhttp "github.com/getsentry/sentry-go/http"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/go-chi/cors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"go.opentelemetry.io/otel/trace"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
)

// sentryMiddleware recovers a panic, reports it to Sentry (a safe no-op if
// telemetry.InitSentry was never called — SENTRY_DSN unset), then re-panics
// so middleware.Recoverer below — registered earlier in NewRouter, so it
// wraps this one from the outside — still catches it and writes the actual
// 500 response. Order matters: Recoverer must run outside this middleware,
// not inside it, or a panic here would have nothing left to write a
// response at all.
var sentryMiddleware = sentryhttp.New(sentryhttp.Options{Repanic: true}).Handle

// RouterOption configures optional NewRouter behavior. Kept as functional
// options (rather than growing NewRouter's positional parameter list)
// specifically so the dozens of existing call sites across this package's
// tests — none of which care about client-IP-source configuration — don't
// need touching every time a new one is added.
type RouterOption func(*routerConfig)

type routerConfig struct {
	trustedProxyHops int
}

// WithTrustedProxyHops configures the rate limiter's client-IP key to come
// from X-Forwarded-For instead of the raw TCP peer address, trusting exactly
// n reverse-proxy hops between the public internet and this server (see
// middleware.ClientIPFromXFFTrustedProxies's doc comment for the exact
// semantics, and its own warning about what happens if n stops matching
// reality). n <= 0 is a no-op — NewRouter's default (ClientIPFromRemoteAddr)
// already covers that case. See config.TrustedProxyHops for why this needs
// to be configurable at all: Cloud Run in production sits one reverse-proxy
// hop in front of this server, and the raw TCP peer address there is always
// that hop's own IP, not the caller's — see NewRouter's own comment for what
// that silently did to the rate limiter before this existed.
func WithTrustedProxyHops(n int) RouterOption {
	return func(c *routerConfig) { c.trustedProxyHops = n }
}

// NewRouter builds the HTTP handler: base middleware plus the routes generated
// from the OpenAPI contract, dispatched to srv. allowedOrigins is the frontend
// origin(s) allowed to call this API cross-origin (see config.AllowedOrigins) —
// the frontend and backend are always separate origins in this product (a
// separate Next.js repo/deploy, e.g. Vercel calling Cloud Run), never a same-
// origin reverse-proxy setup, so CORS is load-bearing in every environment,
// not just local dev.
func NewRouter(srv api.ServerInterface, allowedOrigins []string, opts ...RouterOption) http.Handler {
	cfg := routerConfig{}
	for _, opt := range opts {
		opt(&cfg)
	}

	r := chi.NewRouter()

	// The client-IP middleware runs first, so the rate limiter's key is
	// available before anything else runs. Without WithTrustedProxyHops (the
	// default, and every environment with no reverse proxy in front — local
	// dev, this package's own tests), ClientIPFromRemoteAddr is correct:
	// unlike the deprecated middleware.RealIP, it never trusts a
	// client-controlled header, only the raw TCP peer address, so it can't
	// be spoofed. That address stops being the actual caller once a reverse
	// proxy sits in front, though — Cloud Run production wires
	// WithTrustedProxyHops(1) (via config.TrustedProxyHops) for exactly this
	// reason: the raw TCP peer address there is always Google's front end,
	// which made RATE_LIMIT_REQUESTS (ADR-0006) a near-global budget shared
	// by every caller behind that front end, not a per-caller one.
	if cfg.trustedProxyHops > 0 {
		r.Use(middleware.ClientIPFromXFFTrustedProxies(cfg.trustedProxyHops))
	} else {
		r.Use(middleware.ClientIPFromRemoteAddr)
	}
	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)
	r.Use(sentryMiddleware)
	r.Use(renameSpanToRoutePattern)
	r.Use(cors.Handler(cors.Options{
		AllowedOrigins: allowedOrigins,
		AllowedMethods: []string{http.MethodGet, http.MethodPost, http.MethodOptions},
		// Idempotency-Key: issue #23. Accept: SSE responses from POST
		// /v1/generate set it explicitly (see lib/api/generate.ts).
		AllowedHeaders: []string{"Content-Type", "Accept", "Idempotency-Key"},
		MaxAge:         300,
	}))

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
