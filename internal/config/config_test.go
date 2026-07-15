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
				assert.Equal(t, 10*time.Second, cfg.ScraperStaticTimeout)
				assert.Equal(t, 20*time.Second, cfg.ScraperBrowserTimeout)
				assert.Empty(t, cfg.ChromeExecPath)
				assert.Equal(t, "localhost:6379", cfg.RedisAddr)
				assert.Empty(t, cfg.RedisPassword)
				assert.Equal(t, 10, cfg.RateLimitRequests)
				assert.Equal(t, time.Minute, cfg.RateLimitWindow)
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
		{
			name:    "non-positive scraper static timeout is rejected",
			env:     map[string]string{"SCRAPER_STATIC_TIMEOUT": "0s"},
			wantErr: "invalid SCRAPER_STATIC_TIMEOUT",
		},
		{
			name:    "non-positive scraper browser timeout is rejected",
			env:     map[string]string{"SCRAPER_BROWSER_TIMEOUT": "-1s"},
			wantErr: "invalid SCRAPER_BROWSER_TIMEOUT",
		},
		{
			name: "chrome exec path override is applied",
			env:  map[string]string{"CHROME_EXEC_PATH": "/usr/bin/google-chrome-stable"},
			assert: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, "/usr/bin/google-chrome-stable", cfg.ChromeExecPath)
			},
		},
		{
			name:    "non-positive rate limit requests is rejected",
			env:     map[string]string{"RATE_LIMIT_REQUESTS": "0"},
			wantErr: "invalid RATE_LIMIT_REQUESTS",
		},
		{
			name:    "non-positive rate limit window is rejected",
			env:     map[string]string{"RATE_LIMIT_WINDOW": "-1s"},
			wantErr: "invalid RATE_LIMIT_WINDOW",
		},
		{
			name: "redis and rate limit overrides are applied",
			env: map[string]string{
				"REDIS_ADDR": "redis.internal:6380", "REDIS_PASSWORD": "secret",
				"RATE_LIMIT_REQUESTS": "5", "RATE_LIMIT_WINDOW": "30s",
			},
			assert: func(t *testing.T, cfg Config) {
				t.Helper()
				assert.Equal(t, "redis.internal:6380", cfg.RedisAddr)
				assert.Equal(t, "secret", cfg.RedisPassword)
				assert.Equal(t, 5, cfg.RateLimitRequests)
				assert.Equal(t, 30*time.Second, cfg.RateLimitWindow)
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
