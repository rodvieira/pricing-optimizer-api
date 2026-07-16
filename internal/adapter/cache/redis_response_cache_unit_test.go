package cache_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
	mockcache "github.com/rodvieira/pricing-optimizer-api/test/mocks/cache"
)

// These tests run in CI (unlike redis_response_cache_test.go's real-
// container tests, gated on testing.Short()): they fake the
// redisCacheClient dependency, so there is nothing here that needs a real
// Redis.

func TestRedisResponseCache_Get_Miss(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisCacheClient(ctrl)

	cmd := redis.NewStringCmd(context.Background())
	cmd.SetErr(redis.Nil)
	client.EXPECT().Get(gomock.Any(), "response-cache:missing-key").Return(cmd)

	c := cache.NewRedisResponseCache(client, time.Hour)
	_, err := c.Get(context.Background(), "missing-key")

	require.ErrorIs(t, err, cache.ErrResponseCacheMiss)
}

func TestRedisResponseCache_Get_ClientError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisCacheClient(ctrl)

	cmd := redis.NewStringCmd(context.Background())
	cmd.SetErr(errors.New("connection refused"))
	client.EXPECT().Get(gomock.Any(), "response-cache:some-key").Return(cmd)

	c := cache.NewRedisResponseCache(client, time.Hour)
	_, err := c.Get(context.Background(), "some-key")

	require.Error(t, err)
	require.NotErrorIs(t, err, cache.ErrResponseCacheMiss,
		"a real connection error must not be mistaken for a cache miss")
}

func TestRedisResponseCache_Set_ClientError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisCacheClient(ctrl)

	cmd := redis.NewStatusCmd(context.Background())
	cmd.SetErr(errors.New("connection refused"))
	client.EXPECT().Set(gomock.Any(), "response-cache:some-key", "cached-value", time.Hour).Return(cmd)

	c := cache.NewRedisResponseCache(client, time.Hour)
	err := c.Set(context.Background(), "some-key", "cached-value")

	require.Error(t, err)
}
