// Package domain contains the core entities and ports of Pricing Optimizer. It
// imports nothing outside the standard library: business rules stay isolated
// from HTTP, LLM SDKs, and storage.
package domain

// PricingStrategy is a psychological pricing strategy applied to a variation.
type PricingStrategy string

const (
	StrategyAnchor     PricingStrategy = "anchor"
	StrategyFreemium   PricingStrategy = "freemium"
	StrategyValueBased PricingStrategy = "value_based"
)

// Valid reports whether s is one of the known pricing strategies.
func (s PricingStrategy) Valid() bool {
	switch s {
	case StrategyAnchor, StrategyFreemium, StrategyValueBased:
		return true
	default:
		return false
	}
}
