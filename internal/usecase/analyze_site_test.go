package usecase_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	"github.com/rodvieira/pricing-optimizer-api/internal/usecase"
	mockdomain "github.com/rodvieira/pricing-optimizer-api/test/mocks/domain"
)

func fixtureScrapedPage() *domain.ScrapedPage {
	return &domain.ScrapedPage{
		URL:        "https://example.com",
		Title:      "Acme Analytics",
		Text:       "Real-time analytics for indie SaaS founders.",
		SourceType: domain.SourceTypeStatic,
	}
}

var errScrapeBoom = errors.New("scrape boom")

func TestAnalyzeSite_Execute(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		url     string
		setup   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider)
		wantErr error
	}{
		{
			name: "scrapes then classifies the page",
			url:  "https://example.com",
			setup: func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {
				scraper.EXPECT().
					Scrape(gomock.Any(), "https://example.com").
					Return(fixtureScrapedPage(), nil)
				provider.EXPECT().
					ClassifySite(gomock.Any(), *fixtureScrapedPage()).
					Return(&fixtureAnalyzedProfile, nil)
			},
		},
		{
			name:    "empty url is rejected before any call",
			url:     "",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "malformed url is rejected before any call",
			url:     "http://[::1",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "non-http(s) scheme is rejected before any call",
			url:     "ftp://example.com/file",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "url with no host is rejected before any call",
			url:     "http://",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "localhost is rejected before any call",
			url:     "http://localhost:8080/admin",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "a localhost subdomain is rejected before any call",
			url:     "http://internal.localhost/",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "a loopback IP literal is rejected before any call",
			url:     "http://127.0.0.1/",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name:    "a private-range IP literal is rejected before any call",
			url:     "http://10.0.0.5/",
			setup:   func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "the cloud metadata link-local IP literal is rejected before any call",
			url:  "http://169.254.169.254/latest/meta-data/",
			setup: func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {
			},
			wantErr: domain.ErrInvalidInput,
		},
		{
			name: "scraper failure short-circuits classification",
			url:  "https://example.com",
			setup: func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {
				scraper.EXPECT().
					Scrape(gomock.Any(), "https://example.com").
					Return(nil, errScrapeBoom)
			},
			wantErr: errScrapeBoom,
		},
		{
			name: "classification failure propagates",
			url:  "https://example.com",
			setup: func(scraper *mockdomain.MockScraper, provider *mockdomain.MockLLMProvider) {
				scraper.EXPECT().
					Scrape(gomock.Any(), "https://example.com").
					Return(fixtureScrapedPage(), nil)
				provider.EXPECT().
					ClassifySite(gomock.Any(), *fixtureScrapedPage()).
					Return(nil, domain.ErrInvalidLLMResponse)
			},
			wantErr: domain.ErrInvalidLLMResponse,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			ctrl := gomock.NewController(t)
			scraper := mockdomain.NewMockScraper(ctrl)
			provider := mockdomain.NewMockLLMProvider(ctrl)
			tt.setup(scraper, provider)

			uc := usecase.NewAnalyzeSite(scraper, provider)
			got, err := uc.Execute(context.Background(), tt.url)

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, fixtureAnalyzedProfile, *got)
		})
	}
}

func TestAnalyzeSite_Execute_ScraperFailureIsClassifiedUnreachable(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	scraper := mockdomain.NewMockScraper(ctrl)
	provider := mockdomain.NewMockLLMProvider(ctrl)

	scraper.EXPECT().
		Scrape(gomock.Any(), "https://example.com").
		Return(nil, errScrapeBoom)

	uc := usecase.NewAnalyzeSite(scraper, provider)
	_, err := uc.Execute(context.Background(), "https://example.com")

	// Both the concrete scraper failure and the domain-level classification
	// must be discoverable: callers that only know about the underlying
	// scraper error (tests) and callers that only know about the shared HTTP
	// classification (the httpapi layer, mapping this to a 502) both need
	// errors.Is to succeed.
	require.ErrorIs(t, err, errScrapeBoom)
	require.ErrorIs(t, err, domain.ErrSiteUnreachable)
}

func TestAnalyzeSite_Execute_PropagatesContext(t *testing.T) {
	t.Parallel()

	ctrl := gomock.NewController(t)
	scraper := mockdomain.NewMockScraper(ctrl)
	provider := mockdomain.NewMockLLMProvider(ctrl)

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	scraper.EXPECT().
		Scrape(gomock.Any(), "https://example.com").
		DoAndReturn(func(ctx context.Context, _ string) (*domain.ScrapedPage, error) {
			<-ctx.Done()
			return nil, ctx.Err()
		})

	uc := usecase.NewAnalyzeSite(scraper, provider)
	_, err := uc.Execute(ctx, "https://example.com")

	require.ErrorIs(t, err, context.DeadlineExceeded)
}

var fixtureAnalyzedProfile = domain.SiteProfile{
	URL:              "https://example.com",
	Title:            "Acme Analytics",
	ValueProposition: "Real-time analytics for indie SaaS founders",
	Industry:         "developer-tools",
	Audience: domain.Audience{
		Segment:        "SaaS founders",
		Sophistication: domain.SophisticationMedium,
		PricePosition:  domain.PricePositionMidMarket,
	},
	SourceType: domain.SourceTypeStatic,
}
