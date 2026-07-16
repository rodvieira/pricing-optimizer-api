package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
	"github.com/rodvieira/pricing-optimizer-api/internal/api"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

//go:generate go tool mockgen -source=idempotency.go -destination=../../../test/mocks/httpapi/idempotency_store_mock.go -package=mockhttpapi

// idempotencyStore is the minimal capability the POST /v1/generate handler
// needs. Defined here, at the point of consumption, mirroring
// rateLimiter/generationGetter: cmd wires the concrete
// cache.RedisIdempotencyStore in, which satisfies this interface
// structurally.
type idempotencyStore interface {
	// Reserve atomically claims key for a new generation, so two requests
	// racing the same key can't both start one (see
	// cache.RedisIdempotencyStore.Reserve's doc comment for the SETNX
	// mechanics).
	Reserve(ctx context.Context, key string) (bool, error)
	// Release undoes a Reserve that never reached Save, so a legitimate
	// retry isn't rejected as "still in progress" for the rest of the TTL.
	Release(ctx context.Context, key string) error
	Save(ctx context.Context, key, generationID string) error
	Get(ctx context.Context, key string) (string, error)
}

// checkIdempotency enforces idempotencyKey on POST /v1/generate: it writes
// a response and returns false when the caller must not proceed to start a
// fresh generation (a completed one was replayed, or a 409 was returned
// because one is still in progress); true means proceed, same contract as
// checkRateLimit. An empty idempotencyKey (no header sent) always returns
// true without touching the store at all.
func (s *Server) checkIdempotency(w http.ResponseWriter, r *http.Request, idempotencyKey string) bool {
	if idempotencyKey == "" {
		return true
	}

	reserved, err := s.idempotency.Reserve(r.Context(), idempotencyKey)
	if err != nil {
		// Not fatal: same fail-open shape as checkRateLimit. Losing the
		// reservation only costs this request its dedup protection, not
		// correctness — it just generates fresh, same as a caller that
		// never sent the header at all.
		slog.WarnContext(r.Context(), "idempotency reserve failed, proceeding without it", "error", err)
		return true
	}
	if reserved {
		return true
	}
	return s.resolveIdempotencyConflict(w, r, idempotencyKey)
}

// resolveIdempotencyConflict handles a failed Reserve: idempotencyKey is
// already claimed, by this caller's own earlier attempt or a concurrent
// one. It replays a completed generation, rejects with 409 if one is
// genuinely still in progress, or — for a failed prior generation or a
// broken/expired mapping — lets the caller proceed to a fresh generation
// without re-reserving the key (a further collision on that narrow path
// just costs the optimization once more, never correctness).
func (s *Server) resolveIdempotencyConflict(w http.ResponseWriter, r *http.Request, idempotencyKey string) bool {
	generationID, err := s.idempotency.Get(r.Context(), idempotencyKey)
	if errors.Is(err, cache.ErrIdempotencyKeyPending) {
		writeProblem(w, r, http.StatusConflict, "generation already in progress for this idempotency key", "")
		return false
	}
	if err != nil {
		slog.WarnContext(r.Context(), "idempotency lookup failed after a reserve miss, starting a fresh generation", "error", err)
		return true
	}

	gen, err := s.generations.Get(r.Context(), generationID)
	if err != nil {
		slog.WarnContext(r.Context(), "idempotency key mapped to a missing generation, starting a fresh generation",
			"generation_id", generationID, "error", err)
		return true
	}

	switch gen.Status {
	case domain.GenerationStatusCompleted:
		writeReplayedGeneration(w, r, gen)
		return false
	case domain.GenerationStatusFailed:
		return true
	default: // pending or streaming: genuinely still in progress
		writeProblem(w, r, http.StatusConflict, "generation already in progress for this idempotency key", "")
		return false
	}
}

// writeReplayedGeneration writes gen (already confirmed
// GenerationStatusCompleted) as a full SSE response: one variation_completed
// frame per persisted Variation, then a terminal done frame — the same
// shape a live run ends with, just without re-running it.
func writeReplayedGeneration(w http.ResponseWriter, r *http.Request, gen *domain.Generation) {
	id := parseUUID(gen.ID)
	writeSSEHeader(w, &id)
	flusher, ok := w.(http.Flusher)
	if !ok {
		slog.ErrorContext(r.Context(), "idempotency replay: response writer does not support flushing")
		return
	}

	for _, v := range gen.Variations {
		variation := toAPIVariation(v)
		chunk := api.StreamChunk{Type: api.VariationCompleted, Strategy: &variation.Strategy, Variation: &variation}
		if err := writeSSEFrame(w, chunk); err != nil {
			slog.ErrorContext(r.Context(), "idempotency replay: write sse frame", "error", err)
			return
		}
	}

	g := toAPIGeneration(*gen)
	if err := writeSSEFrame(w, api.StreamChunk{Type: api.Done, GenerationId: &id, Generation: &g}); err != nil {
		slog.ErrorContext(r.Context(), "idempotency replay: write sse frame", "error", err)
		return
	}
	flusher.Flush()
}
