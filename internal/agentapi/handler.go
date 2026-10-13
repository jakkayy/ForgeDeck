// Package agentapi serves the machine-to-machine endpoints used by Agents.
package agentapi

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/deployments"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

// Handler serves /agent endpoints.
type Handler struct {
	shutdown context.Context
	servers  *servers.Service
	jobs     *jobs.Service
	notifier *jobs.Notifier
	deploy   DeployContextMinter
	logger   *slog.Logger
}

// DeployContextMinter builds what an Agent needs, beyond the payload, to run
// one claimed deploy_image job (registry credentials, previous release).
type DeployContextMinter interface {
	Mint(ctx context.Context, payload json.RawMessage) (*agentproto.DeployContext, error)
}

// Option configures a Handler.
type Option func(*Handler)

// WithDeployContext makes claims of deploy_image jobs carry a DeployContext.
// A claim that cannot be given one fails the attempt at once.
func WithDeployContext(minter DeployContextMinter, logger *slog.Logger) Option {
	return func(h *Handler) { h.deploy, h.logger = minter, logger }
}

// NewHandler returns Agent endpoints. Long-poll requests end when shutdown
// is cancelled so they do not hold up graceful shutdown.
func NewHandler(shutdown context.Context, serverService *servers.Service, jobService *jobs.Service, notifier *jobs.Notifier, options ...Option) *Handler {
	h := &Handler{shutdown: shutdown, servers: serverService, jobs: jobService, notifier: notifier}
	for _, option := range options {
		option(h)
	}
	return h
}

// Routes mounts the Agent endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/agent/enroll", httpx.Handle(h.enroll))
	r.Group(func(agent chi.Router) {
		agent.Use(h.RequireAgent)
		agent.Post("/agent/heartbeat", httpx.Handle(h.heartbeat))
		agent.Post("/agent/rotate-credential", httpx.Handle(h.rotateCredential))
		agent.Post("/agent/containers/snapshot", httpx.Handle(h.snapshot))
		agent.Post("/agent/jobs/claim", httpx.Handle(h.claim))
		agent.Post("/agent/jobs/{jobID}/heartbeat", httpx.Handle(h.renewLease))
		agent.Post("/agent/jobs/{jobID}/events", httpx.Handle(h.appendEvents))
		agent.Post("/agent/jobs/{jobID}/complete", httpx.Handle(h.complete))
	})
}

func (h *Handler) enroll(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.EnrollRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := checkProtocol(req.ProtocolVersion); err != nil {
		return err
	}

	resp, err := h.servers.Enroll(r.Context(), req.Token, req.AgentVersion)
	if errors.Is(err, servers.ErrInvalidEnrollmentToken) {
		return httpx.Unauthorized("Enrollment token is invalid, expired, or already used. Create a new one in the dashboard.")
	}
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	return httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (h *Handler) heartbeat(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.HeartbeatRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	if err := checkProtocol(req.ProtocolVersion); err != nil {
		return err
	}
	if err := h.servers.RecordHeartbeat(r.Context(), IdentityFrom(r.Context()), req); err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, agentproto.HeartbeatResponse{
		IntervalSeconds:  int(servers.HeartbeatInterval / time.Second),
		RotateCredential: IdentityFrom(r.Context()).RotationDue,
	})
}

// rotateCredential gives the Agent a new credential. The response is the only
// time it is shown, so it is not cacheable.
func (h *Handler) rotateCredential(w http.ResponseWriter, r *http.Request) error {
	resp, err := h.servers.RotateCredential(r.Context(), IdentityFrom(r.Context()), bearerCredential(r))
	if errors.Is(err, servers.ErrInvalidAgentCredential) {
		return errAgentUnauthorized
	}
	if err != nil {
		return err
	}
	w.Header().Set("Cache-Control", "no-store")
	return httpx.WriteJSON(w, http.StatusOK, resp)
}

