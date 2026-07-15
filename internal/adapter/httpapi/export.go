package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	openapi_types "github.com/oapi-codegen/runtime/types"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

//go:generate go tool mockgen -source=export.go -destination=../../../test/mocks/httpapi/exporter_mock.go -package=mockhttpapi

// maxExportBodyBytes bounds the /v1/export/{id} request body. Same rationale
// as maxAnalyzeBodyBytes/maxGenerateBodyBytes: the contract is two scalar
// fields, never anywhere near this size in a well-formed request.
const maxExportBodyBytes = 1 << 20 // 1 MiB

// exporter is the minimal capability the /v1/export/{id} handler needs.
// Defined here, at the point of consumption, mirroring analyzer/streamer/
// generationGetter: cmd wires the concrete usecase.ExportVariation in, which
// satisfies this interface structurally.
type exporter interface {
	Execute(ctx context.Context, in domain.ExportVariationInput) (*domain.ExportResult, error)
}

// ExportVariation implements api.ServerInterface. (POST /v1/export/{id})
func (s *Server) ExportVariation(w http.ResponseWriter, r *http.Request, id openapi_types.UUID) {
	var req api.ExportRequest
	body := http.MaxBytesReader(w, r.Body, maxExportBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "malformed request body", err.Error())
		return
	}

	in := domain.ExportVariationInput{
		GenerationID: id.String(),
		VariationID:  req.VariationId.String(),
		Format:       domain.ExportFormat(req.Format),
	}

	result, err := s.exporter.Execute(r.Context(), in)
	if err != nil {
		writeExportError(w, r, err)
		return
	}

	writeJSON(w, toAPIExportResult(*result))
}

// writeExportError maps an ExportVariation.Execute error to the RFC 7807
// response openapi.yaml documents for POST /v1/export/{id}. Unlike
// /v1/analyze and /v1/generate, this endpoint's contract has no 422: an
// invalid request (bad format enum, missing ids) is a 400 here.
func writeExportError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeProblem(w, r, http.StatusBadRequest, "invalid export request", err.Error())
	case errors.Is(err, domain.ErrGenerationNotFound), errors.Is(err, domain.ErrVariationNotFound):
		writeProblem(w, r, http.StatusNotFound, "generation or variation not found", "")
	default:
		slog.Error("export variation failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "could not export the variation", "")
	}
}

// toAPIExportResult maps the domain type to the generated response shape.
func toAPIExportResult(r domain.ExportResult) api.ExportResult {
	return api.ExportResult{
		Format:      api.ExportResultFormat(r.Format),
		Filename:    r.Filename,
		ContentType: r.ContentType,
		Content:     r.Content,
	}
}
