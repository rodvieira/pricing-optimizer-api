package llm

import (
	"context"
	"net/http"
	"testing"

	"github.com/openai/openai-go/option"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

func newTestGroqProvider(baseURL string) *GroqProvider {
	return NewGroqProvider("test-key", "llama-3.3-70b-versatile", option.WithBaseURL(baseURL))
}

func TestGroqProvider_GenerateStructured(t *testing.T) {
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
			fixture:    "groq_completion_success.json",
			statusCode: http.StatusOK,
			assertOK: func(t *testing.T, v *domain.Variation) {
				t.Helper()
				assert.Equal(t, domain.StrategyValueBased, v.Strategy)
				assert.Equal(t, "Pick the plan that fits", v.Headline)
				assert.Len(t, v.Tiers, 2)
				assert.Equal(t, "Team", v.Tiers[1].Name)
				assert.True(t, v.Tiers[1].Highlighted)
				assert.NotEmpty(t, v.ID)
			},
		},
		{
			name:       "model not calling the tool is an invalid response",
			fixture:    "groq_completion_no_tool_call.json",
			statusCode: http.StatusOK,
			wantErr:    domain.ErrInvalidLLMResponse,
		},
		{
			name:       "401 maps to ErrProviderUnauthorized",
			fixture:    "groq_error_401.json",
			statusCode: http.StatusUnauthorized,
			wantErr:    domain.ErrProviderUnauthorized,
		},
		{
			name:       "429 maps to ErrProviderUnavailable",
			fixture:    "groq_error_429.json",
			statusCode: http.StatusTooManyRequests,
			wantErr:    domain.ErrProviderUnavailable,
		},
		{
			name:           "400 is wrapped but matches neither sentinel",
			fixture:        "groq_error_401.json",
			statusCode:     http.StatusBadRequest,
			wantGenericErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			srv := jsonServer(t, tt.statusCode, readFixture(t, tt.fixture))
			provider := newTestGroqProvider(srv.URL)

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

func TestGroqProvider_GenerateStructured_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	provider := newTestGroqProvider("http://unused.invalid")
	in := testGenerationInput()
	in.Currency = "usd"

	_, err := provider.GenerateStructured(context.Background(), in)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestGroqProvider_StreamStructured(t *testing.T) {
	t.Parallel()

	srv := sseServer(t, readFixture(t, "groq_stream_success.sse"))
	provider := newTestGroqProvider(srv.URL)

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
	assert.Equal(t, "Pick the plan that fits", final.Headline)
	assert.Len(t, final.Tiers, 1)
	assert.Equal(t, "Team", final.Tiers[0].Name)
}

func TestGroqProvider_StreamStructured_MalformedJSON(t *testing.T) {
	t.Parallel()

	sse := `data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"m",` +
		`"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"not valid json"}}]},"finish_reason":null}]}` + "\n\n" +
		`data: {"id":"c1","object":"chat.completion.chunk","created":1,"model":"m",` +
		`"choices":[{"index":0,"delta":{},"finish_reason":"tool_calls"}]}` + "\n\n" +
		"data: [DONE]\n\n"

	srv := sseServer(t, []byte(sse))
	provider := newTestGroqProvider(srv.URL)

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

func TestGroqProvider_GenerateStructured_TransportError(t *testing.T) {
	t.Parallel()

	// Port 0 on the host portion is never listening; the SDK never receives an
	// HTTP response, so the error is a transport failure, not an *openai.Error.
	provider := newTestGroqProvider("http://127.0.0.1:0")

	_, err := provider.GenerateStructured(context.Background(), testGenerationInput())

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrProviderUnavailable)
	assert.NotErrorIs(t, err, domain.ErrProviderUnauthorized)
}
