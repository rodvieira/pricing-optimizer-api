package cache

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:generate go tool mockgen -source=redis_response_cache.go -destination=../../../test/mocks/cache/redis_cache_client_mock.go -package=mockcache

// ErrResponseCacheMiss indicates key has no cached value — either it was
// never cached, or its TTL already expired.
var ErrResponseCacheMiss = errors.New("response cache: miss")

// redisCacheClient is the minimal capability RedisResponseCache needs — a
// second small interface with the same shape as redisGetSetter's Get/Set
// pair (named distinctly, not reused, since this consumer never needs
// SetNX/Del: RedisResponseCache is a plain cache-aside, no reservation
// semantics like RedisIdempotencyStore's).
type redisCacheClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

// RedisResponseCache is a plain TTL cache-aside keyed on an arbitrary
// caller-supplied string (e.g. a request URL or a content hash), storing
// whatever raw value the caller gives it (a JSON-encoded response body).
// Used by POST /v1/analyze (keyed on the submitted URL) and, indirectly,
// POST /v1/generate's implicit-idempotency-key path (which reuses
// RedisIdempotencyStore instead, since that already has the replay
// machinery a streamed response needs — this type is for the simpler
// synchronous-response case).
type RedisResponseCache struct {
	client redisCacheClient
	ttl    time.Duration
}

// NewRedisResponseCache creates a cache whose entries expire after ttl.
func NewRedisResponseCache(client redisCacheClient, ttl time.Duration) *RedisResponseCache {
	return &RedisResponseCache{client: client, ttl: ttl}
}

// Get returns the cached value for key. Returns ErrResponseCacheMiss if key
// was never cached or its TTL expired.
func (c *RedisResponseCache) Get(ctx context.Context, key string) (string, error) {
	value, err := c.client.Get(ctx, responseCacheRedisKey(key)).Result()
	if errors.Is(err, redis.Nil) {
		return "", ErrResponseCacheMiss
	}
	if err != nil {
		return "", fmt.Errorf("response cache: get: %w", err)
	}
	return value, nil
}

// Set caches value against key for the store's configured TTL.
func (c *RedisResponseCache) Set(ctx context.Context, key, value string) error {
	if err := c.client.Set(ctx, responseCacheRedisKey(key), value, c.ttl).Err(); err != nil {
		return fmt.Errorf("response cache: set: %w", err)
	}
	return nil
}

func responseCacheRedisKey(key string) string {
	return "response-cache:" + key
}
