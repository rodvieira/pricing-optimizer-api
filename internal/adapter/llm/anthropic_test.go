package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/anthropics/anthropic-sdk-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func newTestAnthropicProvider(baseURL string) *AnthropicProvider {
	return NewAnthropicProvider("test-key", "claude-sonnet-5", option.WithBaseURL(baseURL))
}

func TestAnthropicProvider_GenerateStructured(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		fixture        string
		statusCode     int
		wantErr        error
		wantGenericErr bool // an error occurs but matches neither domain sentinel
		assertOK       func(t *testing.T, v *domain.Variation)
	}{
		{
			name:       "decodes a valid tool call into a Variation",
			fixture:    "anthropic_message_success.json",
			statusCode: http.StatusOK,
			assertOK: func(t *testing.T, v *domain.Variation) {
				t.Helper()
				assert.Equal(t, domain.StrategyValueBased, v.Strategy)
				assert.Equal(t, "Simple, transparent pricing", v.Headline)
				assert.Len(t, v.Tiers, 2)
				assert.Equal(t, "Pro", v.Tiers[1].Name)
				assert.True(t, v.Tiers[1].Highlighted)
				assert.NotEmpty(t, v.ID)
			},
		},
		{
			name:       "model not calling the tool is an invalid response",
			fixture:    "anthropic_message_no_tool_use.json",
			statusCode: http.StatusOK,
			wantErr:    domain.ErrInvalidLLMResponse,
		},
		{
			name:       "an empty decoded variation fails domain validation",
			fixture:    "anthropic_message_invalid_variation.json",
			statusCode: http.StatusOK,
			wantErr:    domain.ErrInvalidLLMResponse,
		},
		{
			name:       "401 maps to ErrProviderUnauthorized",
			fixture:    "anthropic_error_401.json",
			statusCode: http.StatusUnauthorized,
			wantErr:    domain.ErrProviderUnauthorized,
		},
		{
			name:       "429 maps to ErrProviderUnavailable",
			fixture:    "anthropic_error_429.json",
			statusCode: http.StatusTooManyRequests,
			wantErr:    domain.ErrProviderUnavailable,
		},
		{
			name:           "400 is wrapped but matches neither sentinel",
			fixture:        "anthropic_error_401.json",
			statusCode:     http.StatusBadRequest,
			wantGenericErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := jsonServer(t, tt.statusCode, readFixture(t, tt.fixture))
			provider := newTestAnthropicProvider(srv.URL)

			got, err := provider.GenerateStructured(context.Background(), testGenerationInput())

			switch {
			case tt.wantErr != nil:
				require.Error(t, err)
				assert.ErrorIs(t, err, tt.wantErr)
			case tt.wantGenericErr:
				require.Error(t, err)
				assert.NotErrorIs(t, err, domain.ErrProviderUnauthorized)
				assert.NotErrorIs(t, err, domain.ErrProviderUnavailable)
			default:
				require.NoError(t, err)
				tt.assertOK(t, got)
			}
		})
	}
}

func TestAnthropicProvider_GenerateStructured_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	provider := newTestAnthropicProvider("http://unused.invalid")
	in := testGenerationInput()
	in.Strategy = "bogus"

	_, err := provider.GenerateStructured(context.Background(), in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestAnthropicProvider_StreamStructured(t *testing.T) {
	t.Parallel()

	srv := sseServer(t, readFixture(t, "anthropic_stream_success.sse"))
	provider := newTestAnthropicProvider(srv.URL)

	chunks, err := provider.StreamStructured(context.Background(), testGenerationInput())
	require.NoError(t, err)

	var tokens int
	var final *domain.Variation
	for chunk := range chunks {
		switch chunk.Type {
		case domain.StreamChunkToken:
			tokens++
		case domain.StreamChunkVariationCompleted:
			final = chunk.Variation
		case domain.StreamChunkError:
			t.Fatalf("unexpected error chunk: %v", chunk.Err)
		}
	}

	assert.Positive(t, tokens)
	require.NotNil(t, final)
	assert.Equal(t, "Simple, transparent pricing", final.Headline)
	assert.Len(t, final.Tiers, 1)
	assert.Equal(t, "Pro", final.Tiers[0].Name)
}

func TestAnthropicProvider_StreamStructured_MalformedJSON(t *testing.T) {
	t.Parallel()

	sse := "event: content_block_delta\n" +
		"data: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"not valid json\"}}\n\n" +
		"event: message_stop\n" +
		"data: {\"type\":\"message_stop\"}\n\n"

	srv := sseServer(t, []byte(sse))
	provider := newTestAnthropicProvider(srv.URL)

	chunks, err := provider.StreamStructured(context.Background(), testGenerationInput())
	require.NoError(t, err)

	var errChunk *domain.StreamChunk
	for chunk := range chunks {
		if chunk.Type == domain.StreamChunkError {
			c := chunk
			errChunk = &c
		}
	}

	require.NotNil(t, errChunk, "expected a terminal error chunk")
	assert.ErrorIs(t, errChunk.Err, domain.ErrInvalidLLMResponse)
}

func TestAnthropicProvider_GenerateStructured_TransportError(t *testing.T) {
	t.Parallel()

	// Port 0 on the host portion is never listening; the SDK never receives an
	// HTTP response, so the error is a transport failure, not an *anthropic.Error.
	provider := newTestAnthropicProvider("http://127.0.0.1:0")

	_, err := provider.GenerateStructured(context.Background(), testGenerationInput())

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrProviderUnavailable)
	assert.NotErrorIs(t, err, domain.ErrProviderUnauthorized)
}

func TestAnthropicProvider_StreamStructured_ClosesOnContextCancellation(t *testing.T) {
	t.Parallel()

	srv := sseServer(t, readFixture(t, "anthropic_stream_success.sse"))
	provider := newTestAnthropicProvider(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	chunks, err := provider.StreamStructured(ctx, testGenerationInput())
	require.NoError(t, err)

	// Simulate a consumer that walks away without draining: cancel
	// immediately, then keep attempting to receive. If the producer
	// goroutine ever blocks on a send instead of honoring ctx.Done(), this
	// receive loop blocks forever too and the deadline below catches it.
	cancel()

	deadline := time.After(2 * time.Second)
	for {
		select {
		case _, ok := <-chunks:
			if !ok {
				return // channel closed promptly: producer respected cancellation
			}
		case <-deadline:
			t.Fatal("channel did not close after context cancellation; producer goroutine leaked")
		}
	}
}

func TestAnthropicProvider_GenerateStructured_RequestConstruction(t *testing.T) {
	t.Parallel()

	srv, capturedBody := capturingServer(t, http.StatusOK, readFixture(t, "anthropic_message_success.json"))
	provider := newTestAnthropicProvider(srv.URL)

	_, err := provider.GenerateStructured(context.Background(), testGenerationInput())
	require.NoError(t, err)

	var req map[string]any
	require.NoError(t, json.Unmarshal(capturedBody(), &req))

	assert.Equal(t, "claude-sonnet-5", req["model"])

	tools, ok := req["tools"].([]any)
	require.True(t, ok, "request must include tools")
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, toolName, tool["name"])

	toolChoice, ok := req["tool_choice"].(map[string]any)
	require.True(t, ok, "request must force tool_choice")
	assert.Equal(t, "tool", toolChoice["type"])
	assert.Equal(t, toolName, toolChoice["name"])
}
