package domain

import "errors"

var (
	// ErrProviderUnavailable indicates a transient LLMProvider failure (rate
	// limit, timeout, 5xx). Callers may retry or fail over to another provider.
	ErrProviderUnavailable = errors.New("llm provider unavailable")

	// ErrProviderUnauthorized indicates the provider rejected the configured
	// credentials. Not retryable without a configuration change.
	ErrProviderUnauthorized = errors.New("llm provider rejected credentials")

	// ErrInvalidLLMResponse indicates the provider returned output that does
	// not match the expected structured tool-call schema.
	ErrInvalidLLMResponse = errors.New("llm returned an invalid structured response")

	// ErrInvalidGenerationInput indicates a GenerationInput failed validation
	// before any LLMProvider call was attempted.
	ErrInvalidGenerationInput = errors.New("invalid generation input")

	// ErrEmptyScrape indicates a Scraper could not extract meaningful content
	// from a URL.
	ErrEmptyScrape = errors.New("scrape produced no usable content")

	// ErrSiteUnreachable indicates a Scraper call failed for any reason (a
	// network failure, a non-2xx response, ErrEmptyScrape, ...). AnalyzeSite
	// wraps every scrape failure with this sentinel in addition to the
	// underlying cause, so callers that only care about "the target site
	// could not be fetched or parsed" (e.g. the HTTP layer choosing a status
	// code) can check for it with errors.Is without needing to know every
	// concrete failure mode a Scraper implementation might produce.
	ErrSiteUnreachable = errors.New("could not fetch or parse the target site")

	// ErrInvalidInput indicates a use case's input failed validation before
	// any external call (LLM, scraper, repository) was attempted. Lives in
	// domain, not usecase, so adapters (e.g. the HTTP layer mapping errors to
	// status codes) can classify it via errors.Is without importing usecase.
	ErrInvalidInput = errors.New("invalid input")
)
