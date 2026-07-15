package usecase

import (
	"fmt"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// currencySymbols covers currencies whose minor unit is 1/100th of the
// major one, matching Price.AmountMinorUnits' own "e.g. cents" assumption;
// anything else (including zero-decimal currencies like JPY, which this
// package does not attempt to format correctly) falls back to its ISO code.
var currencySymbols = map[string]string{
	"USD": "$",
	"EUR": "€",
	"GBP": "£",
	"BRL": "R$",
}

// formatPrice renders p as a short display string for the JSX/HTML exports,
// e.g. "$29/month" or "$0". A CustomLabel ("Contact us") always takes
// precedence, per Price's own doc comment.
func formatPrice(p domain.Price) string {
	if p.CustomLabel != "" {
		return p.CustomLabel
	}

	symbol, ok := currencySymbols[p.Currency]
	if !ok {
		symbol = p.Currency + " "
	}

	var amount string
	if p.AmountMinorUnits%100 == 0 {
		amount = fmt.Sprintf("%d", p.AmountMinorUnits/100)
	} else {
		amount = fmt.Sprintf("%.2f", float64(p.AmountMinorUnits)/100)
	}

	switch p.Interval {
	case domain.IntervalMonthly:
		return fmt.Sprintf("%s%s/month", symbol, amount)
	case domain.IntervalYearly:
		return fmt.Sprintf("%s%s/year", symbol, amount)
	default:
		return symbol + amount
	}
}
