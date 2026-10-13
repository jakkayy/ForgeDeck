// Package app assembles the Control Plane: services, handlers, and routes.
package app

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/agentapi"
	"github.com/jakkayy/ForgeDeck/internal/applications"
	"github.com/jakkayy/ForgeDeck/internal/audit"
	"github.com/jakkayy/ForgeDeck/internal/auth"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/builds"
	"github.com/jakkayy/ForgeDeck/internal/config"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
	"github.com/jakkayy/ForgeDeck/internal/environments"
	"github.com/jakkayy/ForgeDeck/internal/gitconn"
	"github.com/jakkayy/ForgeDeck/internal/gitprovider/github"
	"github.com/jakkayy/ForgeDeck/internal/health"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/overview"
	"github.com/jakkayy/ForgeDeck/internal/projects"
	"github.com/jakkayy/ForgeDeck/internal/registries"
	"github.com/jakkayy/ForgeDeck/internal/retention"
	"github.com/jakkayy/ForgeDeck/internal/runnerapi"
	"github.com/jakkayy/ForgeDeck/internal/runners"
	"github.com/jakkayy/ForgeDeck/internal/secrets"
	"github.com/jakkayy/ForgeDeck/internal/servers"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/webhooks"
	"github.com/jakkayy/ForgeDeck/internal/webui"
)

// retentionInterval is how often old data is cleaned up.
const retentionInterval = 6 * time.Hour

// NewRouter wires every module and returns the root HTTP handler. It also
// starts background work (job notifier, lease recovery) that stops when ctx
// is cancelled.
func NewRouter(ctx context.Context, cfg config.Config, pool *pgxpool.Pool, logger *slog.Logger) http.Handler {
	authService := auth.NewService(store.New(pool), time.Now)
	authHandler := auth.NewHandler(authService, cfg.CookieSecure)
	authorizer := authz.New(store.New(pool))
	projectHandler := projects.NewHandler(projects.NewService(pool, authorizer))
	auditRecorder := audit.NewRecorder(store.New(pool), logger)
	jobService := jobs.NewService(pool, authorizer, time.Now)
	serverService := servers.NewService(pool, authorizer, jobService, time.Now)
	serverHandler := servers.NewHandler(serverService)
	notifier := jobs.NewNotifier(pool, logger)
	go notifier.Run(ctx)
	go jobService.RunRecovery(ctx, logger)
	go retention.NewService(store.New(pool), cfg.Retention, time.Now).Run(ctx, logger, retentionInterval)
	var githubClient *github.Client
	if cfg.GitHub != nil {
		githubClient = github.NewClient(*cfg.GitHub, time.Now)
	}
	gitService := gitconn.NewService(gitconn.Deps{
		Pool: pool, Authz: authorizer, Keyring: cfg.Keyring, GitHub: githubClient, Outbound: cfg.Outbound,
		PublicURL: cfg.PublicURL, Now: time.Now,
	})
	gitHandler := gitconn.NewHandler(gitService, logger)
	registryService := registries.NewService(store.New(pool), authorizer, cfg.Keyring)
	secretService := secrets.NewService(store.New(pool), authorizer, cfg.Keyring)
	jobService.SetLogRedactor(secrets.NewRedactor(secretService))
	runnerHandler := runnerapi.NewHandler(runnerapi.Deps{
		Shutdown: ctx, Runners: runners.NewService(store.New(pool), time.Now), Jobs: jobService, Notifier: notifier,
		Credentials: builds.NewCredentials(store.New(pool), gitService, registryService), Logger: logger,
	})
	appHandler := applications.NewHandler(applications.NewService(store.New(pool), authorizer, gitService, registryService))

	deploymentService := deployments.NewService(pool, authorizer, jobService, auditRecorder)
	agentHandler := agentapi.NewHandler(ctx, serverService, jobService, notifier,
		agentapi.WithDeployContext(deployments.NewContextMinter(store.New(pool), registryService, secretService), logger))
	deploymentHandler := deployments.NewHandler(deploymentService)
	jobService.OnSucceeded(deploymentService.AutoDeploy(logger))
	jobService.OnSucceeded(deploymentService.RecordRelease)

	limits := newLimiters(time.Now)
	router := chi.NewRouter()
	router.Use(httpx.RequestID, httpx.TrustedProxies(cfg.TrustedProxies), httpx.AccessLog(logger), httpx.Recover,
		httpx.SecurityHeaders(cfg.CookieSecure), middleware.GetHead)
	router.Get("/healthz", health.Handler(map[string]health.Check{"database": pool.Ping}))

	router.Route("/api/v1", func(api chi.Router) {
		api.Use(httpx.CSRF, httpx.NoStore)
		api.NotFound(httpx.Handle(func(http.ResponseWriter, *http.Request) error {
			return httpx.NotFound("No API endpoint at this path.")
		}))
		api.Group(func(authRoutes chi.Router) {
			authRoutes.Use(auditRecorder.Middleware)
			authHandler.Routes(authRoutes)
		})
		api.Group(func(agentRoutes chi.Router) {
			agentRoutes.Use(limits.enrollment, httpx.ThrottleFailures(limits.failures), auditRecorder.Middleware)
			agentHandler.Routes(agentRoutes)
		})
		api.Group(func(runnerRoutes chi.Router) {
			runnerRoutes.Use(httpx.ThrottleFailures(limits.failures))
			runnerHandler.Routes(runnerRoutes)
		})
		gitHandler.PublicRoutes(api)
		api.Group(func(webhookRoutes chi.Router) {
			webhookRoutes.Use(httpx.ThrottleFailures(limits.failures))
			webhooks.NewHandler(cfg.GitHubWebhookSecret, gitService, store.New(pool), jobService, auditRecorder).Routes(webhookRoutes)
		})

		api.Group(func(signedIn chi.Router) {
			signedIn.Use(authHandler.RequireUser, limits.userMutations, auditRecorder.Middleware)
			projectHandler.Routes(signedIn)
			serverHandler.Routes(signedIn)
			overview.NewHandler(overview.NewService(store.New(pool), time.Now)).Routes(signedIn)
			jobs.NewHandler(ctx, jobService, notifier).Routes(signedIn)
			gitHandler.Routes(signedIn)
			appHandler.Routes(signedIn)
			registries.NewHandler(registryService).Routes(signedIn)
			secrets.NewHandler(secretService).Routes(signedIn)
			audit.NewHandler(audit.NewReader(store.New(pool), authorizer)).Routes(signedIn)
			deploymentHandler.Routes(signedIn)
			environments.NewHandler(environments.NewService(store.New(pool), authorizer, secretService)).Routes(signedIn)
			builds.NewHandler(builds.NewService(store.New(pool), authorizer, gitService, jobService)).Routes(signedIn)
		})
	})
	router.Get("/*", webui.Handler(webui.FS()).ServeHTTP)
	return router
}
