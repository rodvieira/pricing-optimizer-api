// Package cache implements rate limiting (and, later, response caching)
// backed by Redis.
package cache

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

//go:generate go tool mockgen -destination=../../../test/mocks/cache/redis_scripter_mock.go -package=mockcache github.com/redis/go-redis/v9 Scripter

// allowScript increments key's counter and, on the call that creates it
// (count == 1), sets its expiry to windowMs milliseconds — so later calls in
// the same window never push the expiry back out. It runs as a single Redis
// EVAL rather than separate INCR/PEXPIRE/PTTL round trips specifically so
// the increment-then-expire pair is atomic: two sequential commands from Go
// could leave a key incremented but never expired if the process crashed or
// ctx was canceled in between, silently locking that key out forever once
// it crossed the limit. Returns {allowed (0/1), remaining TTL in ms}.
var allowScript = redis.NewScript(`
local count = redis.call("INCR", KEYS[1])
if count == 1 then
	redis.call("PEXPIRE", KEYS[1], ARGV[2])
end
if count > tonumber(ARGV[1]) then
	local ttl = redis.call("PTTL", KEYS[1])
	if ttl < 0 then
		ttl = tonumber(ARGV[2])
	end
	return {0, ttl}
end
return {1, 0}
`)

// RedisRateLimiter is a fixed-window rate limiter: each key gets a counter
// that increments per call and expires after window, so the count resets to
// zero at the start of every window rather than sliding continuously. This
// trades precision at window boundaries (a caller can burst up to 2x limit
// across a boundary) for a single round trip per call, which is the right
// tradeoff for the MVP's traffic (per project decision).
type RedisRateLimiter struct {
	client redis.Scripter
	limit  int
	window time.Duration
}

// NewRedisRateLimiter creates a limiter allowing at most limit calls per
// window for any given key. client is redis.Scripter (not the concrete
// *redis.Client) so a *redis.Client, *redis.Ring, or *redis.ClusterClient
// all satisfy it, and it can be faked in tests.
func NewRedisRateLimiter(client redis.Scripter, limit int, window time.Duration) *RedisRateLimiter {
	return &RedisRateLimiter{client: client, limit: limit, window: window}
}

// Allow reports whether the caller identified by key may proceed. When it
// may not, retryAfter is how long until the current window ends (rounded up
// to the nearest second, matching the integer-seconds Retry-After header the
// contract documents).
func (l *RedisRateLimiter) Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error) {
	res, err := allowScript.Run(ctx, l.client, []string{"ratelimit:" + key}, l.limit, l.window.Milliseconds()).Result()
	if err != nil {
		return false, 0, fmt.Errorf("rate limiter: eval: %w", err)
	}

	values, ok := res.([]interface{})
	if !ok || len(values) != 2 {
		return false, 0, fmt.Errorf("rate limiter: unexpected script result %v", res)
	}
	allowedCount, ok := values[0].(int64)
	if !ok {
		return false, 0, fmt.Errorf("rate limiter: unexpected script result %v", res)
	}
	ttlMs, ok := values[1].(int64)
	if !ok {
		return false, 0, fmt.Errorf("rate limiter: unexpected script result %v", res)
	}

	if allowedCount == 1 {
		return true, 0, nil
	}
	return false, ceilSecond(time.Duration(ttlMs) * time.Millisecond), nil
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
