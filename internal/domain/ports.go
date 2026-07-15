package domain

import "context"

//go:generate go tool mockgen -source=ports.go -destination=../../test/mocks/domain/ports_mock.go -package=mockdomain

// LLMProvider generates a single pricing-page variation via structured tool
// calling. Implementations must never parse free-form model text: the
// model's output is a typed tool call matching the Variation shape. Selected
// by an env-based factory (adapter/llm); no use case knows the concrete
// provider.
type LLMProvider interface {
	// GenerateStructured produces one complete Variation for the requested
	// strategy in a single call.
	GenerateStructured(ctx context.Context, in GenerationInput) (*Variation, error)

	// StreamStructured produces the same Variation as GenerateStructured, but
	// emits incremental StreamChunk events as the model generates it. The
	// channel is closed after exactly one terminal chunk (variation_completed
	// or error).
	StreamStructured(ctx context.Context, in GenerationInput) (<-chan StreamChunk, error)

	// ClassifySite classifies a scraped page into a SiteProfile in a single
	// call via structured tool calling. There is no streaming variant: only
	// /v1/generate streams over SSE, not /v1/analyze.
	ClassifySite(ctx context.Context, page ScrapedPage) (*SiteProfile, error)
}

// Scraper fetches a URL and extracts its raw content for later
// classification into a SiteProfile. Implementations decide how to render
// the page (a static HTTP fetch vs a real browser for client-rendered SPAs);
// no use case knows the concrete implementation.
type Scraper interface {
	Scrape(ctx context.Context, url string) (*ScrapedPage, error)
}

// GenerationRepo persists and retrieves Generation records created by
// POST /v1/generate, so GET /v1/generations/{id} and POST /v1/export/{id}
// can find them again. Selected by cmd/api's wiring; no use case knows the
// concrete implementation (an in-memory store ahead of Sprint 6's
// Postgres/sqlc-backed one, then the reverse).
type GenerationRepo interface {
	// Save creates or overwrites the Generation at g.ID.
	Save(ctx context.Context, g Generation) error

	// Get fetches the Generation with the given id. Returns
	// ErrGenerationNotFound if none exists.
	Get(ctx context.Context, id string) (*Generation, error)
}
