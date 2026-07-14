package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
	"time"

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

func TestGroqProvider_StreamStructured_ClosesOnContextCancellation(t *testing.T) {
	t.Parallel()

	srv := sseServer(t, readFixture(t, "groq_stream_success.sse"))
	provider := newTestGroqProvider(srv.URL)

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

func TestGroqProvider_ClassifySite(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		fixture        string
		statusCode     int
		wantErr        error
		wantGenericErr bool
		assertOK       func(t *testing.T, p *domain.SiteProfile)
	}{
		{
			name:       "decodes a valid tool call into a SiteProfile",
			fixture:    "groq_site_profile_success.json",
			statusCode: http.StatusOK,
			assertOK: func(t *testing.T, p *domain.SiteProfile) {
				t.Helper()
				assert.Equal(t, "https://example.com", p.URL)
				assert.Equal(t, "Acme Analytics", p.Title, "title comes from the scrape, not the model")
				assert.Equal(t, "developer-tools", p.Industry)
				assert.Equal(t, domain.SophisticationMedium, p.Audience.Sophistication)
				assert.Equal(t, domain.SourceTypeStatic, p.SourceType, "source type comes from the scrape, not the model")
				assert.False(t, p.AnalyzedAt.IsZero())
			},
		},
		{
			name:       "model not calling the tool is an invalid response",
			fixture:    "groq_completion_no_tool_call.json",
			statusCode: http.StatusOK,
			wantErr:    domain.ErrInvalidLLMResponse,
		},
		{
			name:       "an empty decoded profile fails domain validation",
			fixture:    "groq_site_profile_invalid.json",
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

			got, err := provider.ClassifySite(context.Background(), testScrapedPage())

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

func TestGroqProvider_ClassifySite_RejectsInvalidInput(t *testing.T) {
	t.Parallel()

	provider := newTestGroqProvider("http://unused.invalid")
	page := testScrapedPage()
	page.Text = "  "

	_, err := provider.ClassifySite(context.Background(), page)

	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid input")
}

func TestGroqProvider_ClassifySite_RequestConstruction(t *testing.T) {
	t.Parallel()

	srv, capturedBody := capturingServer(t, http.StatusOK, readFixture(t, "groq_site_profile_success.json"))
	provider := newTestGroqProvider(srv.URL)

	_, err := provider.ClassifySite(context.Background(), testScrapedPage())
	require.NoError(t, err)

	var req map[string]any
	require.NoError(t, json.Unmarshal(capturedBody(), &req))

	tools, ok := req["tools"].([]any)
	require.True(t, ok, "request must include tools")
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	function, ok := tool["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, classifySiteToolName, function["name"])

	toolChoice, ok := req["tool_choice"].(map[string]any)
	require.True(t, ok, "request must force tool_choice")
	choiceFn, ok := toolChoice["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, classifySiteToolName, choiceFn["name"])
}

func TestGroqProvider_GenerateStructured_RequestConstruction(t *testing.T) {
	t.Parallel()

	srv, capturedBody := capturingServer(t, http.StatusOK, readFixture(t, "groq_completion_success.json"))
	provider := newTestGroqProvider(srv.URL)

	_, err := provider.GenerateStructured(context.Background(), testGenerationInput())
	require.NoError(t, err)

	var req map[string]any
	require.NoError(t, json.Unmarshal(capturedBody(), &req))

	assert.Equal(t, "llama-3.3-70b-versatile", req["model"])

	tools, ok := req["tools"].([]any)
	require.True(t, ok, "request must include tools")
	require.Len(t, tools, 1)
	tool, ok := tools[0].(map[string]any)
	require.True(t, ok)
	function, ok := tool["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, toolName, function["name"])

	toolChoice, ok := req["tool_choice"].(map[string]any)
	require.True(t, ok, "request must force tool_choice")
	assert.Equal(t, "function", toolChoice["type"])
	choiceFn, ok := toolChoice["function"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, toolName, choiceFn["name"])
}
