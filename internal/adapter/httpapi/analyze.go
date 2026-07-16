package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
	"github.com/rodvieira/pricing-optimizer-api/internal/api"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

//go:generate go tool mockgen -source=analyze.go -destination=../../../test/mocks/httpapi/analyzer_mock.go -package=mockhttpapi

// maxAnalyzeBodyBytes bounds the /v1/analyze request body. The contract is a
// single "url" field; a well-formed request is never more than a few hundred
// bytes, so this only exists to stop an oversized body from being read in
// full before json.Decode gets a chance to reject it.
const maxAnalyzeBodyBytes = 1 << 20 // 1 MiB

// analyzer is the minimal capability the /v1/analyze handler needs. Defined
// here, at the point of consumption, so httpapi never imports usecase (the
// constitution's adapter-never-imports-usecase rule): cmd wires the concrete
// usecase.AnalyzeSite in, which satisfies this interface structurally.
type analyzer interface {
	Execute(ctx context.Context, rawURL string) (*domain.SiteProfile, error)
}

// analyzeCache is the minimal capability the /v1/analyze handler needs to
// cache responses, keyed on the submitted URL. Defined here, at the point
// of consumption, mirroring rateLimiter/idempotencyStore: cmd wires the
// concrete cache.RedisResponseCache in, which satisfies this interface
// structurally. A nil analyzeCache disables caching entirely (same "nil
// means off" shape as rateLimiter/idempotency).
type analyzeCache interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key, value string) error
}

// AnalyzeSite implements api.ServerInterface. (POST /v1/analyze)
func (s *Server) AnalyzeSite(w http.ResponseWriter, r *http.Request) {
	if !checkRateLimit(w, r, s.rateLimiter) {
		return
	}

	var req api.AnalyzeRequest
	body := http.MaxBytesReader(w, r.Body, maxAnalyzeBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "malformed request body", err.Error())
		return
	}

	// Cache key is the raw, unnormalized URL: "https://x.com" and
	// "https://x.com/" (or a different query-param order) are different
	// cache entries. Deliberately simple for now — normalization is a cheap
	// future improvement, not a correctness requirement (a miss just costs
	// a redundant analyze, never a wrong answer).
	if s.analyzeCache != nil {
		if cached, err := s.analyzeCache.Get(r.Context(), req.Url); err == nil {
			writeJSONBytes(w, r, []byte(cached))
			return
		} else if !errors.Is(err, cache.ErrResponseCacheMiss) {
			slog.WarnContext(r.Context(), "analyze cache lookup failed, analyzing fresh", "error", err)
		}
	}

	profile, err := s.analyzer.Execute(r.Context(), req.Url)
	if err != nil {
		writeAnalyzeError(w, r, err)
		return
	}

	data, err := json.Marshal(toAPISiteProfile(*profile))
	if err != nil {
		slog.ErrorContext(r.Context(), "encode analyze response", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "could not encode the response", "")
		return
	}
	if s.analyzeCache != nil {
		if err := s.analyzeCache.Set(r.Context(), req.Url, string(data)); err != nil {
			slog.WarnContext(r.Context(), "analyze cache save failed", "error", err)
		}
	}
	writeJSONBytes(w, r, data)
}

// writeAnalyzeError maps an AnalyzeSite.Execute error to the RFC 7807
// response openapi.yaml documents for POST /v1/analyze. Only ErrInvalidInput
// messages are safe to echo to the client (they describe the caller's own
// request, e.g. "localhost is not an analyzable host"); every other case
// wraps a dependency failure (scraper network errors, LLM provider errors)
// that must not leak internal detail, so it goes to the server log instead.
func writeAnalyzeError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeProblem(w, r, http.StatusUnprocessableEntity, "invalid analyze request", err.Error())
	case errors.Is(err, domain.ErrSiteUnreachable):
		slog.ErrorContext(r.Context(), "analyze site: target unreachable", "error", err)
		writeProblem(w, r, http.StatusBadGateway, "could not fetch or parse the target site", "")
	default:
		// ErrProviderUnavailable, ErrProviderUnauthorized, ErrInvalidLLMResponse,
		// and anything unclassified are all our dependency's fault, not the
		// caller's: the contract has no more specific status for them.
		slog.ErrorContext(r.Context(), "analyze site failed", "error", err)
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
