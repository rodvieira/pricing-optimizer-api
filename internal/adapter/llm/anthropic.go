package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/anthropics/anthropic-sdk-go"
	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/google/uuid"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

const anthropicMaxTokens = 4096

// AnthropicProvider implements domain.LLMProvider using Anthropic's Messages
// API with forced tool use.
type AnthropicProvider struct {
	client anthropic.Client
	model  string
}

// NewAnthropicProvider creates an Anthropic-backed LLMProvider. extraOpts are
// forwarded to the SDK client after the API key, letting tests point it at a
// fake server via option.WithBaseURL.
func NewAnthropicProvider(apiKey, model string, extraOpts ...option.RequestOption) *AnthropicProvider {
	opts := append([]option.RequestOption{option.WithAPIKey(apiKey)}, extraOpts...)
	return &AnthropicProvider{
		client: anthropic.NewClient(opts...),
		model:  model,
	}
}

// GenerateStructured implements domain.LLMProvider.
func (p *AnthropicProvider) GenerateStructured(
	ctx context.Context, in domain.GenerationInput,
) (*domain.Variation, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid input: %w", err)
	}

	msg, err := p.client.Messages.New(ctx, p.newParams(in))
	if err != nil {
		return nil, mapAnthropicError(err)
	}

	out, err := decodeAnthropicToolUse(msg)
	if err != nil {
		return nil, err
	}

	variation := toDomainVariation(uuid.NewString(), in.Strategy, out)
	if err := variation.Validate(); err != nil {
		return nil, fmt.Errorf("llm: %w", err)
	}
	return &variation, nil
}

// StreamStructured implements domain.LLMProvider. It relays the model's
// incremental tool-input JSON as token deltas, then decodes and validates the
// fully assembled input once the stream ends.
func (p *AnthropicProvider) StreamStructured(
	ctx context.Context, in domain.GenerationInput,
) (<-chan domain.StreamChunk, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid input: %w", err)
	}

	stream := p.client.Messages.NewStreaming(ctx, p.newParams(in))
	out := make(chan domain.StreamChunk)

	go func() {
		defer close(out)

		var partialJSON string
		for stream.Next() {
			delta := inputJSONDelta(stream.Current())
			if delta == "" {
				continue
			}
			partialJSON += delta
			if !sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkToken, Delta: delta}) {
				return
			}
		}

		if err := stream.Err(); err != nil {
			sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkError, Err: mapAnthropicError(err)})
			return
		}

		variation, err := decodeStreamedVariation(in.Strategy, partialJSON)
		if err != nil {
			sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkError, Err: err})
			return
		}
		sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkVariationCompleted, Variation: variation})
	}()

	return out, nil
}

// inputJSONDelta extracts the partial tool-input JSON fragment from one
// streamed event, or "" if the event carries none.
//
// This ignores event.Index and simply concatenates every content_block_delta
// in arrival order, which is only correct because ToolChoice forces exactly
// one tool and therefore exactly one content block (index 0). If a future
// change ever allows more than one content block (e.g. extended thinking),
// this must filter by index or it will silently interleave unrelated deltas.
func inputJSONDelta(event anthropic.MessageStreamEventUnion) string {
	if event.Type != "content_block_delta" {
		return ""
	}
	return event.Delta.PartialJSON
}

func (p *AnthropicProvider) newParams(in domain.GenerationInput) anthropic.MessageNewParams {
	return anthropic.MessageNewParams{
		Model:     p.model,
		MaxTokens: anthropicMaxTokens,
		System:    []anthropic.TextBlockParam{{Text: systemPrompt(in)}},
		Messages: []anthropic.MessageParam{
			anthropic.NewUserMessage(anthropic.NewTextBlock(userPrompt(in))),
		},
		Tools: []anthropic.ToolUnionParam{
			{
				OfTool: &anthropic.ToolParam{
					Name:        toolName,
					Description: anthropic.String(toolDescription),
					InputSchema: anthropic.ToolInputSchemaParam{
						Properties: toolProperties(),
						Required:   toolRequired,
					},
				},
			},
		},
		ToolChoice: anthropic.ToolChoiceParamOfTool(toolName),
	}
}

// decodeAnthropicToolUse extracts and decodes the emit_pricing_variation tool
// call from a completed message. The model is instructed to always call it;
// if it did not, that is a structured-output failure, not a bug to recover
// from silently.
func decodeAnthropicToolUse(msg *anthropic.Message) (toolOutput, error) {
	for _, block := range msg.Content {
		if block.Type != "tool_use" || block.Name != toolName {
			continue
		}
		var out toolOutput
		if err := json.Unmarshal(block.Input, &out); err != nil {
			return toolOutput{}, fmt.Errorf("%w: %w", domain.ErrInvalidLLMResponse, err)
		}
		return out, nil
	}
	return toolOutput{}, fmt.Errorf("%w: model did not call %s", domain.ErrInvalidLLMResponse, toolName)
}

// mapAnthropicError classifies an error from the Anthropic SDK into the
// domain's sentinel errors so callers can branch on retryability without
// depending on the SDK's error type.
func mapAnthropicError(err error) error {
	// A context cancellation/timeout is a caller decision, not a signal about
	// provider health: it must never be conflated with ErrProviderUnavailable,
	// which Sprint 4 uses to decide whether to fail over to another provider.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var apiErr *anthropic.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%w: %w", domain.ErrProviderUnavailable, err)
	}
	switch {
	case apiErr.StatusCode == http.StatusUnauthorized, apiErr.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %w", domain.ErrProviderUnauthorized, err)
	case apiErr.StatusCode == http.StatusTooManyRequests, apiErr.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %w", domain.ErrProviderUnavailable, err)
	default:
		return fmt.Errorf("anthropic: %w", err)
	}
}
