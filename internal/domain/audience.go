package domain

// Sophistication is the price/technical sophistication of an audience segment.
type Sophistication string

const (
	SophisticationLow    Sophistication = "low"
	SophisticationMedium Sophistication = "medium"
	SophisticationHigh   Sophistication = "high"
)

// PricePosition is the market position a site's pricing occupies.
type PricePosition string

const (
	PricePositionBudget    PricePosition = "budget"
	PricePositionMidMarket PricePosition = "mid_market"
	PricePositionPremium   PricePosition = "premium"
)

// Audience describes who a product targets, classified during site analysis.
type Audience struct {
	Segment        string
	Sophistication Sophistication
	PricePosition  PricePosition
}
