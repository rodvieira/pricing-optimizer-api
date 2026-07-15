// Package config loads runtime configuration from the environment (12-factor).
package config

import (
	"fmt"
	"time"

	"github.com/caarlos0/env/v11"
)

// Config holds the service configuration, populated from environment variables.
type Config struct {
	Env             string        `env:"APP_ENV" envDefault:"development"`
	Port            int           `env:"PORT" envDefault:"8080"`
	ReadTimeout     time.Duration `env:"HTTP_READ_TIMEOUT" envDefault:"15s"`
	WriteTimeout    time.Duration `env:"HTTP_WRITE_TIMEOUT" envDefault:"15s"`
	ShutdownTimeout time.Duration `env:"HTTP_SHUTDOWN_TIMEOUT" envDefault:"10s"`

	// LLMProvider selects the LLMProvider implementation: "anthropic" or "groq".
	// Per ADR-0003: Anthropic in development, Groq in production.
	LLMProvider string `env:"LLM_PROVIDER" envDefault:"anthropic"`

	AnthropicAPIKey string `env:"ANTHROPIC_API_KEY"`
	AnthropicModel  string `env:"ANTHROPIC_MODEL" envDefault:"claude-sonnet-5"`

	GroqAPIKey        string `env:"GROQ_API_KEY"`
	GroqModel         string `env:"GROQ_MODEL" envDefault:"llama-3.3-70b-versatile"`
	GroqFallbackModel string `env:"GROQ_FALLBACK_MODEL" envDefault:"llama-3.1-8b-instant"`

	// ScraperStaticTimeout bounds the fast colly fetch FallbackScraper always
	// tries first. ScraperBrowserTimeout bounds the chromedp fallback, which
	// launches a real browser and needs materially longer. ChromeExecPath
	// overrides chromedp's own well-known-path search; empty lets it discover
	// the binary itself.
	ScraperStaticTimeout  time.Duration `env:"SCRAPER_STATIC_TIMEOUT" envDefault:"10s"`
	ScraperBrowserTimeout time.Duration `env:"SCRAPER_BROWSER_TIMEOUT" envDefault:"20s"`
	ChromeExecPath        string        `env:"CHROME_EXEC_PATH" envDefault:""`

	// RedisAddr/RedisPassword point at the rate limiter's backing Redis:
	// Upstash in production (per HANDOFF.md's $0/month constraint), a local
	// container in development. RedisPassword is empty for a local Redis
	// with no AUTH configured.
	RedisAddr     string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPassword string `env:"REDIS_PASSWORD" envDefault:""`

	// RateLimitRequests is how many calls a single client (identified by IP)
	// may make to a rate-limited endpoint (/v1/analyze, /v1/generate — the
	// two that spend LLM/scraper budget) per RateLimitWindow, fixed-window.
	RateLimitRequests int           `env:"RATE_LIMIT_REQUESTS" envDefault:"10"`
	RateLimitWindow   time.Duration `env:"RATE_LIMIT_WINDOW" envDefault:"1m"`
}

// Load reads and validates the configuration from the environment.
func Load() (Config, error) {
	var cfg Config
	if err := env.Parse(&cfg); err != nil {
		return Config{}, fmt.Errorf("parse config from env: %w", err)
	}
	if err := cfg.validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validate enforces invariants the type system cannot express.
func (c Config) validate() error {
	if c.Port < 1 || c.Port > 65535 {
		return fmt.Errorf("invalid PORT %d: must be between 1 and 65535", c.Port)
	}
	if c.ReadTimeout <= 0 {
		return fmt.Errorf("invalid HTTP_READ_TIMEOUT %s: must be positive", c.ReadTimeout)
	}
	if c.WriteTimeout <= 0 {
		return fmt.Errorf("invalid HTTP_WRITE_TIMEOUT %s: must be positive", c.WriteTimeout)
	}
	if c.ShutdownTimeout <= 0 {
		return fmt.Errorf("invalid HTTP_SHUTDOWN_TIMEOUT %s: must be positive", c.ShutdownTimeout)
	}
	if c.LLMProvider != "anthropic" && c.LLMProvider != "groq" {
		return fmt.Errorf("invalid LLM_PROVIDER %q: must be \"anthropic\" or \"groq\"", c.LLMProvider)
	}
	if c.ScraperStaticTimeout <= 0 {
		return fmt.Errorf("invalid SCRAPER_STATIC_TIMEOUT %s: must be positive", c.ScraperStaticTimeout)
	}
	if c.ScraperBrowserTimeout <= 0 {
		return fmt.Errorf("invalid SCRAPER_BROWSER_TIMEOUT %s: must be positive", c.ScraperBrowserTimeout)
	}
	if c.RateLimitRequests <= 0 {
		return fmt.Errorf("invalid RATE_LIMIT_REQUESTS %d: must be positive", c.RateLimitRequests)
	}
	if c.RateLimitWindow <= 0 {
		return fmt.Errorf("invalid RATE_LIMIT_WINDOW %s: must be positive", c.RateLimitWindow)
	}
	return nil
}
