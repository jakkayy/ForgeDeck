// Package agentapi serves the machine-to-machine endpoints used by Agents.
package agentapi

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/httpx"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

// Handler serves /agent endpoints.
type Handler struct {
	servers *servers.Service
}

// NewHandler returns Agent endpoints.
func NewHandler(serverService *servers.Service) *Handler {
	return &Handler{servers: serverService}
}

// Routes mounts the Agent endpoints on r.
func (h *Handler) Routes(r chi.Router) {
	r.Post("/agent/enroll", httpx.Handle(h.enroll))
	r.Group(func(agent chi.Router) {
		agent.Use(h.RequireAgent)
		agent.Post("/agent/heartbeat", httpx.Handle(h.heartbeat))
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

func checkProtocol(version int) error {
	if version != agentproto.ProtocolVersion {
		return httpx.BadRequest("unsupported_protocol", fmt.Sprintf(
			"Agent protocol %d is not supported; this Control Plane speaks protocol %d. Upgrade the Agent or Control Plane.",
			version, agentproto.ProtocolVersion))
	}
	return nil
}
