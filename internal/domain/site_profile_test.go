package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSourceType_Valid(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		s    SourceType
		want bool
	}{
		{name: "spa is valid", s: SourceTypeSPA, want: true},
		{name: "static is valid", s: SourceTypeStatic, want: true},
		{name: "empty string is invalid", s: "", want: false},
		{name: "unknown source type is invalid", s: "server_rendered", want: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			if got := tt.s.Valid(); got != tt.want {
				t.Errorf("SourceType(%q).Valid() = %v, want %v", tt.s, got, tt.want)
			}
		})
	}
}

func validSiteProfile() SiteProfile {
	return SiteProfile{
		URL:              "https://example.com",
		Title:            "Acme Analytics",
		ValueProposition: "Real-time analytics for indie SaaS founders",
		Industry:         "developer-tools",
		Audience: Audience{
			Segment:        "SaaS founders",
			Sophistication: SophisticationMedium,
			PricePosition:  PricePositionMidMarket,
		},
		Keywords:   []string{"analytics", "saas"},
		SourceType: SourceTypeStatic,
		AnalyzedAt: time.Now(),
	}
}

func TestSiteProfile_Validate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		mutate  func(p *SiteProfile)
		wantErr error
	}{
		{
			name:   "valid site profile passes",
			mutate: func(p *SiteProfile) {},
		},
		{
			name: "missing url is rejected",
			mutate: func(p *SiteProfile) {
				p.URL = ""
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "empty title is accepted (comes from the scrape, not the model)",
			mutate: func(p *SiteProfile) {
				p.Title = ""
			},
		},
		{
			name: "missing value proposition is rejected",
			mutate: func(p *SiteProfile) {
				p.ValueProposition = ""
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "missing industry is rejected",
			mutate: func(p *SiteProfile) {
				p.Industry = ""
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "missing audience segment is rejected",
			mutate: func(p *SiteProfile) {
				p.Audience.Segment = ""
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "invalid audience sophistication is rejected",
			mutate: func(p *SiteProfile) {
				p.Audience.Sophistication = "extreme"
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "invalid audience price position is rejected",
			mutate: func(p *SiteProfile) {
				p.Audience.PricePosition = "luxury"
			},
			wantErr: ErrInvalidLLMResponse,
		},
		{
			name: "invalid source type is rejected",
			mutate: func(p *SiteProfile) {
				p.SourceType = "server_rendered"
			},
			wantErr: ErrInvalidLLMResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			p := validSiteProfile()
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
