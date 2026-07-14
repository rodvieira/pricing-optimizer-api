package domain

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestScrapedPage_Validate(t *testing.T) {
	t.Parallel()

	valid := func() ScrapedPage {
		return ScrapedPage{
			URL:        "https://example.com",
			Title:      "Acme Analytics",
			Text:       "Real-time analytics for indie SaaS founders.",
			SourceType: SourceTypeStatic,
		}
	}

	tests := []struct {
		name    string
		mutate  func(p *ScrapedPage)
		wantErr error
	}{
		{name: "valid page passes", mutate: func(p *ScrapedPage) {}},
		{
			name: "missing url is rejected",
			mutate: func(p *ScrapedPage) {
				p.URL = ""
			},
			wantErr: ErrEmptyScrape,
		},
		{
			name: "empty text is rejected",
			mutate: func(p *ScrapedPage) {
				p.Text = ""
			},
			wantErr: ErrEmptyScrape,
		},
		{
			name: "whitespace-only text is rejected",
			mutate: func(p *ScrapedPage) {
				p.Text = "   \n\t  "
			},
			wantErr: ErrEmptyScrape,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := valid()
			tt.mutate(&p)

			err := p.Validate()

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}
			assert.NoError(t, err)
		})
	}
}
