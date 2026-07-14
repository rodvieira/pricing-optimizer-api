package domain

// Interval is the billing cadence for a Price.
type Interval string

const (
	IntervalOneTime Interval = "one_time"
	IntervalMonthly Interval = "monthly"
	IntervalYearly  Interval = "yearly"
)

// Valid reports whether i is one of the known billing intervals.
func (i Interval) Valid() bool {
	switch i {
	case IntervalOneTime, IntervalMonthly, IntervalYearly:
		return true
	default:
		return false
	}
}

// Price is one tier's cost. AmountMinorUnits is in the currency's minor unit
// (e.g. cents); 0 means free. CustomLabel, when set, overrides the rendered
// price (e.g. "Contact us") and takes precedence over Amount/Currency.
type Price struct {
	AmountMinorUnits int
	Currency         string
	Interval         Interval
	CustomLabel      string
}

// PricingTier is one plan within a Variation.
type PricingTier struct {
	Name        string
	Price       Price
	Tagline     string
	Features    []string
	CTA         string
	Highlighted bool
	Badge       string
}