func (h *Handler) snapshot(w http.ResponseWriter, r *http.Request) error {
	var req agentproto.ContainerSnapshot
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		return err
	}
	accepted, err := h.servers.ApplySnapshot(r.Context(), IdentityFrom(r.Context()), req)
	if err != nil {
		return err
	}
	return httpx.WriteJSON(w, http.StatusOK, agentproto.SnapshotResponse{Accepted: accepted})
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

	identity := IdentityFrom(ctx)
	credential := bearerCredential(r)
	var deploy *agentproto.DeployContext
	claim := func(ctx context.Context) (*jobs.Claimed, error) {
		job, minted, err := h.claimWithContext(ctx, identity)
		deploy = minted
		return job, err
	}
	job, err := jobs.ClaimWait(ctx, h.notifier, identity.ServerID.String(), claim, maxWait,
		func(ctx context.Context) error {
			_, err := h.servers.AuthenticateAgent(ctx, credential)
			if errors.Is(err, servers.ErrInvalidAgentCredential) {
				return errAgentUnauthorized
			}
			return err
		})
	if err != nil {
		return err
	}
	if job == nil {
		w.WriteHeader(http.StatusNoContent)
		return nil
	}
	return httpx.WriteJSON(w, http.StatusOK, agentproto.Job{
		ID: job.ID, Type: job.Type, Attempt: job.Attempt, Payload: job.Payload, LeaseExpiresAt: job.LeaseExpiresAt,
		Deploy: deploy,
	})
}

// claimWithContext claims the next job for the Agent and, for a deploy_image
// job, mints its DeployContext. An attempt that cannot start fails at once,
// so the Agent is never handed a deployment it cannot run; the next job is
// tried instead.
func (h *Handler) claimWithContext(ctx context.Context, identity servers.AgentIdentity) (*jobs.Claimed, *agentproto.DeployContext, error) {
	for {
		job, err := h.jobs.ClaimForAgent(ctx, identity.ServerID, identity.AgentID.String())
		if err != nil || job == nil || job.Type != agentproto.JobDeployImage || h.deploy == nil {
			return job, nil, err
		}
		minted, err := h.deploy.Mint(ctx, job.Payload)
		if err == nil {
			return job, minted, nil
		}
		h.logger.WarnContext(ctx, "deployment cannot start", "job_id", job.ID, "error", err)
		var mintErr *deployments.MintError
		if !errors.As(err, &mintErr) {
			mintErr = &deployments.MintError{Code: "deploy_context_unavailable", Message: "ForgeDeck could not prepare this deployment."}
		}
		lease := jobs.Lease{JobID: job.ID, Attempt: job.Attempt, WorkerKind: jobs.WorkerAgent,
			WorkerID: identity.AgentID.String(), ServerID: &identity.ServerID}
		if err := h.jobs.Complete(ctx, lease, agentproto.CompleteRequest{
			Outcome: agentproto.OutcomeFailed, ErrorCode: mintErr.Code, ErrorMessage: mintErr.Message,
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
	lease, err := agentLease(r, req.Attempt)
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
	lease, err := agentLease(r, req.Attempt)
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
	lease, err := agentLease(r, req.Attempt)
	if err != nil {
		return err
	}
	if err := h.jobs.Complete(r.Context(), lease, req); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// agentLease identifies the attempt named in the URL as held by the
// authenticated Agent, so the job queries can verify ownership.
func agentLease(r *http.Request, attempt int32) (jobs.Lease, error) {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		return jobs.Lease{}, err
	}
	identity := IdentityFrom(r.Context())
	return jobs.Lease{
		JobID: jobID, Attempt: attempt, WorkerKind: jobs.WorkerAgent,
		WorkerID: identity.AgentID.String(), ServerID: &identity.ServerID,
	}, nil
}

func checkProtocol(version int) error {
	if version != agentproto.ProtocolVersion {
		return httpx.BadRequest("unsupported_protocol", fmt.Sprintf(
			"Agent protocol %d is not supported; this Control Plane speaks protocol %d. Upgrade the Agent or Control Plane.",
			version, agentproto.ProtocolVersion))
	}
	return nil
}
