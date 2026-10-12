// Package app assembles the Control Plane: services, handlers, and routes.
package app

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/health"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/projects"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// NewRouter wires every module and returns the root HTTP handler.
func NewRouter(cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	authService := auth.NewService(store.New(pool), time.Now)
	authHandler := auth.NewHandler(authService, cfg.CookieSecure)
	authorizer := authz.New(store.New(pool))
	projectHandler := projects.NewHandler(projects.NewService(pool, authorizer))

	router := chi.NewRouter()
	router.Use(httpx.RequestID, httpx.AccessLog(logger), httpx.Recover)
	router.Get("/healthz", health.Handler(map[string]health.Check{"database": pool.Ping}))

	router.Route("/api/v1", func(api chi.Router) {
		api.Use(httpx.CSRF)
		authHandler.Routes(api)

		api.Group(func(signedIn chi.Router) {
			signedIn.Use(authHandler.RequireUser)
			projectHandler.Routes(signedIn)
		})
	})
	return router
}
