package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHealthEndpoint(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{
			name:       "GET healthz returns 200 ok",
			method:     http.MethodGet,
			path:       "/v1/healthz",
			wantStatus: http.StatusOK,
		},
		{
			name:       "POST healthz is not allowed",
			method:     http.MethodPost,
			path:       "/v1/healthz",
			wantStatus: http.StatusMethodNotAllowed,
		},
		{
			name:       "unknown route returns 404",
			method:     http.MethodGet,
			path:       "/v1/does-not-exist",
			wantStatus: http.StatusNotFound,
		},
	}

	router := NewRouter()

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			req := httptest.NewRequestWithContext(context.Background(), tt.method, tt.path, nil)
			rec := httptest.NewRecorder()

			router.ServeHTTP(rec, req)

			require.Equal(t, tt.wantStatus, rec.Code)

			if tt.wantStatus != http.StatusOK {
				return
			}

			require.Equal(t, "application/json", rec.Header().Get("Content-Type"))

			var got healthStatus
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &got))
			assert.Equal(t, "ok", got.Status)
			assert.NotEmpty(t, got.Version)
		})
	}
}
