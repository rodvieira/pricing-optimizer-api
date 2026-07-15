// Package cache implements rate limiting (and, later, response caching)
// backed by Redis.
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisRateLimiter is a fixed-window rate limiter: each key gets a counter
// that increments per call and expires after window, so the count resets to
// zero at the start of every window rather than sliding continuously. This
// trades precision at window boundaries (a caller can burst up to 2x limit
// across a boundary) for a single INCR+EXPIRE round trip per call, which is
// the right tradeoff for the MVP's traffic (per project decision).
type RedisRateLimiter struct {
	client *redis.Client
	limit  int
	window time.Duration
}

// NewRedisRateLimiter creates a limiter allowing at most limit calls per
// window for any given key.
func NewRedisRateLimiter(client *redis.Client, limit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{client: client, limit: limit, window: window}
}

// Allow reports whether the caller identified by key may proceed. When it
// may not, retryAfter is how long until the current window ends (rounded up
// to the nearest second, matching the integer-seconds Retry-After header the
// contract documents).
func (l *RedisRateLimiter) Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error) {
	redisKey := "ratelimit:" + key

	count, err := l.client.Incr(ctx, redisKey).Result()
	if err != nil {
		return false, 0, fmt.Errorf("rate limiter: incr: %w", err)
	}

	// Only the call that creates the key (count == 1, the first in a new
	// window) sets its expiry: a later call in the same window must not
	// reset the window's remaining time back to the full duration.
	if count == 1 {
		if err := l.client.Expire(ctx, redisKey, l.window).Err(); err != nil {
			return false, 0, fmt.Errorf("rate limiter: expire: %w", err)
		}
	}

	if count <= int64(l.limit) {
		return true, 0, nil
	}

	ttl, err := l.client.TTL(ctx, redisKey).Result()
	if err != nil {
		return false, 0, fmt.Errorf("rate limiter: ttl: %w", err)
	}
	if ttl < 0 {
		// A negative TTL (no expiry, or key vanished between INCR and TTL)
		// should not happen given the count==1 branch above always sets one,
		// but reporting the full window is a safe caller-facing fallback
		// rather than a nonsensical negative Retry-After.
		ttl = l.window
	}
	return false, ceilSecond(ttl), nil
}

// ceilSecond rounds d up to the nearest second: the contract's Retry-After
// is an integer number of seconds, and rounding down (or to nearest) could
// tell a caller to retry slightly before the window actually ends.
func ceilSecond(d time.Duration) time.Duration {
	if d%time.Second == 0 {
		return d
	}
	return (d/time.Second + 1) * time.Second
}
