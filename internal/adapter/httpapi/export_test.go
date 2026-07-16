package httpapi

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
)

const (
	exportGenerationID = "b6f1c6b2-6b8a-4b1a-9f1a-1c2c3c4c5c6c"
	exportVariationID  = "a1a1a1a1-1111-1111-1111-111111111111"
)

func fixtureExportResult() domain.ExportResult {
	return domain.ExportResult{
		Format:      domain.ExportFormatJSX,
		Filename:    "PricingSection.tsx",
		ContentType: "text/plain",
		Content:     "export default function PricingSection() { /* ... */ }",
	}
}

func TestExportVariation(t *testing.T) {
	t.Parallel()

	validBody := fmt.Sprintf(`{"variationId":%q,"format":"jsx"}`, exportVariationID)

	tests := []struct {
		name            string
		body            string
		setup           func(e *mockhttpapi.Mockexporter)
		wantStatus      int
		wantContent     string
		wantDetailEmpty bool
	}{
		{
			name: "valid request is exported and mapped to the response shape",
			body: validBody,
			setup: func(e *mockhttpapi.Mockexporter) {
				result := fixtureExportResult()
				e.EXPECT().
					Execute(gomock.Any(), domain.ExportVariationInput{
						GenerationID: exportGenerationID, VariationID: exportVariationID, Format: domain.ExportFormatJSX,
					}).
					Return(&result, nil)
			},
			wantStatus:  http.StatusOK,
			wantContent: "application/json",
		},
		{
			name:        "malformed json body is a bad request",
			body:        `{"variationId":`,
			setup:       func(e *mockhttpapi.Mockexporter) {},
			wantStatus:  http.StatusBadRequest,
			wantContent: "application/problem+json",
		},
		{
			name: "invalid format is a bad request, not unprocessable",
			body: fmt.Sprintf(`{"variationId":%q,"format":"pdf"}`, exportVariationID),
			setup: func(e *mockhttpapi.Mockexporter) {
				e.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("%w: invalid export format %q", domain.ErrInvalidInput, "pdf"))
			},
			wantStatus:  http.StatusBadRequest,
			wantContent: "application/problem+json",
		},
		{
			name: "unknown generation is not found",
			body: validBody,
			setup: func(e *mockhttpapi.Mockexporter) {
				e.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("export variation: %w", domain.ErrGenerationNotFound))
			},
			wantStatus:  http.StatusNotFound,
			wantContent: "application/problem+json",
		},
		{
			name: "unknown variation is not found",
			body: validBody,
			setup: func(e *mockhttpapi.Mockexporter) {
				e.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("export variation: %w", domain.ErrVariationNotFound))
			},
			wantStatus:  http.StatusNotFound,
			wantContent: "application/problem+json",
		},
		{
			name: "repo failure maps to internal error without leaking the internal error",
			body: validBody,
			setup: func(e *mockhttpapi.Mockexporter) {
				e.EXPECT().
					Execute(gomock.Any(), gomock.Any()).
					Return(nil, fmt.Errorf("export variation: connection refused"))
			},
			wantStatus:      http.StatusInternalServerError,
			wantContent:     "application/problem+json",
			wantDetailEmpty: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			mockExporter := mockhttpapi.NewMockexporter(ctrl)
			tt.setup(mockExporter)

			router := NewRouter(NewServer(nil, nil, nil, mockExporter, nil, nil))

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost,
				"/v1/export/"+exportGenerationID, bytes.NewBufferString(tt.body))
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantContent, rec.Header().Get("Content-Type"))

			if tt.wantStatus == http.StatusOK {
				var body map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, "jsx", body["format"])
				assert.Equal(t, "PricingSection.tsx", body["filename"])
				assert.Equal(t, "text/plain", body["contentType"])
				assert.NotEmpty(t, body["content"])
				return
			}

			var problem map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &problem))
			assert.EqualValues(t, tt.wantStatus, problem["status"])
			assert.NotEmpty(t, problem["title"])
			if tt.wantDetailEmpty {
				assert.NotContains(t, problem, "detail", "must not leak the internal dependency error to the client")
			}
		})
	}
}
