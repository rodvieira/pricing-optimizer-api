package usecase

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func TestFormatPrice(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    domain.Price
		want string
	}{
		{
			name: "custom label takes precedence over amount",
			p:    domain.Price{CustomLabel: "Contact us", AmountMinorUnits: 999, Currency: "USD"},
			want: "Contact us",
		},
		{
			name: "whole dollar amount renders without cents",
			p:    domain.Price{AmountMinorUnits: 2900, Currency: "USD", Interval: domain.IntervalMonthly},
			want: "$29/month",
		},
		{
			name: "fractional amount renders with cents",
			p:    domain.Price{AmountMinorUnits: 2950, Currency: "USD", Interval: domain.IntervalMonthly},
			want: "$29.50/month",
		},
		{
			name: "yearly interval",
			p:    domain.Price{AmountMinorUnits: 29900, Currency: "USD", Interval: domain.IntervalYearly},
			want: "$299/year",
		},
		{
			name: "one-time interval has no suffix",
			p:    domain.Price{AmountMinorUnits: 9900, Currency: "USD", Interval: domain.IntervalOneTime},
			want: "$99",
		},
		{
			name: "unknown currency falls back to its ISO code",
			p:    domain.Price{AmountMinorUnits: 1000, Currency: "CHF", Interval: domain.IntervalMonthly},
			want: "CHF 10/month",
		},
		{
			name: "zero amount is free",
			p:    domain.Price{AmountMinorUnits: 0, Currency: "USD", Interval: domain.IntervalMonthly},
			want: "$0/month",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, formatPrice(tt.p))
		})
	}
}
