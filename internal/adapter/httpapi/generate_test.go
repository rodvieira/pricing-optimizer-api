package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
)

const validGenerateBody = `{
	"siteProfile": {
		"url": "https://example.com",
		"title": "Acme Analytics",
		"valueProposition": "Real-time analytics for indie SaaS founders",
		"industry": "developer-tools",
		"audience": {"segment": "SaaS founders", "sophistication": "medium"},
		"sourceType": "static",
		"analyzedAt": "2026-07-14T12:00:00Z"
	},
	"strategies": ["anchor"],
	"currency": "USD"
}`

func fixtureGeneration() domain.Generation {
	return domain.Generation{
		ID:        "b6f1c6b2-6b8a-4b1a-9f1a-1c2c3c4c5c6c",
		SourceURL: "https://example.com",
		Status:    domain.GenerationStatusCompleted,
		CreatedAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
		Variations: []domain.Variation{
			{
				ID:       "a1a1a1a1-1111-1111-1111-111111111111",
				Strategy: domain.StrategyAnchor,
				Headline: "Simple, transparent pricing",
				Tiers: []domain.PricingTier{
					{
						Name:     "Pro",
						Price:    domain.Price{AmountMinorUnits: 2900, Currency: "USD", Interval: domain.IntervalMonthly},
						Features: []string{"Feature A"},
					},
				},
			},
		},
	}
}

// sseFrames splits a recorded SSE body into its "data:" payloads, decoded as
// generic maps so the test can assert on individual fields.
func sseFrames(t *testing.T, body string) []map[string]any {
	t.Helper()

	var frames []map[string]any
	for _, line := range strings.Split(body, "\n") {
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		var frame map[string]any
		require.NoError(t, json.Unmarshal([]byte(strings.TrimPrefix(line, "data: ")), &frame))
		frames = append(frames, frame)
	}
	return frames
}

func TestGenerateVariations_Streams(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	gen := fixtureGeneration()
	events := make(chan domain.GenerationEvent, 4)
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	events <- domain.GenerationEvent{Type: domain.GenerationEventVariationStarted, Strategy: domain.StrategyAnchor}
	events <- domain.GenerationEvent{Type: domain.GenerationEventToken, Strategy: domain.StrategyAnchor, Delta: "Simple"}
	events <- domain.GenerationEvent{
		Type: domain.GenerationEventDone, Generation: &gen,
	}
	close(events)

	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	assert.Equal(t, gen.ID, rec.Header().Get("X-Generation-Id"))

	frames := sseFrames(t, rec.Body.String())
	require.Len(t, frames, 4)
	assert.Equal(t, "generation_started", frames[0]["type"])
	assert.Equal(t, "variation_started", frames[1]["type"])
	assert.Equal(t, "token", frames[2]["type"])
	assert.Equal(t, "Simple", frames[2]["delta"])
	assert.Equal(t, "done", frames[3]["type"])
	generation, ok := frames[3]["generation"].(map[string]any)
	require.True(t, ok, "done frame must carry the generation")
	assert.Equal(t, "completed", generation["status"])
}

func TestGenerateVariations_DefaultsStrategiesAndCurrency(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	var captured domain.GenerateVariationsInput
	events := make(chan domain.GenerationEvent)
	close(events)
	mockStreamer.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in domain.GenerateVariationsInput) (<-chan domain.GenerationEvent, error) {
			captured = in
			return events, nil
		})

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	body := `{"siteProfile": {"url": "https://example.com", "title": "Acme", "valueProposition": "x", "industry": "y",
		"audience": {"segment": "z", "sophistication": "medium"}, "sourceType": "static", "analyzedAt": "2026-07-14T12:00:00Z"}}`
	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(body))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.ElementsMatch(t,
		[]domain.PricingStrategy{domain.StrategyAnchor, domain.StrategyFreemium, domain.StrategyValueBased},
		captured.Strategies)
	assert.Equal(t, "USD", captured.Currency)
}

