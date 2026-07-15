package usecase

import (
	"encoding/json"
	"log/slog"
	"strings"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// stripePricingTable is a simplified Stripe Pricing Table config: one
// Stripe-Price-shaped entry per PricingTier. Per the contract's own
// description, no Stripe API calls are made here; this is data for the
// caller to hand to Stripe's dashboard/API themselves, not a live Stripe
// object.
type stripePricingTable struct {
	Prices []stripePrice `json:"prices"`
}

type stripePrice struct {
	Product stripeProduct `json:"product"`

	// UnitAmount/Currency/Recurring are set for a fixed-price tier;
	// CustomLabel is set instead for a tier whose Price carries a
	// CustomLabel ("Contact us") rather than an amount, mirroring
	// PricingTier.validate's own either/or invariant.
	UnitAmount  *int             `json:"unit_amount,omitempty"`
	Currency    string           `json:"currency,omitempty"`
	Recurring   *stripeRecurring `json:"recurring,omitempty"`
	CustomLabel string           `json:"custom_label,omitempty"`
}

type stripeProduct struct {
	Name              string   `json:"name"`
	Description       string   `json:"description,omitempty"`
	MarketingFeatures []string `json:"marketing_features,omitempty"`
}

type stripeRecurring struct {
	Interval string `json:"interval"`
}

// renderStripe renders v as a stripePricingTable.
func renderStripe(v domain.Variation) *domain.ExportResult {
	table := stripePricingTable{Prices: make([]stripePrice, len(v.Tiers))}
	for i, t := range v.Tiers {
		table.Prices[i] = stripePriceFor(t)
	}

	content, err := json.MarshalIndent(table, "", "  ")
	if err != nil {
		// table is composed entirely of strings, ints, and slices thereof:
		// json.Marshal cannot fail on it. Log rather than propagate, the
		// same degrade-not-crash choice renderHTML makes.
		slog.Error("export variation: marshal stripe config", "error", err)
	}

	return &domain.ExportResult{
		Format:      domain.ExportFormatStripe,
		Filename:    "stripe-pricing-table.json",
		ContentType: "application/json",
		Content:     string(content),
	}
}

func stripePriceFor(t domain.PricingTier) stripePrice {
	price := stripePrice{
		Product: stripeProduct{
			Name:              t.Name,
			Description:       t.Tagline,
			MarketingFeatures: t.Features,
		},
	}

	if t.Price.CustomLabel != "" {
		price.CustomLabel = t.Price.CustomLabel
		return price
	}

	amount := t.Price.AmountMinorUnits
	price.UnitAmount = &amount
	price.Currency = strings.ToLower(t.Price.Currency)
	switch t.Price.Interval {
	case domain.IntervalMonthly:
		price.Recurring = &stripeRecurring{Interval: "month"}
	case domain.IntervalYearly:
		price.Recurring = &stripeRecurring{Interval: "year"}
	}
	return price
}
