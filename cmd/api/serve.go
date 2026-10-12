package main

import (
	"context"
	"log/slog"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/health"
	"github.com/jakkayy/ForgeDeck/internal/httpserver"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
)

func serve(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	if err := migrateUp(ctx, cfg, logger); err != nil {
		return err
	}

	pool, err := database.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer pool.Close()

	router := chi.NewRouter()
	router.Use(httpx.RequestID, httpx.AccessLog(logger), httpx.Recover)
	router.Get("/healthz", health.Handler(map[string]health.Check{"database": pool.Ping}))

	return httpserver.Run(ctx, cfg.HTTPAddr, router, logger)
}

func migrateUp(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	migrator, err := database.NewMigrator(cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer func() { _ = migrator.Close() }()
	return migrator.Up(ctx)
}
