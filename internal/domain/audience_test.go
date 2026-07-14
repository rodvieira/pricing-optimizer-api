package domain

import "testing"

func TestSophistication_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    Sophistication
		want bool
	}{
		{name: "low is valid", s: SophisticationLow, want: true},
		{name: "medium is valid", s: SophisticationMedium, want: true},
		{name: "high is valid", s: SophisticationHigh, want: true},
		{name: "empty string is invalid", s: "", want: false},
		{name: "unknown sophistication is invalid", s: "extreme", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.s.Valid(); got != tt.want {
				t.Errorf("Sophistication(%q).Valid() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func TestPricePosition_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		p    PricePosition
		want bool
	}{
		{name: "budget is valid", p: PricePositionBudget, want: true},
		{name: "mid_market is valid", p: PricePositionMidMarket, want: true},
		{name: "premium is valid", p: PricePositionPremium, want: true},
		{name: "empty string is invalid", p: "", want: false},
		{name: "unknown price position is invalid", p: "luxury", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.p.Valid(); got != tt.want {
				t.Errorf("PricePosition(%q).Valid() = %v, want %v", tt.p, got, tt.want)
			}
		})
	}
}
