package servers

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

// errServerNotFound is also returned for servers in projects the user does
// not belong to, so server IDs cannot be probed.
var errServerNotFound = apperr.NotFound("Server not found.")

// Server is the dashboard view of a server.
type Server struct {
	ID           uuid.UUID  `json:"id"`
	ProjectID    uuid.UUID  `json:"project_id"`
	ProjectName  string     `json:"project_name"`
	Name         string     `json:"name"`
	Status       Status     `json:"status"`
	LastSeenAt   *time.Time `json:"last_seen_at"`
	AgentVersion *string    `json:"agent_version"`
	Docker       Docker     `json:"docker"`
	Resources    *Resources `json:"resources"`
	CreatedAt    time.Time  `json:"created_at"`
}

// Docker is the last reported Docker Engine state.
type Docker struct {
	Available bool    `json:"available"`
	Version   *string `json:"version"`
}

// Resources is the last reported host usage in percent.
type Resources struct {
	CPUPercent    float32 `json:"cpu_percent"`
	MemoryPercent float32 `json:"memory_percent"`
	DiskPercent   float32 `json:"disk_percent"`
}

// Get returns a server the user may view.
func (s *Service) Get(ctx context.Context, userID, serverID uuid.UUID) (Server, error) {
	row, err := s.authorizeServer(ctx, userID, serverID, authz.ViewProject)
	if err != nil {
		return Server{}, err
	}
	return s.toServer(row.Server, row.ProjectName), nil
}

// ListFilter narrows a server list. Search is a LIKE-escaped substring of the
// name; Status is one of the connectivity statuses.
type ListFilter struct {
	Search *string
	Status *string
}

// ListForUser returns servers across every project the user belongs to.
func (s *Service) ListForUser(ctx context.Context, userID uuid.UUID, filter ListFilter, limit, offset int32) ([]Server, error) {
	online, unreachable := ConnectivityCutoffs(s.now())
	rows, err := s.queries.ListServersForUser(ctx, store.ListServersForUserParams{
		UserID: userID, Search: filter.Search, Status: filter.Status,
		OnlineAfter: online, UnreachableAfter: unreachable, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list servers: %w", err)
	}
	servers := make([]Server, len(rows))
	for i, row := range rows {
		servers[i] = s.toServer(row.Server, row.ProjectName)
	}
	return servers, nil
}

// ListForProject returns the servers of one project.
func (s *Service) ListForProject(ctx context.Context, userID, projectID uuid.UUID, limit, offset int32) ([]Server, error) {
	if _, err := s.authz.Require(ctx, userID, projectID, authz.ViewProject); err != nil {
		return nil, err
	}
	rows, err := s.queries.ListProjectServers(ctx, store.ListProjectServersParams{
		ProjectID: projectID, PageLimit: limit, PageOffset: offset,
	})
	if err != nil {
		return nil, fmt.Errorf("list project servers: %w", err)
	}
	servers := make([]Server, len(rows))
	for i, row := range rows {
		servers[i] = s.toServer(row.Server, row.ProjectName)
	}
	return servers, nil
}

// authorizeServer loads a server and checks the user's permission in its
// project. Unknown servers and non-members both get errServerNotFound.
func (s *Service) authorizeServer(ctx context.Context, userID, serverID uuid.UUID, p authz.Permission) (store.GetServerRow, error) {
	row, err := s.queries.GetServer(ctx, serverID)
	if errors.Is(err, pgx.ErrNoRows) {
		return store.GetServerRow{}, errServerNotFound
	}
	if err != nil {
		return store.GetServerRow{}, fmt.Errorf("get server: %w", err)
	}
	if _, err := s.authz.Require(ctx, userID, row.Server.ProjectID, p); err != nil {
		if errors.Is(err, authz.ErrProjectNotFound) {
			return store.GetServerRow{}, errServerNotFound
		}
		return store.GetServerRow{}, err
	}
	return row, nil
}

func (s *Service) toServer(row store.Server, projectName string) Server {
	server := Server{
		ID:           row.ID,
		ProjectID:    row.ProjectID,
		ProjectName:  projectName,
		Name:         row.Name,
		Status:       ConnectivityStatus(row.LastSeenAt, s.now()),
		LastSeenAt:   row.LastSeenAt,
		AgentVersion: row.AgentVersion,
		Docker:       Docker{Available: row.DockerAvailable, Version: row.DockerVersion},
		CreatedAt:    row.CreatedAt,
	}
	if row.CpuPercent != nil && row.MemoryPercent != nil && row.DiskPercent != nil {
		server.Resources = &Resources{
			CPUPercent: *row.CpuPercent, MemoryPercent: *row.MemoryPercent, DiskPercent: *row.DiskPercent,
		}
	}
	return server
}
