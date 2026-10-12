// Package agentapi serves the machine-to-machine endpoints used by Agents.
package agentapi

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
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
}

// NewHandler returns Agent endpoints. Long-poll requests end when shutdown
// is cancelled so they do not hold up graceful shutdown.
func NewHandler(shutdown context.Context, serverService *servers.Service, jobService *jobs.Service, notifier *jobs.Notifier) *Handler {
	return &Handler{shutdown: shutdown, servers: serverService, jobs: jobService, notifier: notifier}
}

// Routes mounts the Agent endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/agent/enroll", httpx.Handle(h.enroll))
	r.Group(func(agent chi.Router) {
		agent.Use(h.RequireAgent)
		agent.Post("/agent/heartbeat", httpx.Handle(h.heartbeat))
		agent.Post("/agent/containers/snapshot", httpx.Handle(h.snapshot))
		agent.Post("/agent/jobs/claim", httpx.Handle(h.claim))
		agent.Post("/agent/jobs/{jobID}/heartbeat", httpx.Handle(h.renewLease))
		agent.Post("/agent/jobs/{jobID}/events", httpx.Handle(h.appendEvents))
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
		IntervalSeconds: int(servers.HeartbeatInterval / time.Second),
	})
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
	job, err := h.jobs.ClaimForAgentWait(ctx, h.notifier, identity.ServerID, identity.AgentID.String(), maxWait,
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
	})
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
	expiresAt, err := h.jobs.RenewAgentLease(r.Context(), lease)
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
	if err := h.jobs.AppendAgentLogs(r.Context(), lease, req.Events); err != nil {
		return err
	}
	w.WriteHeader(http.StatusNoContent)
	return nil
}

// agentLease identifies the attempt named in the URL as held by the
// authenticated Agent, so the job queries can verify ownership.
func agentLease(r *http.Request, attempt int32) (jobs.AgentLease, error) {
	jobID, err := httpx.UUIDParam(r, "jobID")
	if err != nil {
		return jobs.AgentLease{}, err
	}
	identity := IdentityFrom(r.Context())
	return jobs.AgentLease{
		JobID: jobID, Attempt: attempt, ServerID: identity.ServerID, WorkerID: identity.AgentID.String(),
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
