// Command api is the entry point for the Pricing Optimizer HTTP service.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/httpapi"
	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/llm"
	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/repository"
	"github.com/rodvieira/pricing-optimizer-api/internal/adapter/scraper"
	"github.com/rodvieira/pricing-optimizer-api/internal/config"
	"github.com/rodvieira/pricing-optimizer-api/internal/usecase"
)

func main() {
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stdout, nil)))

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

	siteScraper := scraper.NewFallbackScraper(
		scraper.NewCollyScraper(cfg.ScraperStaticTimeout),
		scraper.NewChromedpScraper(cfg.ChromeExecPath, cfg.ScraperBrowserTimeout),
	)

	analyzeSite := usecase.NewAnalyzeSite(siteScraper, llmProvider)
	generationRepo := repository.NewInMemoryGenerationRepo()
	generateVariations := usecase.NewGenerateVariations(llmProvider, generationRepo)
	exportVariation := usecase.NewExportVariation(generationRepo)

	srv := &http.Server{
		Addr:              ":" + strconv.Itoa(cfg.Port),
		Handler:           httpapi.NewRouter(httpapi.NewServer(analyzeSite, generateVariations, generationRepo, exportVariation)),
		ReadTimeout:       cfg.ReadTimeout,
		ReadHeaderTimeout: cfg.ReadTimeout,
		WriteTimeout:      cfg.WriteTimeout,
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	serveErr := make(chan error, 1)
	go func() {
		slog.Info("http server listening", "addr", srv.Addr, "env", cfg.Env)
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
