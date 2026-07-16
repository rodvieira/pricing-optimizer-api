package cache_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
)

func TestRedisResponseCache_SetThenGet(t *testing.T) {
	client := newTestRedis(t)
	c := cache.NewRedisResponseCache(client, time.Minute)
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "https://example.com", `{"url":"https://example.com"}`))

	value, err := c.Get(ctx, "https://example.com")
	require.NoError(t, err)
	assert.Equal(t, `{"url":"https://example.com"}`, value)
}

func TestRedisResponseCache_Get_UnusedKeyIsAMiss(t *testing.T) {
	client := newTestRedis(t)
	c := cache.NewRedisResponseCache(client, time.Minute)

	_, err := c.Get(context.Background(), "never-cached")

	require.ErrorIs(t, err, cache.ErrResponseCacheMiss)
}

func TestRedisResponseCache_Get_ExpiredKeyIsAMiss(t *testing.T) {
	client := newTestRedis(t)
	c := cache.NewRedisResponseCache(client, 2*time.Second)
	ctx := context.Background()

	require.NoError(t, c.Set(ctx, "expiring-key", "cached-value"))

	time.Sleep(3 * time.Second)

	_, err := c.Get(ctx, "expiring-key")
	require.ErrorIs(t, err, cache.ErrResponseCacheMiss)
}
