package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

//go:generate go tool mockgen -source=get_generation.go -destination=../../../test/mocks/httpapi/generation_getter_mock.go -package=mockhttpapi

// generationGetter is the minimal capability the GET /v1/generations/{id}
// handler needs. Defined here, at the point of consumption, mirroring
// analyzer/streamer: cmd wires the concrete domain.GenerationRepo in (the
// same one usecase.GenerateVariations saves through), which satisfies this
// interface structurally.
type generationGetter interface {
	Get(ctx context.Context, id string) (*domain.Generation, error)
}

// GetGeneration implements api.ServerInterface. (GET /v1/generations/{id})
func (s *Server) GetGeneration(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	gen, err := s.generations.Get(r.Context(), id.String())
	if err != nil {
		writeGetGenerationError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPIGeneration(*gen))
}

// writeGetGenerationError maps a GenerationRepo.Get error to the RFC 7807
// response openapi.yaml documents for GET /v1/generations/{id}.
func writeGetGenerationError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrGenerationNotFound):
		writeProblem(w, r, http.StatusNotFound, "generation not found", "")
	default:
		slog.Error("get generation failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "could not fetch the generation", "")
	}
}
