package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"codeberg.org/eddiectc/portfoliolab/internal/api"
	"codeberg.org/eddiectc/portfoliolab/internal/config"
	"codeberg.org/eddiectc/portfoliolab/internal/data"
)

func main() {
	// Load configuration
	cfgPath := "config/config.yaml"
	if len(os.Args) > 1 {
		for i := 0; i < len(os.Args)-1; i++ {
			if os.Args[i] == "--config" {
				cfgPath = os.Args[i+1]
				break
			}
		}
	}

	cfg, err := config.Load(cfgPath)
	if err != nil {
		// Use default config if file doesn't exist (allow running without config)
		if os.IsNotExist(err) {
			slog.Warn("config file not found, using defaults", "path", cfgPath)
			cfg = config.Defaults()
		} else {
			slog.Error("failed to load config", "error", err)
			os.Exit(1)
		}
	}

	// Setup logger
	logger := setupLogger(cfg.LogLevel())
	logger.Info("starting server", "addr", cfg.ServerAddr())

	// Open database
	db, err := data.Open(cfg.Database.Path, logger)
	if err != nil {
		logger.Error("failed to open database", "error", err)
		os.Exit(1)
	}
	defer data.Close(db)

	// Run migrations
	if err := data.MigrateUp(db, "migrations", logger); err != nil {
		logger.Error("failed to run migrations", "error", err)
		os.Exit(1)
	}

	// Build router and market cache
	router, marketCache := api.Router(db, logger)

	// Start market cache background workers
	marketCache.Start(context.Background())

	// Create HTTP server
	srv := &http.Server{
		Addr:         cfg.ServerAddr(),
		Handler:      router,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}

	// Start server in background
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server failed", "error", err)
			os.Exit(1)
		}
	}()

	// Wait for interrupt signal
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Info("shutting down server...")

	// Graceful shutdown with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Stop market cache background workers
	marketCache.Stop()

	if err := srv.Shutdown(ctx); err != nil {
		logger.Error("forced shutdown", "error", err)
		os.Exit(1)
	}

	logger.Info("server stopped")
}

func setupLogger(level string) *slog.Logger {
	var slogLevel slog.Level

	switch level {
	case "debug":
		slogLevel = slog.LevelDebug
	case "info":
		slogLevel = slog.LevelInfo
	case "warn":
		slogLevel = slog.LevelWarn
	case "error":
		slogLevel = slog.LevelError
	default:
		slogLevel = slog.LevelInfo
	}

	handler := slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slogLevel,
	})

	return slog.New(handler)
}

func init() {
	// Ensure the data directory exists for the default database path
	if err := os.MkdirAll("data", 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "failed to create data directory: %v\n", err)
		os.Exit(1)
	}
}
