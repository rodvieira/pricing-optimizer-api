package domain

// GenerationEventType discriminates the payload of a GenerationEvent,
// mirroring the SSE event types openapi.yaml documents for /v1/generate.
type GenerationEventType string

const (
	GenerationEventStarted            GenerationEventType = "generation_started"
	GenerationEventVariationStarted   GenerationEventType = "variation_started"
	GenerationEventToken              GenerationEventType = "token"
	GenerationEventVariationCompleted GenerationEventType = "variation_completed"
	GenerationEventDone               GenerationEventType = "done"
	GenerationEventError              GenerationEventType = "error"
)

// Valid reports whether t is one of the known generation event types.
func (t GenerationEventType) Valid() bool {
	switch t {
	case GenerationEventStarted, GenerationEventVariationStarted, GenerationEventToken,
		GenerationEventVariationCompleted, GenerationEventDone, GenerationEventError:
		return true
	default:
		return false
	}
}

// GenerationEvent is one item in the aggregate SSE sequence for a whole
// POST /v1/generate call. It is distinct from StreamChunk, which is scoped
// to a single LLMProvider.StreamStructured call (one strategy): the use case
// that fans out one StreamStructured call per requested strategy multiplexes
// their StreamChunk channels into a single ordered channel of
// GenerationEvent, tagged with which Strategy (if any) each event concerns,
// for the HTTP layer to translate into SSE frames.
type GenerationEvent struct {
	Type       GenerationEventType
	Strategy   PricingStrategy
	Delta      string
	Variation  *Variation
	Generation *Generation
	Err        error
}
