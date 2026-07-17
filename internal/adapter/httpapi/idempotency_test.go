package httpapi

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
)

func newGenerateRequest(idempotencyKey string) *http.Request {
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	if idempotencyKey != "" {
		req.Header.Set("Idempotency-Key", idempotencyKey)
	}
	return req
}

func TestGenerateVariations_Idempotency_ReservedKeyStartsFreshAndSaves(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)

	store.EXPECT().Reserve(gomock.Any(), "fresh-idem-key").Return(true, nil)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Save(gomock.Any(), "fresh-idem-key", gen.ID).Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("fresh-idem-key"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, gen.ID, rec.Header().Get("X-Generation-Id"))
}

func TestGenerateVariations_Idempotency_ReservedKeyIsReleasedWhenExecuteFails(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	store.EXPECT().Reserve(gomock.Any(), "will-fail-key").Return(true, nil)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, domain.ErrInvalidInput)
	// Save must never be called (nothing to save), but the reservation must
	// be released so a legitimate retry with this key isn't stuck reading
	// back as "still in progress" for the rest of the TTL.
	store.EXPECT().Release(gomock.Any(), "will-fail-key").Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("will-fail-key"))

	require.Equal(t, http.StatusUnprocessableEntity, rec.Code)
}

func TestGenerateVariations_Idempotency_ReservedKeyIsReleasedWhenSaveFails(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)

	store.EXPECT().Reserve(gomock.Any(), "save-fails-key").Return(true, nil)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	// Save is attempted (the SSE header has already gone out by this point,
	// so the response itself is still a normal 200) but fails; the
	// reservation must still be released rather than left stuck at
	// "pending" — this is the case a headerWritten-only gate would miss,
	// since headerWritten is already true by the time Save runs.
	store.EXPECT().Save(gomock.Any(), "save-fails-key", gen.ID).Return(errors.New("connection refused"))
	store.EXPECT().Release(gomock.Any(), "save-fails-key").Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("save-fails-key"))

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerateVariations_Idempotency_ReleaseUsesADetachedContextOnClientDisconnect(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	// events is never sent to and never closed: the only select case that
	// can ever become ready is r.Context().Done(), deterministically
	// exercising the disconnect path rather than racing it.
	events := make(chan domain.GenerationEvent)

	store.EXPECT().Reserve(gomock.Any(), "disconnect-key").Return(true, nil)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Release(gomock.Any(), "disconnect-key").DoAndReturn(func(ctx context.Context, _ string) error {
		assert.NoError(t, ctx.Err(),
			"Release must receive a context detached from the already-canceled request context, or every "+
				"release on a real client disconnect — the scenario this whole mechanism exists for — would "+
				"silently fail at the Redis client's own context check before reaching the network")
		return nil
	})

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // simulate a client that already disconnected
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	req.Header.Set("Idempotency-Key", "disconnect-key")

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
}

