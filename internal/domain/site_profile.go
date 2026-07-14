package domain

import "time"

// SourceType records which scraper produced a SiteProfile.
type SourceType string

const (
	SourceTypeSPA    SourceType = "spa"
	SourceTypeStatic SourceType = "static"
)

// SiteProfile is the result of analyzing a product URL: its value
// proposition, classification, and target audience.
type SiteProfile struct {
	URL              string
	Title            string
	ValueProposition string
	Industry         string
	Audience         Audience
	Keywords         []string
	SourceType       SourceType
	AnalyzedAt       time.Time
}
