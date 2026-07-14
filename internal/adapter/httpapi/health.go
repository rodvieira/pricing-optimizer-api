package httpapi

import "net/http"

// Version is the build version, overridable at build time via -ldflags.
var Version = "dev"

// healthStatus mirrors the HealthStatus schema in openapi.yaml. It will be
// replaced by the generated type once oapi-codegen is wired (Sprint 2).
type healthStatus struct {
	Status  string            `json:"status"`
	Version string            `json:"version"`
	Checks  map[string]string `json:"checks,omitempty"`
}

// handleHealth reports service liveness for Fly and uptime monitors.
func handleHealth(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, healthStatus{
		Status:  "ok",
		Version: Version,
	})
}
