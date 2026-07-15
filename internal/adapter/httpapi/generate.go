package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/rodvieira/pricing-optimizer-api/internal/api"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

//go:generate go tool mockgen -source=generate.go -destination=../../../test/mocks/httpapi/streamer_mock.go -package=mockhttpapi

// maxGenerateBodyBytes bounds the /v1/generate request body. Same rationale
// as maxAnalyzeBodyBytes: the contract is a handful of scalar/short-string
// fields plus the already-bounded SiteProfile shape, never anywhere near
// this size in a well-formed request.
const maxGenerateBodyBytes = 1 << 20 // 1 MiB

// allStrategies is every known PricingStrategy, in the fixed order the
// contract's "defaults to all three when omitted" behavior uses.
var allStrategies = []domain.PricingStrategy{
	domain.StrategyAnchor, domain.StrategyFreemium, domain.StrategyValueBased,
}

// streamer is the minimal capability the /v1/generate handler needs. Defined
// here, at the point of consumption, so httpapi never imports usecase (the
// constitution's adapter-never-imports-usecase rule): cmd wires the concrete
// usecase.GenerateVariations in, which satisfies this interface structurally.
// The parameter type is domain.GenerateVariationsInput specifically so this
// interface never needs to name anything from the usecase package.
type streamer interface {
	Execute(ctx context.Context, in domain.GenerateVariationsInput) (<-chan domain.GenerationEvent, error)
}

// GenerateVariations implements api.ServerInterface. (POST /v1/generate)
func (s *Server) GenerateVariations(w http.ResponseWriter, r *http.Request, _ api.GenerateVariationsParams) {
	if !checkRateLimit(w, r, s.rateLimiter) {
		return
	}

	var req api.GenerateRequest
	body := http.MaxBytesReader(w, r.Body, maxGenerateBodyBytes)
	if err := json.NewDecoder(body).Decode(&req); err != nil {
		writeProblem(w, r, http.StatusBadRequest, "malformed request body", err.Error())
		return
	}

	events, err := s.streamer.Execute(r.Context(), toGenerateVariationsInput(req))
	if err != nil {
		writeGenerateError(w, r, err)
		return
	}

	streamSSE(w, r, events)
}

// writeGenerateError maps a GenerateVariations.Execute error to the RFC 7807
// response openapi.yaml documents for POST /v1/generate. Execute only ever
// returns an error before the stream starts (input validation or the
// initial Generation save); any later failure is relayed as an in-stream
// "error" GenerationEvent instead, handled by streamSSE.
func writeGenerateError(w http.ResponseWriter, r *http.Request, err error) {
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		writeProblem(w, r, http.StatusUnprocessableEntity, "invalid generate request", err.Error())
	default:
		slog.Error("generate variations failed", "error", err)
		writeProblem(w, r, http.StatusInternalServerError, "could not start generation", "")
	}
}

// toGenerateVariationsInput maps the request to the use case input,
// resolving the two contract defaults that are the HTTP layer's job (see
// domain.GenerateVariationsInput's doc comment): an omitted or empty
// strategies list becomes all three known strategies, and an omitted or
// empty currency becomes "USD".
func toGenerateVariationsInput(req api.GenerateRequest) domain.GenerateVariationsInput {
	strategies := allStrategies
	if req.Strategies != nil && len(*req.Strategies) > 0 {
		strategies = make([]domain.PricingStrategy, len(*req.Strategies))
		for i, s := range *req.Strategies {
			strategies[i] = domain.PricingStrategy(s)
		}
	}

	currency := "USD"
	if req.Currency != nil && *req.Currency != "" {
		currency = *req.Currency
	}

	return domain.GenerateVariationsInput{
		SiteProfile: fromAPISiteProfile(req.SiteProfile),
		Strategies:  strategies,
		Currency:    currency,
	}
}

// fromAPISiteProfile maps the generated request shape to the domain type,
// the reverse of toAPISiteProfile in analyze.go.
func fromAPISiteProfile(p api.SiteProfile) domain.SiteProfile {
	sp := domain.SiteProfile{
		URL:              p.Url,
		Title:            p.Title,
		ValueProposition: p.ValueProposition,
		Industry:         p.Industry,
		Audience: domain.Audience{
			Segment:        p.Audience.Segment,
			Sophistication: domain.Sophistication(p.Audience.Sophistication),
		},
		SourceType: domain.SourceType(p.SourceType),
		AnalyzedAt: p.AnalyzedAt,
	}
	if p.Audience.PricePosition != nil {
		sp.Audience.PricePosition = domain.PricePosition(*p.Audience.PricePosition)
	}
	if p.Keywords != nil {
		sp.Keywords = *p.Keywords
	}
	return sp
}

