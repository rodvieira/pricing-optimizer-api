package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	mockhttpapi "github.com/rodvieira/pricing-optimizer-api/test/mocks/httpapi"
)

func TestCheckRateLimit(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		setup          func(l *mockhttpapi.MockrateLimiter)
		nilLimiter     bool
		wantOK         bool
		wantStatus     int
		wantRetryAfter string
	}{
		{
			name:       "nil limiter always allows",
			nilLimiter: true,
			wantOK:     true,
		},
		{
			name: "allowed request proceeds",
			setup: func(l *mockhttpapi.MockrateLimiter) {
				l.EXPECT().Allow(gomock.Any(), gomock.Any()).Return(true, time.Duration(0), nil)
			},
			wantOK: true,
		},
		{
			name: "over-budget request is rejected with Retry-After",
			setup: func(l *mockhttpapi.MockrateLimiter) {
				l.EXPECT().Allow(gomock.Any(), gomock.Any()).Return(false, 30*time.Second, nil)
			},
			wantOK:         false,
			wantStatus:     http.StatusTooManyRequests,
			wantRetryAfter: "30",
		},
		{
			name: "limiter error fails open",
			setup: func(l *mockhttpapi.MockrateLimiter) {
				l.EXPECT().Allow(gomock.Any(), gomock.Any()).Return(false, time.Duration(0), errors.New("redis unreachable"))
			},
			wantOK: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			var limiter rateLimiter
			if !tt.nilLimiter {
				ctrl := gomock.NewController(t)
				mockLimiter := mockhttpapi.NewMockrateLimiter(ctrl)
				tt.setup(mockLimiter)
				limiter = mockLimiter
			}

			req := httptest.NewRequestWithContext(context.Background(), http.MethodPost, "/v1/analyze", nil)
			req.RemoteAddr = "203.0.113.1:54321"
			rec := httptest.NewRecorder()

			got := checkRateLimit(rec, req, limiter)

			assert.Equal(t, tt.wantOK, got)
			if tt.wantOK {
				assert.Empty(t, rec.Body.Bytes(), "checkRateLimit must not write a response when allowing")
				return
			}

			require.Equal(t, tt.wantStatus, rec.Code)
			assert.Equal(t, tt.wantRetryAfter, rec.Header().Get("Retry-After"))
			assert.Equal(t, "application/problem+json", rec.Header().Get("Content-Type"))
		})
	}
}
