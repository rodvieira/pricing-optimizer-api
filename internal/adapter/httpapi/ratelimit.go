package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

//go:generate go tool mockgen -source=ratelimit.go -destination=../../../test/mocks/httpapi/rate_limiter_mock.go -package=mockhttpapi

// rateLimiter is the minimal capability the rate-limited handlers need.
// Defined here, at the point of consumption, mirroring analyzer/streamer/
// generationGetter/exporter: cmd wires the concrete cache.RedisRateLimiter
// in, which satisfies this interface structurally.
type rateLimiter interface {
	Allow(ctx context.Context, key string) (allowed bool, retryAfter time.Duration, err error)
}

// checkRateLimit enforces limiter for the caller identified by r's client IP
// on POST /v1/analyze and POST /v1/generate — the two endpoints
// openapi.yaml documents a 429 response for, since they're the ones that
// spend real LLM/scraper budget per call. It writes the 429 Retry-After
// response and returns false when the caller is over budget; callers must
// return immediately without doing any further work.
//
// A nil limiter (not configured, e.g. most handler tests that don't
// exercise this path) always allows. A limiter error — the Redis backing it
// being unreachable, for instance — also allows rather than rejecting: a
// cache outage must not take down the two endpoints it's meant to protect,
// only remove the protection until it recovers.
func checkRateLimit(w http.ResponseWriter, r *http.Request, limiter rateLimiter) bool {
	if limiter == nil {
		return true
	}

	allowed, retryAfter, err := limiter.Allow(r.Context(), middleware.GetClientIP(r.Context()))
	if err != nil {
		// Not fatal: a rateLimiter is expected to fail open on its own
		// (cache.RedisRateLimiter does), so reaching here at all already
		// means the caller is being let through despite the failure.
		slog.WarnContext(r.Context(), "rate limit check failed, allowing the request", "error", err)
		return true
	}
	if !allowed {
		// Retry-After is documented as whole seconds; ceil rather than
		// truncate so a sub-second remainder never rounds down to 0 and
		// tells a rejected caller to retry immediately. The only current
		// rateLimiter implementation already returns whole seconds, but the
		// interface doesn't promise that, so this handler doesn't rely on it.
		retryAfterSeconds := int(retryAfter.Seconds())
		if retryAfter%time.Second != 0 {
			retryAfterSeconds++
		}
		w.Header().Set("Retry-After", strconv.Itoa(retryAfterSeconds))
		writeProblem(w, r, http.StatusTooManyRequests, "rate limit exceeded", "")
		return false
	}
	return true
}