// streamSSE writes each event from events as one SSE "data:" frame of the
// generated api.StreamChunk shape, flushing after every event, until events
// closes (the use case is done, successfully or not) or the client
// disconnects (r.Context() is done, which the use case itself also observes
// and reacts to independently).
//
// It disables the server's default write deadline for this connection via
// http.ResponseController: http.Server.WriteTimeout is sized for ordinary
// request/response handlers and would otherwise cut off a legitimately
// long-lived stream (three parallel LLM generations can easily exceed it).
func streamSSE(w http.ResponseWriter, r *http.Request, events <-chan domain.GenerationEvent) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.Error("generate variations: response writer does not support flushing")
		writeProblem(w, r, http.StatusInternalServerError, "could not stream the response", "")
		return
	}
	if err := http.NewResponseController(w).SetWriteDeadline(time.Time{}); err != nil {
		// Not fatal: some ResponseWriters (notably httptest's) don't support
		// deadlines. Real net/http connections do.
		slog.Warn("generate variations: could not disable write deadline", "error", err)
	}

	headerWritten := false
	for {
		select {
		case ev, ok := <-events:
			if !ok {
				return
			}
			if ev.Type == domain.GenerationEventError && ev.Err != nil {
				slog.Error("generate variations: stream error", "error", ev.Err, "strategy", ev.Strategy)
			}

			chunk := toAPIStreamChunk(ev)
			if !headerWritten {
				writeSSEHeader(w, chunk.GenerationId)
				headerWritten = true
			}
			if err := writeSSEFrame(w, chunk); err != nil {
				slog.Error("generate variations: write sse frame", "error", err)
				return
			}
			flusher.Flush()
		case <-r.Context().Done():
			return
		}
	}
}

// writeSSEHeader writes the response headers and status line exactly once,
// lazily on the first event, since the generation id is only known once the
// use case's first event carries it.
func writeSSEHeader(w http.ResponseWriter, generationID *uuid.UUID) {
	if generationID != nil {
		w.Header().Set("X-Generation-Id", generationID.String())
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)
}

// writeSSEFrame encodes chunk as one SSE "data:" frame.
func writeSSEFrame(w http.ResponseWriter, chunk api.StreamChunk) error {
	data, err := json.Marshal(chunk)
	if err != nil {
		return fmt.Errorf("marshal stream chunk: %w", err)
	}
	if _, err := fmt.Fprintf(w, "data: %s\n\n", data); err != nil {
		return fmt.Errorf("write frame: %w", err)
	}
	return nil
}

// toAPIStreamChunk maps one domain.GenerationEvent to the generated SSE
// frame shape. ev.Err is deliberately not echoed into the Problem's detail:
// by the time a GenerationEvent carries an error, it is always a dependency
// failure (LLM provider, repository), never anything the caller did wrong —
// AnalyzeSite's own error mapping applies the same no-leak rule.
func toAPIStreamChunk(ev domain.GenerationEvent) api.StreamChunk {
	chunk := api.StreamChunk{Type: api.StreamChunkType(ev.Type)}

	if ev.Strategy != "" {
		strategy := api.PricingStrategy(ev.Strategy)
		chunk.Strategy = &strategy
	}
	if ev.Delta != "" {
		chunk.Delta = &ev.Delta
	}
	if ev.Variation != nil {
		v := toAPIVariation(*ev.Variation)
		chunk.Variation = &v
	}
	if ev.Generation != nil {
		id := parseUUID(ev.Generation.ID)
		chunk.GenerationId = &id
		g := toAPIGeneration(*ev.Generation)
		chunk.Generation = &g
	}
	if ev.Err != nil {
		chunk.Problem = &api.Problem{Status: http.StatusInternalServerError, Title: "generation failed"}
	}
	return chunk
}

// parseUUID parses id, logging and returning uuid.Nil on failure rather than
// propagating the error: g.ID/v.ID are always adapter-stamped via
// uuid.NewString() and never expected to fail, but a response mapper must
// not panic or error out over a domain invariant it can't itself enforce.
func parseUUID(id string) uuid.UUID {
	parsed, err := uuid.Parse(id)
	if err != nil {
		slog.Error("generate variations: invalid id in domain value", "id", id, "error", err)
	}
	return parsed
}

func toAPIGeneration(g domain.Generation) api.Generation {
	id := parseUUID(g.ID)
	out := api.Generation{
		Id:         id,
		SourceUrl:  g.SourceURL,
		Status:     api.GenerationStatus(g.Status),
		CreatedAt:  g.CreatedAt,
		Variations: make([]api.Variation, len(g.Variations)),
	}
	for i, v := range g.Variations {
		out.Variations[i] = toAPIVariation(v)
	}
	sp := toAPISiteProfile(g.SiteProfile)
	out.SiteProfile = &sp
	return out
}

func toAPIVariation(v domain.Variation) api.Variation {
	id := parseUUID(v.ID)
	out := api.Variation{
		Id:       id,
		Strategy: api.PricingStrategy(v.Strategy),
		Headline: v.Headline,
		Tiers:    make([]api.PricingTier, len(v.Tiers)),
	}
	if v.Subheadline != "" {
		out.Subheadline = &v.Subheadline
	}
	if v.Rationale != "" {
		out.Rationale = &v.Rationale
	}
	for i, t := range v.Tiers {
		out.Tiers[i] = toAPIPricingTier(t)
	}
	return out
}

func toAPIPricingTier(t domain.PricingTier) api.PricingTier {
	features := t.Features
	if features == nil {
		features = []string{} // Features is a required (non-omitempty) array; must never marshal as null.
	}
	out := api.PricingTier{
		Name:     t.Name,
		Features: features,
		Price:    toAPIPrice(t.Price),
	}
	if t.Tagline != "" {
		out.Tagline = &t.Tagline
	}
	if t.CTA != "" {
		out.Cta = &t.CTA
	}
	if t.Badge != "" {
		out.Badge = &t.Badge
	}
	if t.Highlighted {
		out.Highlighted = &t.Highlighted
	}
	return out
}

func toAPIPrice(p domain.Price) api.Price {
	out := api.Price{
		Amount:   p.AmountMinorUnits,
		Currency: p.Currency,
		Interval: api.PriceInterval(p.Interval),
	}
	if p.CustomLabel != "" {
		out.CustomLabel = &p.CustomLabel
	}
	return out
}
