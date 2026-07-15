package domain

import "testing"

func TestGenerationStatus_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    GenerationStatus
		want bool
	}{
		{name: "pending is valid", s: GenerationStatusPending, want: true},
		{name: "streaming is valid", s: GenerationStatusStreaming, want: true},
		{name: "completed is valid", s: GenerationStatusCompleted, want: true},
		{name: "failed is valid", s: GenerationStatusFailed, want: true},
		{name: "empty string is invalid", s: "", want: false},
		{name: "unknown status is invalid", s: "queued", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.s.Valid(); got != tt.want {
				t.Errorf("GenerationStatus(%q).Valid() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}
