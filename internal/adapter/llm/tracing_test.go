package llm

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel/codes"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	"go.uber.org/mock/gomock"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
	mockdomain "github.com/rodvieira/pricing-optimizer-api/test/mocks/domain"
	"github.com/rodvieira/pricing-optimizer-api/test/otelrecorder"
)

func validGenerationInput() domain.GenerationInput {
	return domain.GenerationInput{
		SiteProfile: domain.SiteProfile{URL: "https://example.com"},
		Strategy:    domain.StrategyAnchor,
		Currency:    "USD",
	}
}

func TestTracingProvider_GenerateStructured(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	t.Run("success produces an unset-status span tagged with provider and strategy", func(t *testing.T) {
		recorder := otelrecorder.WithRecordingTracerProvider(t)
		ctrl := gomock.NewController(t)
		next := mockdomain.NewMockLLMProvider(ctrl)
		want := &domain.Variation{Strategy: domain.StrategyAnchor}
		next.EXPECT().GenerateStructured(gomock.Any(), gomock.Any()).Return(want, nil)

		provider := NewTracingProvider(next, "anthropic")
		got, err := provider.GenerateStructured(context.Background(), validGenerationInput())

		require.NoError(t, err)
		assert.Same(t, want, got)
		spans := recorder.Ended()
		require.Len(t, spans, 1)
		assert.Equal(t, "llm.generate_structured", spans[0].Name())
		assert.Equal(t, sdktrace.Status{Code: codes.Unset}, spans[0].Status())
		otelrecorder.AssertHasAttribute(t, spans[0], "gen_ai.provider.name", "anthropic")
		otelrecorder.AssertHasAttribute(t, spans[0], "pricing.strategy", "anchor")
	})

	t.Run("failure records the error and sets the span status to Error", func(t *testing.T) {
		recorder := otelrecorder.WithRecordingTracerProvider(t)
		ctrl := gomock.NewController(t)
		next := mockdomain.NewMockLLMProvider(ctrl)
		wantErr := errors.New("boom")
		next.EXPECT().GenerateStructured(gomock.Any(), gomock.Any()).Return(nil, wantErr)

		provider := NewTracingProvider(next, "groq")
		_, err := provider.GenerateStructured(context.Background(), validGenerationInput())

		require.ErrorIs(t, err, wantErr)
		spans := recorder.Ended()
		require.Len(t, spans, 1)
		assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "boom"}, spans[0].Status())
		require.Len(t, spans[0].Events(), 1, "RecordError must add an exception event")
	})
}

func TestTracingProvider_StreamStructured(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	t.Run("success produces an unset-status span", func(t *testing.T) {
		recorder := otelrecorder.WithRecordingTracerProvider(t)
		ctrl := gomock.NewController(t)
		next := mockdomain.NewMockLLMProvider(ctrl)
		ch := make(chan domain.StreamChunk)
		next.EXPECT().StreamStructured(gomock.Any(), gomock.Any()).Return((<-chan domain.StreamChunk)(ch), nil)

		provider := NewTracingProvider(next, "anthropic")
		got, err := provider.StreamStructured(context.Background(), validGenerationInput())

		require.NoError(t, err)
		assert.NotNil(t, got)
		spans := recorder.Ended()
		require.Len(t, spans, 1)
		assert.Equal(t, "llm.stream_structured", spans[0].Name())
		assert.Equal(t, sdktrace.Status{Code: codes.Unset}, spans[0].Status())
		otelrecorder.AssertHasAttribute(t, spans[0], "gen_ai.provider.name", "anthropic")
	})

	t.Run("failure records the error and sets the span status to Error", func(t *testing.T) {
		recorder := otelrecorder.WithRecordingTracerProvider(t)
		ctrl := gomock.NewController(t)
		next := mockdomain.NewMockLLMProvider(ctrl)
		wantErr := errors.New("boom")
		next.EXPECT().StreamStructured(gomock.Any(), gomock.Any()).Return(nil, wantErr)

		provider := NewTracingProvider(next, "groq")
		_, err := provider.StreamStructured(context.Background(), validGenerationInput())

		require.ErrorIs(t, err, wantErr)
		spans := recorder.Ended()
		require.Len(t, spans, 1)
		assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "boom"}, spans[0].Status())
		require.Len(t, spans[0].Events(), 1, "RecordError must add an exception event")
	})
}

func TestTracingProvider_ClassifySite(t *testing.T) {
	// Not t.Parallel(): mutates the process-global TracerProvider.
	t.Run("success produces an unset-status span", func(t *testing.T) {
		recorder := otelrecorder.WithRecordingTracerProvider(t)
		ctrl := gomock.NewController(t)
		next := mockdomain.NewMockLLMProvider(ctrl)
		page := domain.ScrapedPage{URL: "https://example.com", Text: "content"}
		want := &domain.SiteProfile{URL: page.URL}
		next.EXPECT().ClassifySite(gomock.Any(), page).Return(want, nil)

		provider := NewTracingProvider(next, "groq")
		got, err := provider.ClassifySite(context.Background(), page)

		require.NoError(t, err)
		assert.Same(t, want, got)
		spans := recorder.Ended()
		require.Len(t, spans, 1)
		assert.Equal(t, "llm.classify_site", spans[0].Name())
		assert.Equal(t, sdktrace.Status{Code: codes.Unset}, spans[0].Status())
		otelrecorder.AssertHasAttribute(t, spans[0], "gen_ai.provider.name", "groq")
	})

	t.Run("failure records the error and sets the span status to Error", func(t *testing.T) {
		recorder := otelrecorder.WithRecordingTracerProvider(t)
		ctrl := gomock.NewController(t)
		next := mockdomain.NewMockLLMProvider(ctrl)
		page := domain.ScrapedPage{URL: "https://example.com", Text: "content"}
		wantErr := errors.New("boom")
		next.EXPECT().ClassifySite(gomock.Any(), page).Return(nil, wantErr)

		provider := NewTracingProvider(next, "anthropic")
		_, err := provider.ClassifySite(context.Background(), page)

		require.ErrorIs(t, err, wantErr)
		spans := recorder.Ended()
		require.Len(t, spans, 1)
		assert.Equal(t, sdktrace.Status{Code: codes.Error, Description: "boom"}, spans[0].Status())
		require.Len(t, spans[0].Events(), 1, "RecordError must add an exception event")
	})
}
