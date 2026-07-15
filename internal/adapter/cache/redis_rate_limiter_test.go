package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcredis "github.com/testcontainers/testcontainers-go/modules/redis"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
)

// newTestRedis starts a real Redis container (this package's integration
// tests exercise the actual INCR/EXPIRE/TTL semantics the rate limiter
// depends on, rather than a fake): matches the project's own "verify
// empirically" testing philosophy already applied to the Postgres and
// chromedp adapters.
func newTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	if testing.Short() {
		t.Skip("skipping Redis container test in short mode: spins up a real container")
	}

	ctx := context.Background()
	container, err := tcredis.Run(ctx, "redis:7-alpine")
	require.NoError(t, err)
	t.Cleanup(func() {
		require.NoError(t, container.Terminate(context.Background()))
	})

	connStr, err := container.ConnectionString(ctx)
	require.NoError(t, err)

	opts, err := redis.ParseURL(connStr)
	require.NoError(t, err)

	client := redis.NewClient(opts)
	t.Cleanup(func() { _ = client.Close() })
	return client
}

func TestRedisRateLimiter_Allow(t *testing.T) {
	client := newTestRedis(t)
	limiter := cache.NewRedisRateLimiter(client, 3, time.Minute)
	ctx := context.Background()

	for i := 1; i <= 3; i++ {
		allowed, retryAfter, err := limiter.Allow(ctx, "same-key")
		require.NoError(t, err)
		assert.True(t, allowed, "call %d of 3 should be allowed", i)
		assert.Zero(t, retryAfter)
	}

	allowed, retryAfter, err := limiter.Allow(ctx, "same-key")
	require.NoError(t, err)
	assert.False(t, allowed, "4th call over a limit of 3 should be rejected")
	assert.Greater(t, retryAfter, time.Duration(0))
	assert.LessOrEqual(t, retryAfter, time.Minute)
}

func TestRedisRateLimiter_Allow_SeparateKeysHaveSeparateBudgets(t *testing.T) {
	client := newTestRedis(t)
	limiter := cache.NewRedisRateLimiter(client, 1, time.Minute)
	ctx := context.Background()

	allowedA, _, err := limiter.Allow(ctx, "caller-a")
	require.NoError(t, err)
	assert.True(t, allowedA)

	allowedB, _, err := limiter.Allow(ctx, "caller-b")
	require.NoError(t, err)
	assert.True(t, allowedB, "a different key must have its own budget")

	allowedA2, _, err := limiter.Allow(ctx, "caller-a")
	require.NoError(t, err)
	assert.False(t, allowedA2, "caller-a already spent its one allowed call")
}

func TestRedisRateLimiter_Allow_WindowResetsAfterExpiry(t *testing.T) {
	client := newTestRedis(t)
	limiter := cache.NewRedisRateLimiter(client, 1, 2*time.Second)
	ctx := context.Background()

	allowed, _, err := limiter.Allow(ctx, "expiring-key")
	require.NoError(t, err)
	require.True(t, allowed)

	blocked, _, err := limiter.Allow(ctx, "expiring-key")
	require.NoError(t, err)
	require.False(t, blocked)

	time.Sleep(3 * time.Second)

	allowedAgain, _, err := limiter.Allow(ctx, "expiring-key")
	require.NoError(t, err)
	assert.True(t, allowedAgain, "a new window must reset the budget")
}
