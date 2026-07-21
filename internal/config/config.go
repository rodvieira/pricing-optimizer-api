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
	// with no AUTH configured. RedisTLSEnabled must be true for Upstash,
	// which only accepts TLS connections (rediss://, not redis://) — false
	// for the local dev container, which doesn't speak TLS at all.
	RedisAddr       string `env:"REDIS_ADDR" envDefault:"localhost:6379"`
	RedisPassword   string `env:"REDIS_PASSWORD" envDefault:""`
	RedisTLSEnabled bool   `env:"REDIS_TLS_ENABLED" envDefault:"false"`

	// RateLimitRequests is how many calls a single client (identified by IP)
	// may make to a rate-limited endpoint (/v1/analyze, /v1/generate — the
	// two that spend LLM/scraper budget) per RateLimitWindow, fixed-window.
	RateLimitRequests int           `env:"RATE_LIMIT_REQUESTS" envDefault:"10"`
	RateLimitWindow   time.Duration `env:"RATE_LIMIT_WINDOW" envDefault:"1m"`

	// IdempotencyTTL is how long an Idempotency-Key on POST /v1/generate
	// stays mapped to its generation: a repeat request with the same key
	// within this window replays the existing generation instead of
	// starting a new (LLM-cost-spending) one. 24h covers a client's own
	// retry window comfortably without keeping keys around indefinitely.
	IdempotencyTTL time.Duration `env:"IDEMPOTENCY_TTL" envDefault:"24h"`

	// AnalyzeCacheTTL is how long a POST /v1/analyze response is cached,
	// keyed on the submitted URL: a repeat analysis of the same URL within
	// this window skips scraping and the LLM classification call entirely.
	// Shorter than IdempotencyTTL on purpose — this is a freshness/cost
	// tradeoff for a page whose content can genuinely change, not a
	// retry-safety window for one specific in-flight request.
	AnalyzeCacheTTL time.Duration `env:"ANALYZE_CACHE_TTL" envDefault:"1h"`

	// AllowedOrigins is the frontend origin(s) allowed to call this API
	// cross-origin (comma-separated). The frontend is always a separate
	// origin from this API — a separate Next.js repo/deploy (local dev on a
	// different port, Vercel calling Cloud Run in production) — so CORS is
	// required in every environment.
	AllowedOrigins []string `env:"ALLOWED_ORIGINS" envSeparator:"," envDefault:"http://localhost:3000"`

	// DatabaseURL is the pgx connection string PostgresGenerationRepo
	// connects with: Neon in production (per HANDOFF.md's $0/month
	// constraint), a local container in development.
	DatabaseURL string `env:"DATABASE_URL" envDefault:"postgres://postgres:postgres@localhost:5432/pricing?sslmode=disable"`

	// OTELExporterEndpoint gates OpenTelemetry export: empty (the default,
	// no collector/Grafana Cloud provisioned yet — that's Sprint 7) means
	// tracing is a no-op. When set, telemetry.Init reads the rest of the
	// standard OTEL_EXPORTER_OTLP_* env vars itself; this field only
	// decides on/off. See ADR-0007.
	OTELExporterEndpoint string `env:"OTEL_EXPORTER_OTLP_ENDPOINT" envDefault:""`
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
	if c.IdempotencyTTL <= 0 {
		return fmt.Errorf("invalid IDEMPOTENCY_TTL %s: must be positive", c.IdempotencyTTL)
	}
	if c.AnalyzeCacheTTL <= 0 {
		return fmt.Errorf("invalid ANALYZE_CACHE_TTL %s: must be positive", c.AnalyzeCacheTTL)
	}
	if len(c.AllowedOrigins) == 0 {
		return fmt.Errorf("invalid ALLOWED_ORIGINS: must not be empty")
	}
	return nil
}
