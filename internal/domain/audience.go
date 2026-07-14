package domain

// Sophistication is the price/technical sophistication of an audience segment.
type Sophistication string

const (
	SophisticationLow    Sophistication = "low"
	SophisticationMedium Sophistication = "medium"
	SophisticationHigh   Sophistication = "high"
)

// Valid reports whether s is one of the known sophistication levels.
func (s Sophistication) Valid() bool {
	switch s {
	case SophisticationLow, SophisticationMedium, SophisticationHigh:
		return true
	default:
		return false
	}
}

// PricePosition is the market position a site's pricing occupies.
type PricePosition string

const (
	PricePositionBudget    PricePosition = "budget"
	PricePositionMidMarket PricePosition = "mid_market"
	PricePositionPremium   PricePosition = "premium"
)

// Valid reports whether p is one of the known price positions.
func (p PricePosition) Valid() bool {
	switch p {
	case PricePositionBudget, PricePositionMidMarket, PricePositionPremium:
		return true
	default:
		return false
	}
}

// Audience describes who a product targets, classified during site analysis.
type Audience struct {
	Segment        string
	Sophistication Sophistication
	PricePosition  PricePosition
}