func TestGenerateVariations_MalformedBodyIsBadRequest(t *testing.T) {
	t.Parallel()

	router := NewRouter(NewServer(nil, nil, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(`{"siteProfile":`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	assert.Equal(t, http.StatusBadRequest, rec.Code)
	assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
}

func TestGenerateVariations_ExecuteErrors(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name            string
		err             error
		wantStatus      int
		wantDetailEmpty bool
	}{
		{
			// ErrInvalidInput messages describe the caller's own request
			// (e.g. "duplicate strategy"), so echoing them is safe and
			// matches AnalyzeSite's error mapping.
			name:       "invalid input is unprocessable and echoes the detail",
			err:        domain.ErrInvalidInput,
			wantStatus: http.StatusUnprocessableEntity,
		},
		{
			name:            "unclassified failure is an internal error without leaking detail",
			err:             errors.New("db down"),
			wantStatus:      http.StatusInternalServerError,
			wantDetailEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
			mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return(nil, tt.err)

			router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))

			var problem map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			if tt.wantDetailEmpty {
				assert.NotContains(t, problem, "detail", "must not leak the internal error to the client")
			}
		})
	}
}

func TestGenerateVariations_MidStreamErrorEventDoesNotLeak(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{
		Type: domain.GenerationEventError, Strategy: domain.StrategyAnchor,
		Err: errors.New("credentials rejected by upstream llm provider"),
	}
	close(events)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	frames := sseFrames(t, rec.Body.String())
	require.Len(t, frames, 1)
	assert.Equal(t, "error", frames[0]["type"])
	problem, ok := frames[0]["problem"].(map[string]any)
	require.True(t, ok, "error frame must carry a problem")
	assert.NotContains(t, rec.Body.String(), "credentials rejected", "must not leak the internal error text")
	assert.NotEmpty(t, problem["title"])
}

// nonFlushingWriter implements http.ResponseWriter but deliberately not
// http.Flusher, to exercise streamSSE's defensive check for a ResponseWriter
// that can't support streaming.
type nonFlushingWriter struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (w *nonFlushingWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *nonFlushingWriter) Write(p []byte) (int, error) { return w.body.Write(p) }
func (w *nonFlushingWriter) WriteHeader(status int)      { w.status = status }

func TestGenerateVariations_NonFlushingWriterIsAnInternalError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)
	events := make(chan domain.GenerationEvent)
	close(events)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	w := &nonFlushingWriter{}

	router.ServeHTTP(w, req)

	assert.Equal(t, http.StatusInternalServerError, w.status)
	assert.Contains(t, w.header.Get("Content-Type"), "application/problem+json")
}

// full mapping fixtures: exercise every optional field the response mappers
// handle, in both the request (fromAPISiteProfile) and the streamed result
// (toAPIVariation/toAPIPricingTier/toAPIPrice/toAPIGeneration).
const fullGenerateBody = `{
	"siteProfile": {
		"url": "https://example.com",
		"title": "Acme Analytics",
		"valueProposition": "Real-time analytics for indie SaaS founders",
		"industry": "developer-tools",
		"audience": {"segment": "SaaS founders", "sophistication": "medium", "pricePosition": "premium"},
		"keywords": ["analytics", "saas"],
		"sourceType": "static",
		"analyzedAt": "2026-07-14T12:00:00Z"
	},
	"strategies": ["anchor"],
	"currency": "USD"
}`

func fixtureFullVariation() domain.Variation {
	return domain.Variation{
		ID:          "a1a1a1a1-1111-1111-1111-111111111111",
		Strategy:    domain.StrategyAnchor,
		Headline:    "Simple, transparent pricing",
		Subheadline: "Start free, upgrade when you grow",
		Rationale:   "A generous free tier builds trust before asking for payment.",
		Tiers: []domain.PricingTier{
			{
				Name:        "Pro",
				Tagline:     "For growing teams",
				CTA:         "Start Pro trial",
				Badge:       "Most popular",
				Highlighted: true,
				Features:    []string{"Feature A"},
				Price: domain.Price{
					AmountMinorUnits: 0,
					Currency:         "USD",
					CustomLabel:      "Contact us",
				},
			},
		},
	}
}

