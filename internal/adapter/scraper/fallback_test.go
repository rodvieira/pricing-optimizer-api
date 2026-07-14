package scraper

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	mockdomain "github.com/rodvieira/pricing-optimizer-api/test/mocks/domain"
)

var errBoom = errors.New("boom")

func TestFallbackScraper_Scrape(t *testing.T) {
	t.Parallel()

	longText := strings.Repeat("word ", 100) // well above minStaticTextLength

	tests := []struct {
		name    string
		url     string
		setup   func(static, browser *mockdomain.MockScraper)
		wantErr error
	}{
		{
			name: "sufficient static result skips the browser",
			url:  "https://example.com",
			setup: func(static, browser *mockdomain.MockScraper) {
				static.EXPECT().
					Scrape(gomock.Any(), "https://example.com").
					Return(&domain.ScrapedPage{URL: "https://example.com", Text: longText}, nil)
				browser.EXPECT().Scrape(gomock.Any(), gomock.Any()).Times(0)
			},
		},
		{
			name: "thin static result falls back to the browser",
			url:  "https://spa.example.com",
			setup: func(static, browser *mockdomain.MockScraper) {
				static.EXPECT().
					Scrape(gomock.Any(), "https://spa.example.com").
					Return(&domain.ScrapedPage{URL: "https://spa.example.com", Text: "short"}, nil)
				browser.EXPECT().
					Scrape(gomock.Any(), "https://spa.example.com").
					Return(&domain.ScrapedPage{URL: "https://spa.example.com", Text: longText}, nil)
			},
		},
		{
			name: "failed static scrape falls back to the browser",
			url:  "https://example.com",
			setup: func(static, browser *mockdomain.MockScraper) {
				static.EXPECT().Scrape(gomock.Any(), gomock.Any()).Return(nil, errBoom)
				browser.EXPECT().
					Scrape(gomock.Any(), gomock.Any()).
					Return(&domain.ScrapedPage{URL: "https://example.com", Text: longText}, nil)
			},
		},
		{
			name: "both static and browser failing is an error",
			url:  "https://example.com",
			setup: func(static, browser *mockdomain.MockScraper) {
				static.EXPECT().Scrape(gomock.Any(), gomock.Any()).Return(nil, errBoom)
				browser.EXPECT().Scrape(gomock.Any(), gomock.Any()).Return(nil, errBoom)
			},
			wantErr: errBoom,
		},
		{
			name: "thin static result whose browser fallback also fails is an error",
			url:  "https://example.com",
			setup: func(static, browser *mockdomain.MockScraper) {
				static.EXPECT().
					Scrape(gomock.Any(), gomock.Any()).
					Return(&domain.ScrapedPage{Text: "short"}, nil)
				browser.EXPECT().Scrape(gomock.Any(), gomock.Any()).Return(nil, errBoom)
			},
			wantErr: errBoom,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			static := mockdomain.NewMockScraper(ctrl)
			browser := mockdomain.NewMockScraper(ctrl)
			tt.setup(static, browser)

			s := NewFallbackScraper(static, browser)
			page, err := s.Scrape(context.Background(), tt.url)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, page)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tt.url, page.URL)
		})
	}
}
