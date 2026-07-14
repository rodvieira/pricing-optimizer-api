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
)
