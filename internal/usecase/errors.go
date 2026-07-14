// Package usecase orchestrates domain logic. It imports domain only — never
// adapters, never external packages beyond the standard library and
// golang.org/x/sync/errgroup.
package usecase

import "errors"

// ErrInvalidInput indicates a use case's input failed validation before any
// external call (LLM, scraper, repository) was attempted.
var ErrInvalidInput = errors.New("invalid input")
