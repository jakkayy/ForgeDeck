package main

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/health"
	"github.com/jakkayy/ForgeDeck/internal/httpserver"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/store"
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

	return httpserver.Run(ctx, cfg.HTTPAddr, newRouter(cfg, pool, logger), logger)
}

func newRouter(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	queries := store.New(pool)
	authService := auth.NewService(queries, time.Now)

	router := chi.NewRouter()
	router.Use(httpx.RequestID, httpx.AccessLog(logger), httpx.Recover)
	router.Get("/healthz", health.Handler(map[string]health.Check{"database": pool.Ping}))

	router.Route("/api/v1", func(api chi.Router) {
		auth.NewHandler(authService, cfg.CookieSecure).Routes(api)
	})
	return router
}

func migrateUp(ctx context.Context, cfg config.Config, logger *slog.Logger) error {
	migrator, err := database.NewMigrator(cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer func() { _ = migrator.Close() }()
	return migrator.Up(ctx)
}
