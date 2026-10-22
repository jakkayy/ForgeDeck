package servers

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apperr"
	"github.com/jakkayy/ForgeDeck/internal/authz"
	"github.com/jakkayy/ForgeDeck/internal/jobs"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

var (
	errContainerNotFound = apperr.NotFound("Container not found on this server. Refresh the container list.")
	containerActions     = map[string]bool{
		agentproto.ActionStart: true, agentproto.ActionStop: true, agentproto.ActionRestart: true,
	}
)

// QueuedJob is returned when an operation was handed to an Agent.
type QueuedJob struct {
	JobID  uuid.UUID   `json:"job_id"`
	Status jobs.Status `json:"status"`
}

// RequestContainerAction queues a start/stop/restart for a container the server last reported
func (s *Service) RequestContainerAction(ctx context.Context, userID, serverID uuid.UUID, containerID, action string) (QueuedJob, error) {
	if !containerActions[action] {
		return QueuedJob{}, validate.Errors{"action": "Action must be start, stop or restart."}
	}
	payload := agentproto.ContainerActionPayload{ContainerID: containerID, Action: action}
	return s.enqueueContainerJob(ctx, userID, serverID, containerID, authz.OperateContainer, agentproto.JobContainerAction, payload)
}

// enqueueContainerJob authorizes the user, checks the snapshot, and queues an Agent job
func (s *Service) enqueueContainerJob(ctx context.Context, userID, serverID uuid.UUID, containerID string,
	permission authz.Permission, jobType string, payload any,
) (QueuedJob, error) {
	row, err := s.authorizeServer(ctx, userID, serverID, permission)
	if err != nil {
		return QueuedJob{}, err
	}
	exists, err := s.queries.ServerHasContainer(ctx, store.ServerHasContainerParams{ServerID: serverID, ContainerID: containerID})
	if err != nil {
		return QueuedJob{}, fmt.Errorf("check container: %w", err)
	}
	if !exists {
		return QueuedJob{}, errContainerNotFound
	}

	job, err := s.jobs.Enqueue(ctx, jobs.NewJob{
		ProjectID:      row.Server.ProjectID,
		Type:           jobType,
		WorkerKind:     jobs.WorkerAgent,
		TargetServerID: &serverID,
		Payload:        payload,
		CreatedBy:      &userID,
	})
	if err != nil {
		return QueuedJob{}, err
	}
	return QueuedJob{JobID: job.ID, Status: jobs.Status(job.Status)}, nil
}

// LogRequest selects which past container output to fetch.
type LogRequest struct {
	TailLines  *int
	Since      *time.Time
	Timestamps bool
}

// RequestContainerLogs queues a bounded log fetch
func (s *Service) RequestContainerLogs(ctx context.Context, userID, serverID uuid.UUID, containerID string, req LogRequest) (QueuedJob, error) {
	tail := agentproto.DefaultTailLines
	if req.TailLines != nil {
		if *req.TailLines < 1 || *req.TailLines > agentproto.MaxTailLines {
			return QueuedJob{}, validate.Errors{"tail_lines": fmt.Sprintf("tail_lines must be 1–%d.", agentproto.MaxTailLines)}
		}
		tail = *req.TailLines
	}
	payload := agentproto.FetchLogsPayload{
		ContainerID: containerID, TailLines: tail, Since: req.Since, Timestamps: req.Timestamps,
	}
	return s.enqueueContainerJob(ctx, userID, serverID, containerID, authz.ViewLogs, agentproto.JobFetchLogs, payload)
}
