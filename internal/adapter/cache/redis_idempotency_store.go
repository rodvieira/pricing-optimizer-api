package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:generate go tool mockgen -source=redis_idempotency_store.go -destination=../../../test/mocks/cache/redis_get_setter_mock.go -package=mockcache

// pendingValue is what Reserve stores before the generation it reserved
// key for has produced a real ID. Get returns ErrIdempotencyKeyPending for
// it rather than the sentinel string itself, so callers never need to know
// this implementation detail.
const pendingValue = "pending"

// ErrIdempotencyKeyNotFound indicates no generation is mapped to the given
// key — either it was never used, or its TTL already expired.
var ErrIdempotencyKeyNotFound = errors.New("idempotency key not found")

// ErrIdempotencyKeyPending indicates key was reserved by Reserve but the
// generation it belongs to hasn't produced an ID yet (Save hasn't run) —
// another request with the same key is still being processed.
var ErrIdempotencyKeyPending = errors.New("idempotency key: generation still in progress")

// redisGetSetter is the minimal capability RedisIdempotencyStore needs — a
// small interface defined at the point of consumption (mirroring
// redis.Scripter for RedisRateLimiter) rather than depending on the much
// larger redis.Cmdable, which *redis.Client also satisfies structurally.
type redisGetSetter interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
	SetNX(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.BoolCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
}

// RedisIdempotencyStore maps an Idempotency-Key (POST /v1/generate) to the
// generation ID it produced, so a repeat request with the same key can
// return the existing generation instead of starting a new, LLM-cost-
// spending one.
type RedisIdempotencyStore struct {
	client redisGetSetter
	ttl    time.Duration
}

// NewRedisIdempotencyStore creates a store whose entries expire after ttl.
func NewRedisIdempotencyStore(client redisGetSetter, ttl time.Duration) *RedisIdempotencyStore {
	return &RedisIdempotencyStore{client: client, ttl: ttl}
}

// Reserve atomically claims key for a new generation, returning true if key
// was previously unused (or its prior reservation/mapping expired) and is
// now held by the caller. Returns false — without error — when key is
// already claimed, by this caller's own earlier attempt or a concurrent
// one; the caller must then use Get to find out which. The SETNX this runs
// as a single Redis command is what makes two requests racing the same key
// resolve deterministically: exactly one gets true.
func (s *RedisIdempotencyStore) Reserve(ctx context.Context, key string) (bool, error) {
	reserved, err := s.client.SetNX(ctx, idempotencyRedisKey(key), pendingValue, s.ttl).Result()
	if err != nil {
		return false, fmt.Errorf("idempotency store: reserve: %w", err)
	}
	return reserved, nil
}

// Release undoes a Reserve that never reached Save — the generation it was
// held for failed to start, or its stream ended before producing an ID —
// so a legitimate retry with the same key isn't wrongly rejected as
// "still in progress" for the rest of the TTL window. A plain DEL, not a
// compare-and-delete Lua script like Reserve's SETNX: Release only ever
// runs synchronously, within the same request that just won the Reserve
// moments earlier, so nothing else can have legitimately taken over key in
// between (that would require the multi-hour TTL to have already expired,
// which can't happen within one request's lifetime).
func (s *RedisIdempotencyStore) Release(ctx context.Context, key string) error {
	if err := s.client.Del(ctx, idempotencyRedisKey(key)).Err(); err != nil {
		return fmt.Errorf("idempotency store: release: %w", err)
	}
	return nil
}

// Save records that key (already Reserve'd) produced generationID,
// replacing the pending sentinel and refreshing the TTL to the store's full
// window.
func (s *RedisIdempotencyStore) Save(ctx context.Context, key, generationID string) error {
	if err := s.client.Set(ctx, idempotencyRedisKey(key), generationID, s.ttl).Err(); err != nil {
		return fmt.Errorf("idempotency store: save: %w", err)
	}
	return nil
}

// Get returns the generation ID key was last mapped to. Returns
// ErrIdempotencyKeyNotFound if key is unused or its TTL expired, or
// ErrIdempotencyKeyPending if key was Reserve'd but Save hasn't run yet.
func (s *RedisIdempotencyStore) Get(ctx context.Context, key string) (string, error) {
	generationID, err := s.client.Get(ctx, idempotencyRedisKey(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrIdempotencyKeyNotFound
	}
	if err != nil {
		return "", fmt.Errorf("idempotency store: get: %w", err)
	}
	if generationID == pendingValue {
		return "", ErrIdempotencyKeyPending
	}
	return generationID, nil
}

func idempotencyRedisKey(key string) string {
	return "idempotency:" + key
}
