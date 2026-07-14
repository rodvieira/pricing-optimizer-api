package domain

import "testing"

func TestPricingStrategy_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    PricingStrategy
		want bool
	}{
		{name: "anchor is valid", s: StrategyAnchor, want: true},
		{name: "freemium is valid", s: StrategyFreemium, want: true},
		{name: "value_based is valid", s: StrategyValueBased, want: true},
		{name: "empty string is invalid", s: "", want: false},
		{name: "unknown strategy is invalid", s: "premium_anchor", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.s.Valid(); got != tt.want {
				t.Errorf("PricingStrategy(%q).Valid() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
