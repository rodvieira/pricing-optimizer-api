package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
)

func TestAnalyzeSite_Cache_MissAnalyzesAndSaves(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockAnalyzer := mockhttpapi.NewMockanalyzer(ctrl)
	analyzeCache := mockhttpapi.NewMockanalyzeCache(ctrl)

	profile := fixtureSiteProfile()
	analyzeCache.EXPECT().Get(gomock.Any(), "https://example.com").Return("", cache.ErrResponseCacheMiss)
	mockAnalyzer.EXPECT().Execute(gomock.Any(), "https://example.com").Return(&profile, nil)
	analyzeCache.EXPECT().Set(gomock.Any(), "https://example.com", gomock.Any()).
		DoAndReturn(func(_ context.Context, _, value string) error {
			var body map[string]any
			require.NoError(t, json.Unmarshal([]byte(value), &body))
			assert.Equal(t, "https://example.com", body["url"], "the cached value must be the same JSON the client receives")
			return nil
		})

	router := NewRouter(NewServer(mockAnalyzer, nil, nil, nil, nil, nil, analyzeCache), []string{"http://localhost:3000"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAnalyzeSite_Cache_HitSkipsAnalyzeEntirely(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	analyzeCache := mockhttpapi.NewMockanalyzeCache(ctrl)

	cached := `{"url":"https://example.com","title":"Cached Title","valueProposition":"cached","industry":"cached-industry","audience":{"segment":"cached","sophistication":"medium"},"sourceType":"static","analyzedAt":"2026-07-14T12:00:00Z"}`
	analyzeCache.EXPECT().Get(gomock.Any(), "https://example.com").Return(cached, nil)

	// analyzer is nil: a cache hit must never reach it. If it did, calling
	// Execute on a nil interface would panic, which doubles as proof.
	router := NewRouter(NewServer(nil, nil, nil, nil, nil, nil, analyzeCache), []string{"http://localhost:3000"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
	assert.Equal(t, "application/json", rec.Header().Get("Content-Type"))
	assert.JSONEq(t, cached, rec.Body.String())
}

func TestAnalyzeSite_Cache_LookupErrorFailsOpenToAFreshAnalysis(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockAnalyzer := mockhttpapi.NewMockanalyzer(ctrl)
	analyzeCache := mockhttpapi.NewMockanalyzeCache(ctrl)

	profile := fixtureSiteProfile()
	analyzeCache.EXPECT().Get(gomock.Any(), "https://example.com").Return("", errors.New("connection refused"))
	mockAnalyzer.EXPECT().Execute(gomock.Any(), "https://example.com").Return(&profile, nil)
	analyzeCache.EXPECT().Set(gomock.Any(), "https://example.com", gomock.Any()).Return(nil)

	router := NewRouter(NewServer(mockAnalyzer, nil, nil, nil, nil, nil, analyzeCache), []string{"http://localhost:3000"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}

func TestAnalyzeSite_Cache_SaveErrorDoesNotFailTheRequest(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockAnalyzer := mockhttpapi.NewMockanalyzer(ctrl)
	analyzeCache := mockhttpapi.NewMockanalyzeCache(ctrl)

	profile := fixtureSiteProfile()
	analyzeCache.EXPECT().Get(gomock.Any(), "https://example.com").Return("", cache.ErrResponseCacheMiss)
	mockAnalyzer.EXPECT().Execute(gomock.Any(), "https://example.com").Return(&profile, nil)
	analyzeCache.EXPECT().Set(gomock.Any(), "https://example.com", gomock.Any()).Return(errors.New("connection refused"))

	router := NewRouter(NewServer(mockAnalyzer, nil, nil, nil, nil, nil, analyzeCache), []string{"http://localhost:3000"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code, "a cache save failure must not fail a request that otherwise succeeded")
}

func TestAnalyzeSite_Cache_NilCacheNeverTouchesTheStore(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockAnalyzer := mockhttpapi.NewMockanalyzer(ctrl)

	profile := fixtureSiteProfile()
	mockAnalyzer.EXPECT().Execute(gomock.Any(), "https://example.com").Return(&profile, nil)

	router := NewRouter(NewServer(mockAnalyzer, nil, nil, nil, nil, nil, nil), []string{"http://localhost:3000"})

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)
}
