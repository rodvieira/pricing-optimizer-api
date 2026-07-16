package httpapi

import (
	"net/http"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
	"github.com/rodvieira/pricing-optimizer-api/internal/buildinfo"
)

// Server implements the generated api.ServerInterface. Endpoints not yet built
// fall through to api.Unimplemented, which responds with 501 Not Implemented.
type Server struct {
	api.Unimplemented
	analyzer     analyzer
	streamer     streamer
	generations  generationGetter
	exporter     exporter
	rateLimiter  rateLimiter
	idempotency  idempotencyStore
	analyzeCache analyzeCache
}

// NewServer creates the API server implementation. analyzer backs
// POST /v1/analyze (cmd/api wires in the concrete usecase.AnalyzeSite);
// streamer backs POST /v1/generate (usecase.GenerateVariations); generations
// backs GET /v1/generations/{id} (the same domain.GenerationRepo streamer's
// use case saves through); exporter backs POST /v1/export/{id}
// (usecase.ExportVariation); rateLimiter guards the two endpoints
// openapi.yaml documents a 429 for (cache.RedisRateLimiter). A nil
// rateLimiter always allows, see checkRateLimit. idempotency backs the
// Idempotency-Key header on POST /v1/generate (cache.RedisIdempotencyStore);
// a nil idempotency disables the feature entirely (no lookup, no save),
// same "nil means off" shape as rateLimiter. analyzeCache backs
// POST /v1/analyze's response cache (cache.RedisResponseCache), same "nil
// means off" shape.
func NewServer(
	analyzer analyzer, streamer streamer, generations generationGetter, exporter exporter,
	rateLimiter rateLimiter, idempotency idempotencyStore, analyzeCache analyzeCache,
) *Server {
	return &Server{
		analyzer: analyzer, streamer: streamer, generations: generations,
		exporter: exporter, rateLimiter: rateLimiter, idempotency: idempotency,
		analyzeCache: analyzeCache,
	}
}

// HealthCheck reports service liveness for Fly and uptime monitors.
// (GET /v1/healthz)
func (s *Server) HealthCheck(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, r, api.HealthStatus{
		Status:  api.HealthStatusStatusOk,
		Version: buildinfo.Version,
	})
}
