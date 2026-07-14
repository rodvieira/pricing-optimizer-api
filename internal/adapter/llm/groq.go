package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"github.com/openai/openai-go"
	"github.com/openai/openai-go/option"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// groqBaseURL is Groq's OpenAI-compatible endpoint.
const groqBaseURL = "https://api.groq.com/openai/v1"

// GroqProvider implements domain.LLMProvider using Groq's OpenAI-compatible
// chat completions API with forced tool use.
type GroqProvider struct {
	client openai.Client
	model  string
}

// NewGroqProvider creates a Groq-backed LLMProvider. extraOpts are forwarded
// to the SDK client after the API key and base URL, letting tests point it at
// a fake server via option.WithBaseURL (overriding groqBaseURL).
func NewGroqProvider(apiKey, model string, extraOpts ...option.RequestOption) *GroqProvider {
	opts := append([]option.RequestOption{
		option.WithAPIKey(apiKey),
		option.WithBaseURL(groqBaseURL),
	}, extraOpts...)
	return &GroqProvider{
		client: openai.NewClient(opts...),
		model:  model,
	}
}

// GenerateStructured implements domain.LLMProvider.
func (p *GroqProvider) GenerateStructured(
	ctx context.Context, in domain.GenerationInput,
) (*domain.Variation, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid input: %w", err)
	}

	resp, err := p.client.Chat.Completions.New(ctx, p.newParams(in))
	if err != nil {
		return nil, mapGroqError(err)
	}

	out, err := decodeGroqToolCall(resp)
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
// incremental tool-call argument JSON as token deltas, then decodes and
// validates the fully assembled arguments once the stream ends.
func (p *GroqProvider) StreamStructured(
	ctx context.Context, in domain.GenerationInput,
) (<-chan domain.StreamChunk, error) {
	if err := in.Validate(); err != nil {
		return nil, fmt.Errorf("llm: invalid input: %w", err)
	}

	stream := p.client.Chat.Completions.NewStreaming(ctx, p.newParams(in))
	out := make(chan domain.StreamChunk)

	go func() {
		defer close(out)

		var arguments string
		for stream.Next() {
			for _, delta := range toolCallDeltas(stream.Current()) {
				arguments += delta
				if !sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkToken, Delta: delta}) {
					return
				}
			}
		}

		if err := stream.Err(); err != nil {
			sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkError, Err: mapGroqError(err)})
			return
		}

		variation, err := decodeStreamedVariation(in.Strategy, arguments)
		if err != nil {
			sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkError, Err: err})
			return
		}
		sendChunk(ctx, out, domain.StreamChunk{Type: domain.StreamChunkVariationCompleted, Variation: variation})
	}()

	return out, nil
}

// toolCallDeltas extracts non-empty tool-call argument fragments from one
// streamed chunk.
//
// This ignores each call's Index and simply concatenates every fragment in
// arrival order, which is only correct because ToolChoice forces exactly one
// named function tool call. If a future change ever allows the model to call
// more than one tool concurrently, this must group fragments by Index or it
// will silently interleave unrelated arguments.
func toolCallDeltas(chunk openai.ChatCompletionChunk) []string {
	if len(chunk.Choices) == 0 {
		return nil
	}
	deltas := make([]string, 0, len(chunk.Choices[0].Delta.ToolCalls))
	for _, call := range chunk.Choices[0].Delta.ToolCalls {
		if call.Function.Arguments != "" {
			deltas = append(deltas, call.Function.Arguments)
		}
	}
	return deltas
}

func (p *GroqProvider) newParams(in domain.GenerationInput) openai.ChatCompletionNewParams {
	return openai.ChatCompletionNewParams{
		Model: p.model,
		Messages: []openai.ChatCompletionMessageParamUnion{
			openai.SystemMessage(systemPrompt(in)),
			openai.UserMessage(userPrompt(in)),
		},
		Tools: []openai.ChatCompletionToolParam{
			{
				Function: openai.FunctionDefinitionParam{
					Name:        toolName,
					Description: openai.String(toolDescription),
					Parameters:  toolJSONSchema(),
				},
			},
		},
		ToolChoice: openai.ChatCompletionToolChoiceOptionParamOfChatCompletionNamedToolChoice(
			openai.ChatCompletionNamedToolChoiceFunctionParam{Name: toolName},
		),
	}
}

// decodeGroqToolCall extracts and decodes the emit_pricing_variation tool
// call from a completed response. The model is instructed to always call it;
// if it did not, that is a structured-output failure, not a bug to recover
// from silently.
func decodeGroqToolCall(resp *openai.ChatCompletion) (toolOutput, error) {
	if len(resp.Choices) == 0 {
		return toolOutput{}, fmt.Errorf("%w: no choices in response", domain.ErrInvalidLLMResponse)
	}
	for _, call := range resp.Choices[0].Message.ToolCalls {
		if call.Function.Name != toolName {
			continue
		}
		var out toolOutput
		if err := json.Unmarshal([]byte(call.Function.Arguments), &out); err != nil {
			return toolOutput{}, fmt.Errorf("%w: %w", domain.ErrInvalidLLMResponse, err)
		}
		return out, nil
	}
	return toolOutput{}, fmt.Errorf("%w: model did not call %s", domain.ErrInvalidLLMResponse, toolName)
}

// mapGroqError classifies an error from the OpenAI-compatible SDK into the
// domain's sentinel errors so callers can branch on retryability without
// depending on the SDK's error type.
func mapGroqError(err error) error {
	// A context cancellation/timeout is a caller decision, not a signal about
	// provider health: it must never be conflated with ErrProviderUnavailable,
	// which Sprint 4 uses to decide whether to fail over to another provider.
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}

	var apiErr *openai.Error
	if !errors.As(err, &apiErr) {
		return fmt.Errorf("%w: %w", domain.ErrProviderUnavailable, err)
	}
	switch {
	case apiErr.StatusCode == http.StatusUnauthorized, apiErr.StatusCode == http.StatusForbidden:
		return fmt.Errorf("%w: %w", domain.ErrProviderUnauthorized, err)
	case apiErr.StatusCode == http.StatusTooManyRequests, apiErr.StatusCode >= http.StatusInternalServerError:
		return fmt.Errorf("%w: %w", domain.ErrProviderUnavailable, err)
	default:
		return fmt.Errorf("groq: %w", err)
	}
}
