// Package overview summarizes what needs attention across every project the
// signed-in user belongs to.
package overview

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/servers"
	"github.com/jakkayy/ForgeDeck/internal/store"
)

const (
	recentLimit  = 8
	failureLimit = 5
)

// Overview is the dashboard summary.
type Overview struct {
	Servers      ServerCounts `json:"servers"`
	Environments EnvCounts    `json:"environments"`
	Recent       []Deployment `json:"recent_deployments"`
	Failures     []Deployment `json:"failing_deployments"`
	GeneratedAt  time.Time    `json:"generated_at"`
}

// ServerCounts groups servers by connectivity.
type ServerCounts struct {
	Total       int64 `json:"total"`
	Online      int64 `json:"online"`
	Unreachable int64 `json:"unreachable"`
	Offline     int64 `json:"offline"`
	Pending     int64 `json:"pending"`
}

// EnvCounts groups environments by how their newest deployment ended.
type EnvCounts struct {
	Total       int64 `json:"total"`
	Succeeded   int64 `json:"succeeded"`
	Failed      int64 `json:"failed"`
	InProgress  int64 `json:"in_progress"`
	NotDeployed int64 `json:"not_deployed"`
}

// Deployment is one deployment with the names needed to link to it.
type Deployment struct {
	ID              uuid.UUID  `json:"id"`
	EnvironmentID   uuid.UUID  `json:"environment_id"`
	EnvironmentName string     `json:"environment_name"`
	ApplicationID   uuid.UUID  `json:"application_id"`
	ApplicationName string     `json:"application_name"`
	ProjectName     string     `json:"project_name"`
	CommitSHA       string     `json:"commit_sha"`
	Status          string     `json:"status"`
	ErrorCode       *string    `json:"error_code"`
	CreatedAt       time.Time  `json:"created_at"`
	FinishedAt      *time.Time `json:"finished_at"`
}

// Service reads the summary. It needs no role check: every query is limited
// to projects the user is a member of, and the data is visible to a viewer.
type Service struct {
	queries *store.Queries
	now     func() time.Time
}

// NewService returns an overview reader.
func NewService(queries *store.Queries, now func() time.Time) *Service {
	return &Service{queries: queries, now: now}
}

// Get summarizes the user's projects.
func (s *Service) Get(ctx context.Context, userID uuid.UUID) (Overview, error) {
	now := s.now()
	online, unreachable := servers.ConnectivityCutoffs(now)
	serverRow, err := s.queries.CountServersByConnectivity(ctx, store.CountServersByConnectivityParams{
		UserID: userID, OnlineAfter: online, UnreachableAfter: unreachable,
	})
	if err != nil {
		return Overview{}, fmt.Errorf("count servers: %w", err)
	}
	envRow, err := s.queries.CountEnvironmentsByLastDeployment(ctx, userID)
	if err != nil {
		return Overview{}, fmt.Errorf("count environments: %w", err)
	}
	recent, err := s.deployments(ctx, userID, false, recentLimit)
	if err != nil {
		return Overview{}, err
	}
	failing, err := s.deployments(ctx, userID, true, failureLimit)
	if err != nil {
		return Overview{}, err
	}
	return Overview{
		Servers: ServerCounts{
			Total: serverRow.Total, Online: serverRow.Online, Unreachable: serverRow.Unreachable,
			Offline: serverRow.Offline, Pending: serverRow.Pending,
		},
		Environments: EnvCounts(envRow),
		Recent:       recent,
		Failures:     failing,
		GeneratedAt:  now,
	}, nil
}

func (s *Service) deployments(ctx context.Context, userID uuid.UUID, onlyFailed bool, limit int32) ([]Deployment, error) {
	rows, err := s.queries.ListRecentDeploymentsForUser(ctx, store.ListRecentDeploymentsForUserParams{
		UserID: userID, OnlyFailed: onlyFailed, PageLimit: limit,
	})
	if err != nil {
		return nil, fmt.Errorf("list recent deployments: %w", err)
	}
	out := make([]Deployment, len(rows))
	for i, row := range rows {
		out[i] = Deployment{
			ID: row.ID, EnvironmentID: row.EnvironmentID, EnvironmentName: row.EnvironmentName,
			ApplicationID: row.ApplicationID, ApplicationName: row.ApplicationName, ProjectName: row.ProjectName,
			CommitSHA: row.CommitSha, Status: row.Status, ErrorCode: row.ErrorCode,
			CreatedAt: row.CreatedAt, FinishedAt: row.FinishedAt,
		}
	}
	return out, nil
}
