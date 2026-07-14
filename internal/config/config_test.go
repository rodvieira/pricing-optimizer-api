package config

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoad(t *testing.T) {
	// No t.Parallel: these cases mutate process env via t.Setenv, which is
	// incompatible with parallel tests.
	tests := []struct {
		name    string
		env     map[string]string
		assert  func(t *testing.T, cfg Config)
		wantErr string
	}{
		{
			name: "defaults when env is unset",
			assert: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, "development", cfg.Env)
				assert.Equal(t, 8080, cfg.Port)
				assert.Equal(t, 15*time.Second, cfg.ReadTimeout)
				assert.Equal(t, 15*time.Second, cfg.WriteTimeout)
				assert.Equal(t, 10*time.Second, cfg.ShutdownTimeout)
				assert.Equal(t, "anthropic", cfg.LLMProvider)
				assert.Equal(t, "claude-sonnet-5", cfg.AnthropicModel)
				assert.Equal(t, "llama-3.3-70b-versatile", cfg.GroqModel)
				assert.Equal(t, "llama-3.1-8b-instant", cfg.GroqFallbackModel)
			},
		},
		{
			name: "env overrides are applied",
			env:  map[string]string{"PORT": "9090", "APP_ENV": "production"},
			assert: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, 9090, cfg.Port)
				assert.Equal(t, "production", cfg.Env)
			},
		},
		{
			name:    "port above range is rejected",
			env:     map[string]string{"PORT": "70000"},
			wantErr: "invalid PORT",
		},
		{
			name:    "zero port is rejected",
			env:     map[string]string{"PORT": "0"},
			wantErr: "invalid PORT",
		},
		{
			name:    "non-positive timeout is rejected",
			env:     map[string]string{"HTTP_READ_TIMEOUT": "0s"},
			wantErr: "invalid HTTP_READ_TIMEOUT",
		},
		{
			name:    "unparseable port is a parse error",
			env:     map[string]string{"PORT": "not-a-number"},
			wantErr: "parse config from env",
		},
		{
			name:    "unknown LLM provider is rejected",
			env:     map[string]string{"LLM_PROVIDER": "openai"},
			wantErr: "invalid LLM_PROVIDER",
		},
		{
			name: "groq provider is accepted",
			env:  map[string]string{"LLM_PROVIDER": "groq"},
			assert: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, "groq", cfg.LLMProvider)
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()

			if tt.wantErr != "" {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.wantErr)
				return
			}

			require.NoError(t, err)
			tt.assert(t, cfg)
		})
	}
}
