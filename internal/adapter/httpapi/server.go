package httpapi

import (
	"net/http"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
)

// Version is the build version, overridable at build time via -ldflags.
// Tracked for relocation into internal/buildinfo (issue #5).
var Version = "dev"

// Server implements the generated api.ServerInterface. Endpoints not yet built
// fall through to api.Unimplemented, which responds with 501 Not Implemented.
type Server struct {
	api.Unimplemented
}

// NewServer creates the API server implementation.
func NewServer() *Server {
	return &Server{}
}

// HealthCheck reports service liveness for Fly and uptime monitors.
// (GET /v1/healthz)
func (s *Server) HealthCheck(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, api.HealthStatus{
		Status:  api.HealthStatusStatusOk,
		Version: Version,
	})
}
