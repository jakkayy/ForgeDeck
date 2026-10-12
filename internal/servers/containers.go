package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/jakkayy/ForgeDeck/internal/authz"
)

// snapshotStaleAfter marks a container list stale when no snapshot has been
// observed for this long. Agents send one every 30–60 seconds.
const snapshotStaleAfter = 2 * time.Minute

// ContainerList is the latest snapshot for a server. It is cached Agent
// data, not a live query: ObservedAt and Stale tell the UI how fresh it is.
type ContainerList struct {
	ServerID   uuid.UUID   `json:"server_id"`
	ObservedAt *time.Time  `json:"observed_at"`
	Stale      bool        `json:"stale"`
	Containers []Container `json:"containers"`
}

// Container is one container from the latest snapshot.
type Container struct {
	ContainerID string            `json:"container_id"`
	Name        string            `json:"name"`
	Image       string            `json:"image"`
	State       string            `json:"state"`
	StatusText  *string           `json:"status_text"`
	Labels      map[string]string `json:"labels"`
}

// ListContainers returns the server's latest container snapshot.
func (s *Service) ListContainers(ctx context.Context, userID, serverID uuid.UUID) (ContainerList, error) {
	row, err := s.authorizeServer(ctx, userID, serverID, authz.ViewProject)
	if err != nil {
		return ContainerList{}, err
	}
	rows, err := s.queries.ListServerContainers(ctx, serverID)
	if err != nil {
		return ContainerList{}, fmt.Errorf("list containers: %w", err)
	}

	now := s.now()
	observedAt := row.Server.ContainersObservedAt
	list := ContainerList{
		ServerID:   serverID,
		ObservedAt: observedAt,
		Stale: observedAt == nil ||
			now.Sub(*observedAt) > snapshotStaleAfter ||
			ConnectivityStatus(row.Server.LastSeenAt, now) != StatusOnline,
		Containers: make([]Container, len(rows)),
	}
	for i, c := range rows {
		var labels map[string]string
		if err := json.Unmarshal(c.Labels, &labels); err != nil {
			return ContainerList{}, fmt.Errorf("decode labels of %s: %w", c.ContainerID, err)
		}
		list.Containers[i] = Container{
			ContainerID: c.ContainerID, Name: c.Name, Image: c.Image,
			State: c.State, StatusText: c.StatusText, Labels: labels,
		}
	}
	return list, nil
}
