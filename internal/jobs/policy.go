package jobs

import "github.com/jakkayy/ForgeDeck/internal/agentproto"

// attemptsByType is how many times a job may run when its worker is lost
var attemptsByType = map[string]int32{
	// a repeated restart could restart the container twice
	agentproto.JobContainerAction: 1,
	agentproto.JobFetchLogs:       2,
	// builds are pinned to a commit, so rebuilding is safe
	agentproto.JobBuild: 2,
	// applying a deployment converges to the same state
	agentproto.JobDeployImage: 2,
}

func maxAttempts(jobType string, requested int32) int32 {
	if requested > 0 {
		return requested
	}
	if attempts, ok := attemptsByType[jobType]; ok {
		return attempts
	}
	return 1
}
