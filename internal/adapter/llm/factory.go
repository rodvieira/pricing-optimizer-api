package llm

import (
	"fmt"

	"github.com/rodvieira/pricing-optimizer-api/internal/domain"
)

// Config is the subset of configuration NewProvider needs to select and
// construct an LLMProvider. cmd/api maps the loaded internal/config.Config
// into this shape at wiring time, keeping this adapter decoupled from
// unrelated (HTTP, DB) configuration concerns.
type Config struct {
	Provider        string
	AnthropicAPIKey string
	AnthropicModel  string
	GroqAPIKey      string
	GroqModel       string
}

// NewProvider selects and constructs the LLMProvider configured by cfg. Per
// ADR-0003, cfg.Provider is "anthropic" in development and "groq" in
// production; no use case or handler is aware of which concrete provider
// this returns.
func NewProvider(cfg Config) (domain.LLMProvider, error) {
	switch cfg.Provider {
	case "anthropic":
		if cfg.AnthropicAPIKey == "" {
			return nil, fmt.Errorf("llm: ANTHROPIC_API_KEY is required when LLM_PROVIDER=anthropic")
		}
		return NewAnthropicProvider(cfg.AnthropicAPIKey, cfg.AnthropicModel), nil
	case "groq":
		if cfg.GroqAPIKey == "" {
			return nil, fmt.Errorf("llm: GROQ_API_KEY is required when LLM_PROVIDER=groq")
		}
		return NewGroqProvider(cfg.GroqAPIKey, cfg.GroqModel), nil
	default:
		return nil, fmt.Errorf("llm: unknown provider %q", cfg.Provider)
	}
}
