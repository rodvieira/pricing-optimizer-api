package domain

import "time"

// GenerationStatus is the lifecycle state of a Generation.
type GenerationStatus string

const (
	GenerationStatusPending   GenerationStatus = "pending"
	GenerationStatusStreaming GenerationStatus = "streaming"
	GenerationStatusCompleted GenerationStatus = "completed"
	GenerationStatusFailed    GenerationStatus = "failed"
)

// Valid reports whether s is one of the known generation statuses.
func (s GenerationStatus) Valid() bool {
	switch s {
	case GenerationStatusPending, GenerationStatusStreaming, GenerationStatusCompleted, GenerationStatusFailed:
		return true
	default:
		return false
	}
}

// Generation is the persisted record of one POST /v1/generate call: the
// source URL and site profile it was generated from, its variations (empty
// until Status reaches completed), and when it was created. Created by the
// use case that orchestrates streaming generation, saved via GenerationRepo,
// and later fetched by GET /v1/generations/{id} or referenced by
// POST /v1/export/{id}.
type Generation struct {
	ID          string
	SourceURL   string
	SiteProfile SiteProfile
	Status      GenerationStatus
	Variations  []Variation
	CreatedAt   time.Time
}
