// Package app assembles the Control Plane: services, handlers, and routes.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/agentapi"
	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/health"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/projects"
	"github.com/jakkayy/ForgeDeck/internal/servers"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// NewRouter wires every module and returns the root HTTP handler. It also
// starts background work (the job notifier) that stops when ctx is cancelled.
func NewRouter(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	authService := auth.NewService(store.New(pool), time.Now)
	authHandler := auth.NewHandler(authService, cfg.CookieSecure)
	authorizer := authz.New(store.New(pool))
	projectHandler := projects.NewHandler(projects.NewService(pool, authorizer))
	jobService := jobs.NewService(pool, authorizer, time.Now)
	serverService := servers.NewService(pool, authorizer, jobService, time.Now)
	serverHandler := servers.NewHandler(serverService)
	notifier := jobs.NewNotifier(pool, logger)
	go notifier.Run(ctx)
	agentHandler := agentapi.NewHandler(ctx, serverService, jobService, notifier)

	router := chi.NewRouter()
	router.Use(httpx.RequestID, httpx.AccessLog(logger), httpx.Recover)
	router.Get("/healthz", health.Handler(map[string]health.Check{"database": pool.Ping}))

	router.Route("/api/v1", func(api chi.Router) {
		api.Use(httpx.CSRF)
		authHandler.Routes(api)
		agentHandler.Routes(api)

		api.Group(func(signedIn chi.Router) {
			signedIn.Use(authHandler.RequireUser)
			projectHandler.Routes(signedIn)
			serverHandler.Routes(signedIn)
			jobs.NewHandler(jobService).Routes(signedIn)
		})
	})
	return router
}
