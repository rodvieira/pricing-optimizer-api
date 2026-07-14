package domain

import "testing"

func TestInterval_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		i    Interval
		want bool
	}{
		{name: "one_time is valid", i: IntervalOneTime, want: true},
		{name: "monthly is valid", i: IntervalMonthly, want: true},
		{name: "yearly is valid", i: IntervalYearly, want: true},
		{name: "empty string is invalid", i: "", want: false},
		{name: "unknown interval is invalid", i: "annual", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.i.Valid(); got != tt.want {
				t.Errorf("Interval(%q).Valid() = %v, want %v", tt.i, got, tt.want)
			}
		})
	}
}
