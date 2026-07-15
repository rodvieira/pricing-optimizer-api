package domain

import "testing"

func TestGenerationEventType_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		t    GenerationEventType
		want bool
	}{
		{name: "generation_started is valid", t: GenerationEventStarted, want: true},
		{name: "variation_started is valid", t: GenerationEventVariationStarted, want: true},
		{name: "token is valid", t: GenerationEventToken, want: true},
		{name: "variation_completed is valid", t: GenerationEventVariationCompleted, want: true},
		{name: "done is valid", t: GenerationEventDone, want: true},
		{name: "error is valid", t: GenerationEventError, want: true},
		{name: "empty string is invalid", t: "", want: false},
		{name: "unknown event type is invalid", t: "progress", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.t.Valid(); got != tt.want {
				t.Errorf("GenerationEventType(%q).Valid() = %v, want %v", tt.t, got, tt.want)
			}
		})
	}
}
