package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
)

func fixtureSiteProfile() domain.SiteProfile {
	return domain.SiteProfile{
		URL:              "https://example.com",
		Title:            "Acme Analytics",
		ValueProposition: "Real-time analytics for indie SaaS founders",
		Industry:         "developer-tools",
		Audience: domain.Audience{
			Segment:        "SaaS founders",
			Sophistication: domain.SophisticationMedium,
			PricePosition:  domain.PricePositionMidMarket,
		},
		Keywords:   []string{"analytics", "saas"},
		SourceType: domain.SourceTypeStatic,
		AnalyzedAt: time.Date(2026, 7, 14, 12, 0, 0, 0, time.UTC),
	}
}

func TestAnalyzeSite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		body        string
		setup       func(a *mockhttpapi.Mockanalyzer)
		wantStatus  int
		wantContent string
	}{
		{
			name: "valid url is analyzed and mapped to the response shape",
			body: `{"url":"https://example.com"}`,
			setup: func(a *mockhttpapi.Mockanalyzer) {
				profile := fixtureSiteProfile()
				a.EXPECT().
					Execute(gomock.Any(), "https://example.com").
					Return(&profile, nil)
			},
			wantStatus:  http.StatusOK,
			wantContent: "application/json",
		},
		{
			name:        "malformed json body is a bad request",
			body:        `{"url":`,
			setup:       func(a *mockhttpapi.Mockanalyzer) {},
			wantStatus:  http.StatusBadRequest,
			wantContent: "application/problem+json",
		},
		{
			name: "invalid input is unprocessable",
			body: `{"url":"http://localhost/"}`,
			setup: func(a *mockhttpapi.Mockanalyzer) {
				a.EXPECT().
					Execute(gomock.Any(), "http://localhost/").
					Return(nil, domain.ErrInvalidInput)
			},
			wantStatus:  http.StatusUnprocessableEntity,
			wantContent: "application/problem+json",
		},
		{
			name: "unreachable site maps to bad gateway",
			body: `{"url":"https://example.com"}`,
			setup: func(a *mockhttpapi.Mockanalyzer) {
				a.EXPECT().
					Execute(gomock.Any(), "https://example.com").
					Return(nil, domain.ErrSiteUnreachable)
			},
			wantStatus:  http.StatusBadGateway,
			wantContent: "application/problem+json",
		},
		{
			name: "provider failure maps to internal error",
			body: `{"url":"https://example.com"}`,
			setup: func(a *mockhttpapi.Mockanalyzer) {
				a.EXPECT().
					Execute(gomock.Any(), "https://example.com").
					Return(nil, domain.ErrProviderUnavailable)
			},
			wantStatus:  http.StatusInternalServerError,
			wantContent: "application/problem+json",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockAnalyzer := mockhttpapi.NewMockanalyzer(ctrl)
			tt.setup(mockAnalyzer)

			router := NewRouter(NewServer(mockAnalyzer))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze", bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantContent, rec.Header().Get("Content-Type"))

			if tt.wantStatus == http.StatusOK {
				var body map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "https://example.com", body["url"])
				assert.Equal(t, "developer-tools", body["industry"])
				assert.Equal(t, "static", body["sourceType"])
				keywords, ok := body["keywords"].([]any)
				require.True(t, ok, "keywords must be present when non-empty")
				assert.Len(t, keywords, 2)
				return
			}

			var problem map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			assert.EqualValues(t, tt.wantStatus, problem["status"])
			assert.NotEmpty(t, problem["title"])
		})
	}
}

func TestAnalyzeSite_OmitsPricePositionWhenEmpty(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockAnalyzer := mockhttpapi.NewMockanalyzer(ctrl)
	profile := fixtureSiteProfile()
	profile.Audience.PricePosition = ""
	mockAnalyzer.EXPECT().Execute(gomock.Any(), "https://example.com").Return(&profile, nil)

	router := NewRouter(NewServer(mockAnalyzer))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	audience, ok := body["audience"].(map[string]any)
	require.True(t, ok)
	assert.NotContains(t, audience, "pricePosition", "pricePosition must be omitted when the classifier didn't set one")
}

func TestAnalyzeSite_OmitsKeywordsWhenEmpty(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	mockAnalyzer := mockhttpapi.NewMockanalyzer(ctrl)
	profile := fixtureSiteProfile()
	profile.Keywords = nil
	mockAnalyzer.EXPECT().Execute(gomock.Any(), "https://example.com").Return(&profile, nil)

	router := NewRouter(NewServer(mockAnalyzer))

	req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze",
		bytes.NewBufferString(`{"url":"https://example.com"}`))
	rec := httptest.NewRecorder()

	router.ServeHTTP(rec, req)

	require.Equal(t, http.StatusOK, rec.Code)

	var body map[string]any
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
	assert.NotContains(t, body, "keywords", "keywords must be omitted when empty per the contract")
}
