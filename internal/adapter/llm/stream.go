package llm

import (
	"context"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// sendChunk sends chunk on out, but gives up if ctx is done first. Without
// this, a StreamStructured consumer that stops draining after ctx is
// canceled (e.g. an SSE handler returning on a dropped client) would leave
// the producer goroutine blocked forever on an unbuffered send. It reports
// whether the send happened.
func sendChunk(ctx context.Context, out chan<- domain.StreamChunk, chunk domain.StreamChunk) bool {
	select {
	case out <- chunk:
		return true
	case <-ctx.Done():
		return false
	}
}
