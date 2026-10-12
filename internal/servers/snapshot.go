package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/validate"
)

const (
	maxSnapshotContainers = 500
	maxContainerLabels    = 64
	maxLabelBytes         = 1024
	maxContainerText      = 512
	// clockSkewTolerance is how far ahead of the Control Plane an Agent's
	// clock may be before observed_at is clamped to the receive time.
	clockSkewTolerance = 30 * time.Second
)

var (
	validContainerID = regexp.MustCompile(`^[a-f0-9]{12,64}$`)
	containerStates  = map[string]bool{
		"created": true, "running": true, "paused": true, "restarting": true,
		"removing": true, "exited": true, "dead": true,
	}
)

// ApplySnapshot replaces the server's stored containers with snapshot unless
// a newer snapshot was already stored. It reports whether it was applied.
func (s *Service) ApplySnapshot(ctx context.Context, identity AgentIdentity, snapshot agentproto.ContainerSnapshot) (bool, error) {
	if err := validateSnapshot(snapshot); err != nil {
		return false, err
	}
	// PostgreSQL stores microseconds; truncate so comparisons with the stored
	// time are exact.
	receivedAt := s.now().Truncate(time.Microsecond)
	observedAt := snapshot.ObservedAt.Truncate(time.Microsecond)
	if observedAt.After(receivedAt.Add(clockSkewTolerance)) {
		observedAt = receivedAt
	}
	rows, err := snapshotRows(identity, snapshot.Containers, observedAt, receivedAt)
	if err != nil {
		return false, err
	}

	applied := false
	err = database.InTx(ctx, s.pool, func(tx pgx.Tx) error {
		q := s.queries.WithTx(tx)
		stored, err := q.LockServerSnapshotTime(ctx, identity.ServerID)
		if err != nil {
			return fmt.Errorf("lock server: %w", err)
		}
		if stored != nil && !observedAt.After(*stored) {
			return nil
		}
		if err := q.DeleteServerContainers(ctx, identity.ServerID); err != nil {
			return fmt.Errorf("clear containers: %w", err)
		}
		if _, err := q.InsertServerContainers(ctx, rows); err != nil {
			return fmt.Errorf("insert containers: %w", err)
		}
		if err := q.SetServerSnapshotTime(ctx, store.SetServerSnapshotTimeParams{ID: identity.ServerID, ContainersObservedAt: &observedAt}); err != nil {
			return fmt.Errorf("set snapshot time: %w", err)
		}
		applied = true
		return nil
	})
	return applied, err
}

func snapshotRows(identity AgentIdentity, containers []agentproto.Container, observedAt, receivedAt time.Time) ([]store.InsertServerContainersParams, error) {
	rows := make([]store.InsertServerContainersParams, len(containers))
	for i, c := range containers {
		labels := []byte("{}")
		if len(c.Labels) > 0 {
			var err error
			if labels, err = json.Marshal(c.Labels); err != nil {
				return nil, fmt.Errorf("encode labels: %w", err)
			}
		}
		rows[i] = store.InsertServerContainersParams{
			ServerID:    identity.ServerID,
			ContainerID: c.ID,
			Name:        c.Name,
			Image:       c.Image,
			State:       c.State,
			StatusText:  optional(c.StatusText),
			Labels:      labels,
			ObservedAt:  observedAt,
			ReceivedAt:  receivedAt,
		}
	}
	return rows, nil
}

func validateSnapshot(snapshot agentproto.ContainerSnapshot) error {
	errs := validate.Errors{}
	if snapshot.ObservedAt.IsZero() {
		errs.Add("observed_at", "observed_at is required.")
	}
	if len(snapshot.Containers) > maxSnapshotContainers {
		errs.Add("containers", fmt.Sprintf("At most %d containers per snapshot.", maxSnapshotContainers))
		return errs.Err()
	}
	seen := make(map[string]bool, len(snapshot.Containers))
	for i, c := range snapshot.Containers {
		field := fmt.Sprintf("containers[%d]", i)
		switch {
		case !validContainerID.MatchString(c.ID):
			errs.Add(field+".container_id", "Must be a lowercase hex Docker container ID.")
		case seen[c.ID]:
			errs.Add(field+".container_id", "Duplicate container ID.")
		}
		seen[c.ID] = true
		if !containerStates[c.State] {
			errs.Add(field+".state", "Unknown container state.")
		}
		if c.Name == "" || len(c.Name) > maxContainerText || len(c.Image) > maxContainerText || len(c.StatusText) > maxContainerText {
			errs.Add(field, "Name is required and text fields must be at most 512 bytes.")
		}
		if len(c.Labels) > maxContainerLabels || labelBytes(c.Labels) > maxLabelBytes*maxContainerLabels {
			errs.Add(field+".labels", "Too many or too large labels.")
		}
	}
	return errs.Err()
}

func labelBytes(labels map[string]string) int {
	n := 0
	for k, v := range labels {
		n += len(k) + len(v)
	}
	return n
}
