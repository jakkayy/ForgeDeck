package main

import (
	"context"
	"log/slog"

	"github.com/jakkayy/ForgeDeck/internal/app"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/httpserver"
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

	return httpserver.Run(ctx, cfg.HTTPAddr, app.NewRouter(cfg, pool, logger), logger)
}

func migrateUp(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	migrator, err := database.NewMigrator(cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer func() { _ = migrator.Close() }()
	return migrator.Up(ctx)
}