func TestGenerateVariations_FullMapping(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	var captured domain.GenerateVariationsInput
	gen := fixtureGeneration()
	gen.Variations = []domain.Variation{fixtureFullVariation()}
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventDone, Generation: &gen}
	close(events)

	mockStreamer.EXPECT().
		Execute(gomock.Any(), gomock.Any()).
		DoAndReturn(func(_ context.Context, in domain.GenerateVariationsInput) (<-chan domain.GenerationEvent, error) {
			captured = in
			return events, nil
		})

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(fullGenerateBody))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	// Request-side optional fields (fromAPISiteProfile).
	assert.Equal(t, domain.PricePositionPremium, captured.SiteProfile.Audience.PricePosition)
	assert.Equal(t, []string{"analytics", "saas"}, captured.SiteProfile.Keywords)

	// Response-side optional fields (toAPIVariation/toAPIPricingTier/toAPIPrice).
	frames := sseFrames(t, rec.Body.String())
	require.Len(t, frames, 1)
	generation, ok := frames[0]["generation"].(map[string]any)
	require.True(t, ok)
	variations, ok := generation["variations"].([]any)
	require.True(t, ok)
	require.Len(t, variations, 1)
	variation, ok := variations[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Start free, upgrade when you grow", variation["subheadline"])
	assert.Equal(t, "A generous free tier builds trust before asking for payment.", variation["rationale"])

	tiers, ok := variation["tiers"].([]any)
	require.True(t, ok)
	require.Len(t, tiers, 1)
	tier, ok := tiers[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "For growing teams", tier["tagline"])
	assert.Equal(t, "Start Pro trial", tier["cta"])
	assert.Equal(t, "Most popular", tier["badge"])
	assert.Equal(t, true, tier["highlighted"])

	price, ok := tier["price"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "Contact us", price["customLabel"])
}

func TestGenerateVariations_NilFeaturesMarshalsAsEmptyArray(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	v := fixtureFullVariation()
	v.Tiers[0].Features = nil
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventVariationCompleted, Variation: &v}
	close(events)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	frames := sseFrames(t, rec.Body.String())
	require.Len(t, frames, 1)
	variation, ok := frames[0]["variation"].(map[string]any)
	require.True(t, ok)
	tiers, ok := variation["tiers"].([]any)
	require.True(t, ok)
	require.Len(t, tiers, 1)
	tier, ok := tiers[0].(map[string]any)
	require.True(t, ok)

	features, ok := tier["features"].([]any)
	require.True(t, ok, "features must marshal as [] not null for a required array field")
	assert.Empty(t, features)
}

func TestGenerateVariations_InvalidVariationIDFallsBackToNilUUID(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	badVariation := fixtureFullVariation()
	badVariation.ID = "not-a-uuid"
	events := make(chan domain.GenerationEvent, 1)
	events <- domain.GenerationEvent{Type: domain.GenerationEventVariationCompleted, Variation: &badVariation}
	close(events)
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	rec := httptest.NewRecorder()

	// Must not panic on an unparseable id; the mapper falls back to the zero UUID.
	require.NotPanics(t, func() { router.ServeHTTP(rec, req) })

	require.Equal(t, http.StatusOK, rec.Code)
	frames := sseFrames(t, rec.Body.String())
	require.Len(t, frames, 1)
	variation, ok := frames[0]["variation"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "00000000-0000-0000-0000-000000000000", variation["id"])
}

func TestGenerateVariations_ClosesOnClientDisconnect(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockStreamer := mockhttpapi.NewMockstreamer(ctrl)

	events := make(chan domain.GenerationEvent)
	gen := fixtureGeneration()
	mockStreamer.EXPECT().Execute(gomock.Any(), gomock.Any()).Return((<-chan domain.GenerationEvent)(events), nil)

	router := NewRouter(NewServer(nil, mockStreamer, nil, nil))

	ctx, cancel := context.WithCancel(context.Background())
	req := httptest.NewRequestWithContext(ctx, http.MethodPost, "/v1/generate", bytes.NewBufferString(validGenerateBody))
	rec := httptest.NewRecorder()

	done := make(chan struct{})
	go func() {
		router.ServeHTTP(rec, req)
		close(done)
	}()

	// The unbuffered send only returns once the handler's select has
	// received it, proving the handler is inside its read loop before
	// cancellation happens.
	events <- domain.GenerationEvent{Type: domain.GenerationEventStarted, Generation: &gen}
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("handler did not return after client disconnect; goroutine leaked")
	}
}
