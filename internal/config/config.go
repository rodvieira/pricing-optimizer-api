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
	return nil
}
