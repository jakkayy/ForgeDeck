package jobs

import "github.com/jakkayy/ForgeDeck/internal/agentproto"

// attemptsByType is how many times a job may run when its worker is lost.
// A lost attempt may still have done its work, so only jobs that are safe
// to repeat get more than one attempt.
var attemptsByType = map[string]int32{
	// A restart that seemed lost may already have happened; repeating it
	// could restart the container twice. The user can retry by hand.
	agentproto.JobContainerAction: 1,
	// Reading logs has no side effects.
	agentproto.JobFetchLogs: 2,
	// Builds are pinned to one commit and keyed by it, so rebuilding is safe.
	agentproto.JobBuild: 2,
	// Applying a deployment converges to the same state (pull, then compose
	// up), so repeating one whose Agent was lost is safe.
	agentproto.JobDeployImage: 2,
}

// maxAttempts returns the attempts for a job: the explicit value when set,
// else the type's policy, else one.
func maxAttempts(jobType string, requested int32) int32 {
	if requested > 0 {
		return requested
	}
	if attempts, ok := attemptsByType[jobType]; ok {
		return attempts
	}
	return 1
}
