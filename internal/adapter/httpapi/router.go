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

	r.Use(middleware.RequestID)
	r.Use(middleware.Recoverer)

	return api.HandlerFromMux(srv, r)
}
