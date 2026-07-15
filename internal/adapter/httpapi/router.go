package httpapi

import (
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

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

	return api.HandlerFromMux(srv, r)
}
