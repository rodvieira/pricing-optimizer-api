package llm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewProvider(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		cfg     Config
		want    any // expected concrete type, via assert.IsType
		wantErr string
	}{
		{
			name: "anthropic with a key returns an AnthropicProvider",
			cfg: Config{
				Provider:        "anthropic",
				AnthropicAPIKey: "sk-ant-test",
				AnthropicModel:  "claude-sonnet-5",
			},
			want: &AnthropicProvider{},
		},
		{
			name:    "anthropic without a key is rejected",
			cfg:     Config{Provider: "anthropic"},
			wantErr: "ANTHROPIC_API_KEY is required",
		},
		{
			name: "groq with a key returns a GroqProvider",
			cfg: Config{
				Provider:   "groq",
				GroqAPIKey: "gsk-test",
				GroqModel:  "llama-3.3-70b-versatile",
			},
			want: &GroqProvider{},
		},
		{
			name:    "groq without a key is rejected",
			cfg:     Config{Provider: "groq"},
			wantErr: "GROQ_API_KEY is required",
		},
		{
			name:    "unknown provider is rejected",
			cfg:     Config{Provider: "openai"},
			wantErr: `unknown provider "openai"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			got, err := NewProvider(tt.cfg)

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				assert.Nil(t, got)
				return
			}
			require.NoError(t, err)
			assert.IsType(t, tt.want, got)
		})
	}
}
