// Command api is the entry point for the Pricing Optimizer HTTP service.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/cache"
	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/httpapi"
	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/llm"
	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/repository"
	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/scraper"
	"github.com/rodvieira/pricing-optimizer-api/internal/buildinfo"
	"github.com/rodvieira/pricing-optimizer-api/internal/config"
	"github.com/rodvieira/pricing-optimizer-api/internal/telemetry"
	"github.com/rodvieira/pricing-optimizer-api/internal/usecase"
)

func main() {
	slog.SetDefault(slog.New(telemetry.NewSlogHandler(slog.NewJSONHandler(os.Stdout, nil))))

	if err := run(); err != nil {
		slog.Error("server terminated", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	shutdownTelemetry, err := telemetry.Init(context.Background(), telemetry.Config{
		ServiceName:    "pricing-optimizer-api",
		ServiceVersion: buildinfo.Version,
		Endpoint:       cfg.OTELExporterEndpoint,
	})
	if err != nil {
		return fmt.Errorf("init telemetry: %w", err)
	}
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		if err := shutdownTelemetry(shutdownCtx); err != nil {
			slog.Error("telemetry shutdown failed", "error", err)
		}
	}()

	llmProvider, err := llm.NewProvider(llm.Config{
		Provider:        cfg.LLMProvider,
		AnthropicAPIKey: cfg.AnthropicAPIKey,
		AnthropicModel:  cfg.AnthropicModel,
		GroqAPIKey:      cfg.GroqAPIKey,
		GroqModel:       cfg.GroqModel,
	})
	if err != nil {
		return fmt.Errorf("build llm provider: %w", err)
	}
	llmProvider = llm.NewTracingProvider(llmProvider, cfg.LLMProvider)

	siteScraper := scraper.NewFallbackScraper(
		scraper.NewCollyScraper(cfg.ScraperStaticTimeout),
		scraper.NewChromedpScraper(cfg.ChromeExecPath, cfg.ScraperBrowserTimeout),
	)

	poolCfg, err := pgxpool.ParseConfig(cfg.DatabaseURL)
	if err != nil {
		return fmt.Errorf("parse postgres pool config: %w", err)
	}
	poolCfg.ConnConfig.Tracer = repository.NewQueryTracer()
	pool, err := pgxpool.NewWithConfig(context.Background(), poolCfg)
	if err != nil {
		return fmt.Errorf("create postgres pool: %w", err)
	}
	defer pool.Close()
	generationRepo := repository.NewPostgresGenerationRepo(pool)

	analyzeSite := usecase.NewAnalyzeSite(siteScraper, llmProvider)
	generateVariations := usecase.NewGenerateVariations(llmProvider, generationRepo)
	exportVariation := usecase.NewExportVariation(generationRepo)

	redisOpts := &redis.Options{Addr: cfg.RedisAddr, Password: cfg.RedisPassword}
	if cfg.RedisTLSEnabled {
		// Upstash only accepts TLS connections; the local dev container
		// doesn't speak TLS at all, hence the env-gated toggle rather than
		// always setting this.
		redisOpts.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	redisClient := redis.NewClient(redisOpts)
	defer redisClient.Close()
	rateLimiter := cache.NewRedisRateLimiter(redisClient, cfg.RateLimitRequests, cfg.RateLimitWindow)
	idempotencyStore := cache.NewRedisIdempotencyStore(redisClient, cfg.IdempotencyTTL)
	analyzeCache := cache.NewRedisResponseCache(redisClient, cfg.AnalyzeCacheTTL)

	srv := &http.Server{
		Addr: ":" + strconv.Itoa(cfg.Port),
		Handler: httpapi.NewRouter(httpapi.NewServer(
			analyzeSite, generateVariations, generationRepo, exportVariation,
			rateLimiter, idempotencyStore, analyzeCache,
		), cfg.AllowedOrigins),
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", srv.Addr, "env", cfg.Env,
			"version", buildinfo.Version, "commit", buildinfo.Commit, "build_time", buildinfo.BuildTime)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			serveErr <- err
		}
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("listen and serve: %w", err)
	case <-ctx.Done():
		slog.Info("shutdown signal received")
	}

	shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}

	slog.Info("server stopped cleanly")
	return nil
}
