package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
	mockcache "github.com/rodvieira/pricing-optimizer-api/test/mocks/cache"
)

// These tests run in CI (unlike redis_rate_limiter_test.go's real-container
// tests, gated on testing.Short()): they fake the Scripter dependency, so
// there is nothing here that needs a real Redis.

func TestRedisRateLimiter_Allow_ScriptError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	scripter := mockcache.NewMockScripter(ctrl)

	// Script.Run tries EvalSha first (the script's SHA-1 is computed
	// client-side by redis.NewScript); a plain error there is returned
	// as-is, with no fallback to Eval.
	cmd := redis.NewCmd(context.Background())
	cmd.SetErr(errors.New("connection refused"))
	scripter.EXPECT().EvalSha(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(cmd)

	limiter := cache.NewRedisRateLimiter(scripter, 10, time.Minute)
	allowed, retryAfter, err := limiter.Allow(context.Background(), "some-key")

	require.Error(t, err)
	assert.False(t, allowed)
	assert.Zero(t, retryAfter)
}

func TestRedisRateLimiter_Allow_UnexpectedScriptResult(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	scripter := mockcache.NewMockScripter(ctrl)

	cmd := redis.NewCmd(context.Background())
	cmd.SetVal("not the [allowed, ttl] shape the script always returns")
	scripter.EXPECT().EvalSha(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(cmd)

	limiter := cache.NewRedisRateLimiter(scripter, 10, time.Minute)
	allowed, retryAfter, err := limiter.Allow(context.Background(), "some-key")

	require.Error(t, err)
	assert.False(t, allowed)
	assert.Zero(t, retryAfter)
}

func TestCeilSecond(t *testing.T) {
	t.Parallel()

	// ceilSecond is unexported and only reachable through Allow's over-limit
	// path, but its exact rounding behavior is worth locking down directly:
	// Retry-After (openapi.yaml) is an integer number of seconds, and
	// rounding down or to nearest could tell a caller to retry too early.
	tests := []struct {
		name string
		ttl  time.Duration
		want time.Duration
	}{
		{name: "already whole seconds passes through", ttl: 61 * time.Second, want: 61 * time.Second},
		{name: "sub-second remainder rounds up", ttl: 1500 * time.Millisecond, want: 2 * time.Second},
		{name: "just under a second rounds up to one second", ttl: 999 * time.Millisecond, want: time.Second},
		{name: "zero stays zero", ttl: 0, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			// ceilSecond only needs to be reachable via the over-limit path;
			// the fastest way to pin its behavior without exporting it is
			// through a limiter whose script result forces that path, using
			// a fixed known TTL.
			ctrl := gomock.NewController(t)
			scripter := mockcache.NewMockScripter(ctrl)
			cmd := redis.NewCmd(context.Background())
			cmd.SetVal([]interface{}{int64(0), tt.ttl.Milliseconds()})
			scripter.EXPECT().EvalSha(gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any(), gomock.Any()).Return(cmd)

			limiter := cache.NewRedisRateLimiter(scripter, 10, time.Minute)
			allowed, retryAfter, err := limiter.Allow(context.Background(), "some-key")

			require.NoError(t, err)
			assert.False(t, allowed)
			assert.Equal(t, tt.want, retryAfter)
		})
	}
}
