// Package runnerapi serves the machine-to-machine endpoints used by build
// Runners. It speaks the Agent protocol (internal/agentproto) under
// /runner/, with Runner identities instead of Agent ones.
package runnerapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/builds"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/runners"
)

var errUnauthorized = httpx.Unauthorized("Runner credential is invalid or the Runner was deleted.")

// CredentialMinter issues the credentials for one attempt of a build job.
type CredentialMinter interface {
	Mint(ctx context.Context, payload json.RawMessage) (*agentproto.BuildCredentials, error)
}

// Handler serves /runner endpoints.
type Handler struct {
	shutdown    context.Context
	runners     *runners.Service
	jobs        *jobs.Service
	notifier    *jobs.Notifier
	credentials CredentialMinter
	logger      *slog.Logger
}

// Deps are the collaborators a Handler needs.
type Deps struct {
	// Shutdown ends open long-polls when cancelled.
	Shutdown    context.Context
	Runners     *runners.Service
	Jobs        *jobs.Service
	Notifier    *jobs.Notifier
	Credentials CredentialMinter
	Logger      *slog.Logger
}

// NewHandler returns Runner endpoints.
func NewHandler(d Deps) *Handler {
	return &Handler{
		shutdown: d.Shutdown, runners: d.Runners, jobs: d.Jobs, notifier: d.Notifier,
		credentials: d.Credentials, logger: d.Logger,
	}
}

// Routes mounts the Runner endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Group(func(runner chi.Router) {
		runner.Use(h.requireRunner)
		runner.Post("/runner/heartbeat", httpx.Handle(h.heartbeat))
		runner.Post("/runner/jobs/claim", httpx.Handle(h.claim))
		runner.Post("/runner/jobs/{jobID}/heartbeat", httpx.Handle(h.renewLease))
		runner.Post("/runner/jobs/{jobID}/events", httpx.Handle(h.appendEvents))
		runner.Post("/runner/jobs/{jobID}/complete", httpx.Handle(h.complete))
	})
}

type identityKey struct{}

func (h *Handler) requireRunner(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := h.authenticate(r.Context(), r)
		if err != nil {
			httpx.WriteError(w, r, err)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), identityKey{}, identity)))
	})
}

func (h *Handler) authenticate(ctx context.Context, r *http.Request) (runners.Identity, error) {
	credential, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
	if !ok || credential == "" {
		return runners.Identity{}, errUnauthorized
	}
	identity, err := h.runners.Authenticate(ctx, credential)
	if errors.Is(err, runners.ErrInvalidCredential) {
		return runners.Identity{}, errUnauthorized
	}
	return identity, err
}

func identityFrom(ctx context.Context) runners.Identity {
	identity, ok := ctx.Value(identityKey{}).(runners.Identity)
	if !ok {
		panic("runnerapi: route without requireRunner")
	}
	return identity
}

func (h *Handler) heartbeat(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.HeartbeatRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := checkProtocol(req.ProtocolVersion); err != nil {
		return err
	}
	if err := h.runners.RecordHeartbeat(r.Context(), identityFrom(r.Context()).ID, req.AgentVersion); err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, agentproto.HeartbeatResponse{IntervalSeconds: 20})
}

func (h *Handler) claim(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.ClaimRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	wait := agentproto.MaxClaimWaitSeconds
	if req.WaitSeconds != nil {
		if *req.WaitSeconds < 0 || *req.WaitSeconds > agentproto.MaxClaimWaitSeconds {
			return httpx.BadRequest("invalid_wait", fmt.Sprintf("wait_seconds must be 0–%d.", agentproto.MaxClaimWaitSeconds))
		}
		wait = *req.WaitSeconds
	}
	maxWait := time.Duration(wait) * time.Second

	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	defer context.AfterFunc(h.shutdown, cancel)()
	_ = http.NewResponseController(w).SetWriteDeadline(time.Now().Add(maxWait + 10*time.Second))

	var credentials *agentproto.BuildCredentials
	claim := func(ctx context.Context) (*jobs.Claimed, error) {
		job, minted, err := h.claimWithCredentials(ctx, identityFrom(ctx).ID.String())
		credentials = minted
		return job, err
	}
	stillAuthorized := func(ctx context.Context) error {
		_, err := h.authenticate(ctx, r)
		return err
	}
	job, err := jobs.ClaimWait(ctx, h.notifier, jobs.RunnerKey, claim, maxWait, stillAuthorized)
	if err != nil {
		return err
	}
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return httpx.WriteJSON(w, http.StatusOK, agentproto.Job{
		ID: job.ID, Type: job.Type, Attempt: job.Attempt, Payload: job.Payload, LeaseExpiresAt: job.LeaseExpiresAt,
		Credentials: credentials,
	})
}

// claimWithCredentials claims the next job and mints its build credentials.
// An attempt that cannot start (no image destination, credentials cannot be
// minted) fails at once, so the Runner
// is never handed a build it cannot start; the next job is tried instead.
func (h *Handler) claimWithCredentials(ctx context.Context, workerID string) (*jobs.Claimed, *agentproto.BuildCredentials, error) {
	for {
		job, err := h.jobs.ClaimForRunner(ctx, workerID)
		if err != nil || job == nil || job.Type != agentproto.JobBuild {
			return job, nil, err
		}
		credentials, err := h.credentials.Mint(ctx, job.Payload)
		if err == nil {
			return job, credentials, nil
		}
		h.logger.WarnContext(ctx, "build cannot start", "job_id", job.ID, "error", err)
		var buildErr *builds.Error
		if !errors.As(err, &buildErr) {
			buildErr = &builds.Error{Code: "build_credentials_unavailable", Message: "ForgeDeck could not prepare credentials for this build."}
		}
		lease := jobs.Lease{JobID: job.ID, Attempt: job.Attempt, WorkerKind: jobs.WorkerRunner, WorkerID: workerID}
		if err := h.jobs.Complete(ctx, lease, agentproto.CompleteRequest{
			Outcome: agentproto.OutcomeFailed, ErrorCode: buildErr.Code, ErrorMessage: buildErr.Message,
		}); err != nil {
			return nil, nil, err
		}
	}
}

func (h *Handler) renewLease(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.LeaseRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	lease, err := runnerLease(r, req.Attempt)
	if err != nil {
		return err
	}
	expiresAt, err := h.jobs.RenewLease(r.Context(), lease)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, agentproto.LeaseResponse{LeaseExpiresAt: expiresAt})
}

func (h *Handler) appendEvents(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.EventsRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	lease, err := runnerLease(r, req.Attempt)
	if err != nil {
		return err
	}
	if err := h.jobs.AppendLogs(r.Context(), lease, req.Events); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func (h *Handler) complete(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.CompleteRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	lease, err := runnerLease(r, req.Attempt)
	if err != nil {
		return err
	}
	if err := h.jobs.Complete(r.Context(), lease, req); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

func runnerLease(r *http.Request, attempt int32) (jobs.Lease, error) {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		return jobs.Lease{}, err
	}
	return jobs.Lease{
		JobID: jobID, Attempt: attempt, WorkerKind: jobs.WorkerRunner, WorkerID: identityFrom(r.Context()).ID.String(),
	}, nil
}

func checkProtocol(version int) error {
	if version != agentproto.ProtocolVersion {
		return httpx.BadRequest("unsupported_protocol", fmt.Sprintf(
			"Runner protocol %d is not supported; this Control Plane speaks protocol %d.", version, agentproto.ProtocolVersion))
	}
	return nil
}
