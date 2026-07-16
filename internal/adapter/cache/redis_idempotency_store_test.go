package cache_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
)

func TestRedisIdempotencyStore_SaveThenGet(t *testing.T) {
	client := newTestRedis(t)
	store := cache.NewRedisIdempotencyStore(client, time.Minute)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, "some-key", "generation-1"))

	generationID, err := store.Get(ctx, "some-key")
	require.NoError(t, err)
	assert.Equal(t, "generation-1", generationID)
}

func TestRedisIdempotencyStore_Get_UnusedKeyIsNotFound(t *testing.T) {
	client := newTestRedis(t)
	store := cache.NewRedisIdempotencyStore(client, time.Minute)

	_, err := store.Get(context.Background(), "never-used")

	require.ErrorIs(t, err, cache.ErrIdempotencyKeyNotFound)
}

func TestRedisIdempotencyStore_Get_ExpiredKeyIsNotFound(t *testing.T) {
	client := newTestRedis(t)
	store := cache.NewRedisIdempotencyStore(client, 2*time.Second)
	ctx := context.Background()

	require.NoError(t, store.Save(ctx, "expiring-key", "generation-1"))

	time.Sleep(3 * time.Second)

	_, err := store.Get(ctx, "expiring-key")
	require.ErrorIs(t, err, cache.ErrIdempotencyKeyNotFound)
}

func TestRedisIdempotencyStore_Release_AllowsAFreshReserve(t *testing.T) {
	client := newTestRedis(t)
	store := cache.NewRedisIdempotencyStore(client, time.Minute)
	ctx := context.Background()

	reserved, err := store.Reserve(ctx, "some-key")
	require.NoError(t, err)
	require.True(t, reserved)

	require.NoError(t, store.Release(ctx, "some-key"))

	_, err = store.Get(ctx, "some-key")
	require.ErrorIs(t, err, cache.ErrIdempotencyKeyNotFound, "Release must fully clear the key, not just unblock it")

	reservedAgain, err := store.Reserve(ctx, "some-key")
	require.NoError(t, err)
	assert.True(t, reservedAgain, "a released key must be reservable again")
}

func TestRedisIdempotencyStore_Reserve_OnlyOneOfManyConcurrentCallersWins(t *testing.T) {
	// This is the scenario Reserve exists for: two requests racing the same
	// Idempotency-Key (a double-click, or a client retry while the first
	// attempt is still in flight) must not both start a generation. SETNX
	// makes this a single atomic Redis command, so exactly one caller among
	// any number racing the same key gets true.
	client := newTestRedis(t)
	store := cache.NewRedisIdempotencyStore(client, time.Minute)
	ctx := context.Background()

	const callers = 20
	results := make([]bool, callers)
	var wg sync.WaitGroup
	for i := range callers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			reserved, err := store.Reserve(ctx, "racing-key")
			require.NoError(t, err)
			results[i] = reserved
		}(i)
	}
	wg.Wait()

	wins := 0
	for _, r := range results {
		if r {
			wins++
		}
	}
	assert.Equal(t, 1, wins, "exactly one of %d concurrent Reserve calls on the same key must win", callers)
}

func TestRedisIdempotencyStore_Reserve_ThenSaveOverwritesThePendingSentinel(t *testing.T) {
	client := newTestRedis(t)
	store := cache.NewRedisIdempotencyStore(client, time.Minute)
	ctx := context.Background()

	reserved, err := store.Reserve(ctx, "some-key")
	require.NoError(t, err)
	require.True(t, reserved)

	_, err = store.Get(ctx, "some-key")
	require.ErrorIs(t, err, cache.ErrIdempotencyKeyPending, "before Save, the key must read back as pending")

	require.NoError(t, store.Save(ctx, "some-key", "generation-1"))

	generationID, err := store.Get(ctx, "some-key")
	require.NoError(t, err)
	assert.Equal(t, "generation-1", generationID)
}

func TestRedisIdempotencyStore_Reserve_SecondCallerSeesPendingUntilSaved(t *testing.T) {
	client := newTestRedis(t)
	store := cache.NewRedisIdempotencyStore(client, time.Minute)
	ctx := context.Background()

	reserved, err := store.Reserve(ctx, "some-key")
	require.NoError(t, err)
	require.True(t, reserved, "first caller must win the reservation")

	reservedAgain, err := store.Reserve(ctx, "some-key")
	require.NoError(t, err)
	assert.False(t, reservedAgain, "a second caller must not also win the same key")
}
