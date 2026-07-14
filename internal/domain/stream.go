package domain

// StreamChunkType discriminates the payload of a StreamChunk.
type StreamChunkType string

const (
	// StreamChunkToken carries an incremental delta of the model's structured
	// output as it is generated.
	StreamChunkToken StreamChunkType = "token"
	// StreamChunkVariationCompleted is the terminal success chunk, carrying the
	// fully decoded and validated Variation.
	StreamChunkVariationCompleted StreamChunkType = "variation_completed"
	// StreamChunkError is the terminal failure chunk.
	StreamChunkError StreamChunkType = "error"
)

// StreamChunk is one incremental event from a single LLMProvider.StreamStructured
// call — one provider, one strategy.
//
// It is distinct from the generated api.StreamChunk, which is the SSE-level
// event for the whole /v1/generate response aggregating all requested
// strategies: the use case that fans out N parallel StreamStructured calls
// (Sprint 4/5) maps their StreamChunk channels into that aggregate sequence.
type StreamChunk struct {
	Type      StreamChunkType
	Delta     string
	Variation *Variation
	Err       error
}
