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

	// analyzer is nil: this test only exercises /v1/healthz, which never
	// touches it.
	router := NewRouter(NewServer(nil))

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

			// Assert the response shape against the contract using a raw map so
			// that omitted fields (checks) are actually verified as absent.
			var body map[string]any
			require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &body))
			assert.Equal(t, "ok", body["status"])
			assert.NotEmpty(t, body["version"])
			assert.NotContains(t, body, "checks", "checks must be omitted when empty per the contract")
		})
	}
}
