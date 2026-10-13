package servers

import "time"

// Status is a server's connectivity as derived from its last heartbeat.
type Status string

const (
	// StatusPending means the Agent enrolled but has not sent a heartbeat yet.
	StatusPending Status = "pending"
	// StatusOnline means a heartbeat arrived within two intervals.
	StatusOnline Status = "online"
	// StatusUnreachable means heartbeats stopped recently; it may be a blip.
	StatusUnreachable Status = "unreachable"
	// StatusOffline means no heartbeat for long enough to assume it is down.
	StatusOffline Status = "offline"
)

const (
	onlineWindow      = 2 * HeartbeatInterval
	unreachableWindow = 5 * time.Minute
)

// ConnectivityStatus classifies a server by the age of its last heartbeat.
// Status is computed on read rather than stored, so it never goes stale.
func ConnectivityStatus(lastSeen *time.Time, now time.Time) Status {
	if lastSeen == nil {
		return StatusPending
	}
	switch age := now.Sub(*lastSeen); {
	case age <= onlineWindow:
		return StatusOnline
	case age <= unreachableWindow:
		return StatusUnreachable
	default:
		return StatusOffline
	}
}

// ConnectivityCutoffs returns the oldest last-seen time that still counts as
// online and as unreachable at now: the same windows as ConnectivityStatus,
// for queries that classify many servers at once.
func ConnectivityCutoffs(now time.Time) (online, unreachable time.Time) {
	return now.Add(-onlineWindow), now.Add(-unreachableWindow)
}
