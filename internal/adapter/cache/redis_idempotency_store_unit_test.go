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

// These tests run in CI (unlike redis_idempotency_store_test.go's real-
// container tests, gated on testing.Short()): they fake the redisGetSetter
// dependency, so there is nothing here that needs a real Redis.

func TestRedisIdempotencyStore_Save_ClientError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisGetSetter(ctrl)

	cmd := redis.NewStatusCmd(context.Background())
	cmd.SetErr(errors.New("connection refused"))
	client.EXPECT().Set(gomock.Any(), "idempotency:some-key", "gen-1", time.Hour).Return(cmd)

	store := cache.NewRedisIdempotencyStore(client, time.Hour)
	err := store.Save(context.Background(), "some-key", "gen-1")

	require.Error(t, err)
}

func TestRedisIdempotencyStore_Get_NotFound(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisGetSetter(ctrl)

	cmd := redis.NewStringCmd(context.Background())
	cmd.SetErr(redis.Nil)
	client.EXPECT().Get(gomock.Any(), "idempotency:missing-key").Return(cmd)

	store := cache.NewRedisIdempotencyStore(client, time.Hour)
	_, err := store.Get(context.Background(), "missing-key")

	require.ErrorIs(t, err, cache.ErrIdempotencyKeyNotFound)
}

func TestRedisIdempotencyStore_Get_ClientError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisGetSetter(ctrl)

	cmd := redis.NewStringCmd(context.Background())
	cmd.SetErr(errors.New("connection refused"))
	client.EXPECT().Get(gomock.Any(), "idempotency:some-key").Return(cmd)

	store := cache.NewRedisIdempotencyStore(client, time.Hour)
	_, err := store.Get(context.Background(), "some-key")

	require.Error(t, err)
	assert.NotErrorIs(t, err, cache.ErrIdempotencyKeyNotFound,
		"a real connection error must not be mistaken for a not-found key")
}

func TestRedisIdempotencyStore_Get_Pending(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisGetSetter(ctrl)

	cmd := redis.NewStringCmd(context.Background())
	cmd.SetVal("pending")
	client.EXPECT().Get(gomock.Any(), "idempotency:reserved-key").Return(cmd)

	store := cache.NewRedisIdempotencyStore(client, time.Hour)
	_, err := store.Get(context.Background(), "reserved-key")

	require.ErrorIs(t, err, cache.ErrIdempotencyKeyPending)
}

func TestRedisIdempotencyStore_Release_ClientError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisGetSetter(ctrl)

	cmd := redis.NewIntCmd(context.Background())
	cmd.SetErr(errors.New("connection refused"))
	client.EXPECT().Del(gomock.Any(), "idempotency:some-key").Return(cmd)

	store := cache.NewRedisIdempotencyStore(client, time.Hour)
	err := store.Release(context.Background(), "some-key")

	require.Error(t, err)
}

func TestRedisIdempotencyStore_Reserve_ClientError(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	client := mockcache.NewMockredisGetSetter(ctrl)

	cmd := redis.NewBoolCmd(context.Background())
	cmd.SetErr(errors.New("connection refused"))
	client.EXPECT().SetNX(gomock.Any(), "idempotency:some-key", "pending", time.Hour).Return(cmd)

	store := cache.NewRedisIdempotencyStore(client, time.Hour)
	_, err := store.Reserve(context.Background(), "some-key")

	require.Error(t, err)
}

func TestRedisIdempotencyStore_Reserve_ReturnsSetNXResult(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		want bool
	}{
		{name: "key was free", want: true},
		{name: "key already claimed", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			client := mockcache.NewMockredisGetSetter(ctrl)

			cmd := redis.NewBoolCmd(context.Background())
			cmd.SetVal(tt.want)
			client.EXPECT().SetNX(gomock.Any(), "idempotency:some-key", "pending", time.Hour).Return(cmd)

			store := cache.NewRedisIdempotencyStore(client, time.Hour)
			reserved, err := store.Reserve(context.Background(), "some-key")

			require.NoError(t, err)
			assert.Equal(t, tt.want, reserved)
		})
	}
}