func TestGenerateVariations_Idempotency_ReservedKeyIsReleasedWhenTheStreamEndsBeforeAnyEvent(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	// A channel that closes without ever sending an event: the edge case
	// where the use case's context was already canceled (e.g. an
	// instantaneous client disconnect) before GenerationEventStarted could
	// be sent.
	events := make(chan domain.GenerationEvent)
	close(events)

	store.EXPECT().Reserve(gomock.Any(), "empty-stream-key").Return(true, nil)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Release(gomock.Any(), "empty-stream-key").Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("empty-stream-key"))

	// streamSSE returns immediately on the closed, empty channel without
	// ever calling writeSSEHeader; httptest.ResponseRecorder reports 200
	// when WriteHeader was never explicitly called. The behavior under test
	// here is the mock's strict Release expectation above, not this status.
	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerateVariations_Idempotency_ReserveErrorFailsOpen(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)

	store.EXPECT().Reserve(gomock.Any(), "some-key").Return(false, errors.New("connection refused"))
	// Get must never be called: a Reserve error fails open straight to a
	// fresh generation, same as checkRateLimit does on a limiter error.
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Save(gomock.Any(), "some-key", gen.ID).Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("some-key"))

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerateVariations_Idempotency_UnreservedKeyMappedToACompletedGenerationReplays(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGetter := mockhttpapi.NewMockgenerationGetter(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	gen := fixtureGeneration()
	store.EXPECT().Reserve(gomock.Any(), "repeat-key").Return(false, nil)
	store.EXPECT().Get(gomock.Any(), "repeat-key").Return(gen.ID, nil)
	mockGetter.EXPECT().Get(gomock.Any(), gen.ID).Return(&gen, nil)

	// streamer is nil: a replay must never reach it. If it did, calling
	// Execute on a nil interface would panic, which doubles as proof.
	router := NewRouter(NewServer(nil, nil, mockGetter, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("repeat-key"))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, gen.ID, rec.Header().Get("X-Generation-Id"))

	frames := sseFrames(t, rec.Body.String())
	require.Len(t, frames, 2, "one variation_completed per persisted variation, then done")
	assert.Equal(t, "variation_completed", frames[0]["type"])
	assert.Equal(t, "done", frames[1]["type"])
}

func TestGenerateVariations_Idempotency_PendingKeyReturnsConflict(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	store.EXPECT().Reserve(gomock.Any(), "pending-key").Return(false, nil)
	store.EXPECT().Get(gomock.Any(), "pending-key").Return("", cache.ErrIdempotencyKeyPending)

	// analyzer/streamer/generations are all nil: a 409 must be written
	// without touching any of them.
	router := NewRouter(NewServer(nil, nil, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("pending-key"))

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestGenerateVariations_Idempotency_KeyMappedToAStillStreamingGenerationReturnsConflict(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockGetter := mockhttpapi.NewMockgenerationGetter(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	streaming := fixtureGeneration()
	streaming.Status = domain.GenerationStatusStreaming
	store.EXPECT().Reserve(gomock.Any(), "in-flight-key").Return(false, nil)
	store.EXPECT().Get(gomock.Any(), "in-flight-key").Return(streaming.ID, nil)
	mockGetter.EXPECT().Get(gomock.Any(), streaming.ID).Return(&streaming, nil)

	// streamer is nil: a genuinely-in-progress generation must not start a
	// second one.
	router := NewRouter(NewServer(nil, nil, mockGetter, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("in-flight-key"))

	require.Equal(t, http.StatusConflict, rec.Code)
}

func TestGenerateVariations_Idempotency_KeyMappedToAFailedGenerationStartsFresh(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	mockGetter := mockhttpapi.NewMockgenerationGetter(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	failed := fixtureGeneration()
	failed.Status = domain.GenerationStatusFailed
	store.EXPECT().Reserve(gomock.Any(), "failed-key").Return(false, nil)
	store.EXPECT().Get(gomock.Any(), "failed-key").Return(failed.ID, nil)
	mockGetter.EXPECT().Get(gomock.Any(), failed.ID).Return(&failed, nil)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Save(gomock.Any(), "failed-key", gen.ID).Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, mockGetter, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("failed-key"))

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerateVariations_Idempotency_UnreservedKeyMappedToAMissingGenerationStartsFresh(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	mockGetter := mockhttpapi.NewMockgenerationGetter(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	store.EXPECT().Reserve(gomock.Any(), "stale-key").Return(false, nil)
	store.EXPECT().Get(gomock.Any(), "stale-key").Return("some-deleted-generation-id", nil)
	mockGetter.EXPECT().Get(gomock.Any(), "some-deleted-generation-id").Return(nil, domain.ErrGenerationNotFound)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Save(gomock.Any(), "stale-key", gen.ID).Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, mockGetter, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest("stale-key"))

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerateVariations_Idempotency_NilStoreNeverDerivesAContentKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)

	// idempotency store is nil: checkIdempotency, contentIdempotencyKey, and
	// saveIdempotencyMapping must all be no-ops regardless of the missing
	// header, same as before issue #24's implicit-key derivation existed.
	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, nil, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest(""))

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestGenerateVariations_Idempotency_ExplicitEmptyHeaderAlsoDerivesAContentKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)

	var reservedKey string
	store.EXPECT().Reserve(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, key string) (bool, error) {
		reservedKey = key
		return true, nil
	})
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Save(gomock.Any(), gomock.Any(), gen.ID).Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	// An explicit but empty header (as opposed to no header at all — nil vs.
	// a pointer to "") must fall back to the content-derived key the same
	// way, not be treated as a real (empty) idempotency key.
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	req.Header.Set("Idempotency-Key", "")
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, reservedKey)
}

func TestGenerateVariations_Idempotency_NoHeaderDerivesAContentKeyAndReservesIt(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	close(events)

	var reservedKey string
	store.EXPECT().Reserve(gomock.Any(), gomock.Any()).DoAndReturn(func(_ context.Context, key string) (bool, error) {
		reservedKey = key
		return true, nil
	})
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)
	store.EXPECT().Save(gomock.Any(), gomock.Any(), gen.ID).Return(nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, newGenerateRequest(""))

	require.Equal(t, http.StatusOK, rec.Code)
	assert.NotEmpty(t, reservedKey, "a content-derived key must have been reserved even with no Idempotency-Key header")
}

func TestGenerateVariations_Idempotency_IdenticalRequestsWithoutHeaderShareTheSameDerivedKey(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	var keys []string
	store.EXPECT().Reserve(gomock.Any(), gomock.Any()).Times(2).
		DoAndReturn(func(_ context.Context, key string) (bool, error) {
			keys = append(keys, key)
			return false, nil // both calls resolve via Get below; Reserve's result isn't what's under test here
		})
	store.EXPECT().Get(gomock.Any(), gomock.Any()).Times(2).Return("", cache.ErrIdempotencyKeyPending)

	router := NewRouter(NewServer(nil, nil, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	for range 2 {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, newGenerateRequest(""))
		require.Equal(t, http.StatusConflict, rec.Code)
	}

	require.Len(t, keys, 2)
	assert.Equal(t, keys[0], keys[1], "two structurally identical requests must derive the same implicit key")
}

func TestGenerateVariations_Idempotency_DifferentRequestsWithoutHeaderDeriveDifferentKeys(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	store := mockhttpapi.NewMockidempotencyStore(ctrl)

	var keys []string
	store.EXPECT().Reserve(gomock.Any(), gomock.Any()).Times(2).
		DoAndReturn(func(_ context.Context, key string) (bool, error) {
			keys = append(keys, key)
			return false, nil
		})
	store.EXPECT().Get(gomock.Any(), gomock.Any()).Times(2).Return("", cache.ErrIdempotencyKeyPending)

	router := NewRouter(NewServer(nil, nil, nil, nil, nil, store, nil), []string{"http://localhost:3000"})

	bodies := []string{
		validGenerateBody,
		`{"siteProfile":{"url":"https://different.example","title":"Other","valueProposition":"other","industry":"other","audience":{"segment":"other","sophistication":"medium"},"sourceType":"static","analyzedAt":"2026-07-14T12:00:00Z"},"strategies":["anchor"],"currency":"USD"}`,
	}
	for _, body := range bodies {
		req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(body))
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, req)
		require.Equal(t, http.StatusConflict, rec.Code)
	}

	require.Len(t, keys, 2)
	assert.NotEqual(t, keys[0], keys[1], "requests with different content must derive different implicit keys")
}
