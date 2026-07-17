package httpapi

import (
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

func TestGetGeneration(t *testing.T) {
	t.Parallel()

	const id = "b6f1c6b2-6b8a-4b1a-9f1a-1c2c3c4c5c6c"

	tests := []struct {
		name            string
		setup           func(g *mockhttpapi.MockgenerationGetter)
		wantStatus      int
		wantContent     string
		wantDetailEmpty bool
	}{
		{
			name: "existing generation is fetched and mapped to the response shape",
			setup: func(g *mockhttpapi.MockgenerationGetter) {
				gen := fixtureGeneration()
				g.EXPECT().Get(gomock.Any(), id).Return(&gen, nil)
			},
			wantStatus:  http.StatusOK,
			wantContent: "application/json",
		},
		{
			name: "unknown id is not found",
			setup: func(g *mockhttpapi.MockgenerationGetter) {
				g.EXPECT().Get(gomock.Any(), id).Return(nil, fmt.Errorf("%w: %s", domain.ErrGenerationNotFound, id))
			},
			wantStatus:  http.StatusNotFound,
			wantContent: "application/problem+json",
		},
		{
			name: "repo failure maps to internal error without leaking the internal error",
			setup: func(g *mockhttpapi.MockgenerationGetter) {
				g.EXPECT().Get(gomock.Any(), id).Return(nil, fmt.Errorf("get generation: query %s: connection refused", id))
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
			mockGetter := mockhttpapi.NewMockgenerationGetter(ctrl)
			tt.setup(mockGetter)

			router := NewRouter(NewServer(nil, nil, mockGetter, nil, nil, nil, nil), []string{"http://localhost:3000"})

			req := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/v1/generations/"+id, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantContent, rec.Header().Get("Content-Type"))

			if tt.wantStatus == http.StatusOK {
				var body map[string]any
				require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
				assert.Equal(t, id, body["id"])
				assert.Equal(t, "https://example.com", body["sourceUrl"])
				assert.Equal(t, "completed", body["status"])
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
