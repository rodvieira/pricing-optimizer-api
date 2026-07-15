package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestExportFormat_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		f    ExportFormat
		want bool
	}{
		{name: "jsx is valid", f: ExportFormatJSX, want: true},
		{name: "html is valid", f: ExportFormatHTML, want: true},
		{name: "stripe is valid", f: ExportFormatStripe, want: true},
		{name: "empty string is invalid", f: "", want: false},
		{name: "unknown format is invalid", f: "pdf", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			assert.Equal(t, tt.want, tt.f.Valid())
		})
	}
}

func TestExportVariationInput_Validate(t *testing.T) {
	t.Parallel()

	valid := func() ExportVariationInput {
		return ExportVariationInput{
			GenerationID: "b6f1c6b2-6b8a-4b1a-9f1a-1c2c3c4c5c6c",
			VariationID:  "a1a1a1a1-1111-1111-1111-111111111111",
			Format:       ExportFormatJSX,
		}
	}

	tests := []struct {
		name    string
		mutate  func(in *ExportVariationInput)
		wantErr error
	}{
		{
			name:   "valid input passes",
			mutate: func(in *ExportVariationInput) {},
		},
		{
			name: "missing generation id is rejected",
			mutate: func(in *ExportVariationInput) {
				in.GenerationID = ""
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "missing variation id is rejected",
			mutate: func(in *ExportVariationInput) {
				in.VariationID = ""
			},
			wantErr: ErrInvalidInput,
		},
		{
			name: "unknown format is rejected",
			mutate: func(in *ExportVariationInput) {
				in.Format = "pdf"
			},
			wantErr: ErrInvalidInput,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			in := valid()
			tt.mutate(&in)

			err := in.Validate()

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
