package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

//go:generate go tool mockgen -source=analyze.go -destination=../../../test/mocks/httpapi/analyzer_mock.go -package=mockhttpapi

// analyzer is the minimal capability the /v1/analyze handler needs. Defined
// here, at the point of consumption, so httpapi never imports usecase (the
// constitution's adapter-never-imports-usecase rule): cmd wires the concrete
// usecase.AnalyzeSite in, which satisfies this interface structurally.
type analyzer interface {
	Execute(ctx context.Context, rawURL string) (*domain.SiteProfile, error)
}

// AnalyzeSite implements api.ServerInterface. (POST /v1/analyze)
func (s *Server) AnalyzeSite(w http.ResponseWriter, r *http.Request) {
	var req api.AnalyzeRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "malformed request body", err.Error())
		return
	}

	profile, err := s.analyzer.Execute(r.Context(), req.Url)
	if err != nil {
		writeAnalyzeError(w, r, err)
		return
	}

	writeJSON(w, http.StatusOK, toAPISiteProfile(*profile))
}

// writeAnalyzeError maps an AnalyzeSite.Execute error to the RFC 7807
// response openapi.yaml documents for POST /v1/analyze.
func writeAnalyzeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeProblem(w, r, http.StatusUnprocessableEntity, "invalid analyze request", err.Error())
	case errors.Is(err, domain.ErrSiteUnreachable):
		writeProblem(w, r, http.StatusBadGateway, "could not fetch or parse the target site", err.Error())
	default:
		// ErrProviderUnavailable, ErrProviderUnauthorized, ErrInvalidLLMResponse,
		// and anything unclassified are all our dependency's fault, not the
		// caller's: the contract has no more specific status for them.
		slog.Error("analyze site failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "could not analyze the site", "")
	}
}

// toAPISiteProfile maps the domain type to the generated response shape.
func toAPISiteProfile(p domain.SiteProfile) api.SiteProfile {
	out := api.SiteProfile{
		Url:              p.URL,
		Title:            p.Title,
		ValueProposition: p.ValueProposition,
		Industry:         p.Industry,
		Audience: api.Audience{
			Segment:        p.Audience.Segment,
			Sophistication: api.AudienceSophistication(p.Audience.Sophistication),
			PricePosition:  audiencePricePosition(p.Audience.PricePosition),
		},
		SourceType: api.SiteProfileSourceType(p.SourceType),
		AnalyzedAt: p.AnalyzedAt,
	}
	if len(p.Keywords) > 0 {
		out.Keywords = &p.Keywords
	}
	return out
}

func audiencePricePosition(p domain.PricePosition) *api.AudiencePricePosition {
	if p == "" {
		return nil
	}
	v := api.AudiencePricePosition(p)
	return &v
}
