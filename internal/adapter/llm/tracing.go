package llm

import (
	"context"
	"fmt"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	semconv "go.opentelemetry.io/otel/semconv/v1.41.0"
	"go.opentelemetry.io/otel/trace"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// TracingProvider wraps a domain.LLMProvider with one span per call, so a
// generation's trace shows LLM latency alongside the HTTP and pgx spans the
// same request already produces. It is a decorator, not a new concrete
// provider: cmd/api wraps whichever provider llm.NewProvider selected
// (Anthropic in dev, Groq in prod) around it, leaving the concrete adapters
// and their existing tests untouched.
type TracingProvider struct {
	next     domain.LLMProvider
	provider string
	tracer   trace.Tracer
}

// NewTracingProvider wraps next, tagging every span with providerName (e.g.
// "anthropic", "groq") so a trace shows which backend served a given call.
func NewTracingProvider(next domain.LLMProvider, providerName string) *TracingProvider {
	return &TracingProvider{
		next:     next,
		provider: providerName,
		tracer:   otel.Tracer("github.com/rodvieira/pricing-optimizer-api/internal/adapter/llm"),
	}
}

// GenerateStructured implements domain.LLMProvider.
func (p *TracingProvider) GenerateStructured(ctx context.Context, in domain.GenerationInput) (*domain.Variation, error) {
	ctx, span := p.tracer.Start(ctx, "llm.generate_structured", trace.WithAttributes(
		semconv.GenAIProviderNameKey.String(p.provider),
		attribute.String("pricing.strategy", string(in.Strategy)),
	))
	defer span.End()

	variation, err := p.next.GenerateStructured(ctx, in)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("generate structured: %w", err)
	}
	return variation, nil
}

// StreamStructured implements domain.LLMProvider. The span covers only the
// call that opens the stream, matching the interface's own contract that
// StreamStructured returns as soon as the channel exists; the terminal
// variation_completed/error chunk is the use case's concern, not this
// adapter's, so it is not traced here.
func (p *TracingProvider) StreamStructured(ctx context.Context, in domain.GenerationInput) (<-chan domain.StreamChunk, error) {
	ctx, span := p.tracer.Start(ctx, "llm.stream_structured", trace.WithAttributes(
		semconv.GenAIProviderNameKey.String(p.provider),
		attribute.String("pricing.strategy", string(in.Strategy)),
	))
	defer span.End()

	chunks, err := p.next.StreamStructured(ctx, in)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("stream structured: %w", err)
	}
	return chunks, nil
}

// ClassifySite implements domain.LLMProvider.
func (p *TracingProvider) ClassifySite(ctx context.Context, page domain.ScrapedPage) (*domain.SiteProfile, error) {
	ctx, span := p.tracer.Start(ctx, "llm.classify_site", trace.WithAttributes(
		semconv.GenAIProviderNameKey.String(p.provider),
	))
	defer span.End()

	profile, err := p.next.ClassifySite(ctx, page)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return nil, fmt.Errorf("classify site: %w", err)
	}
	return profile, nil
}
