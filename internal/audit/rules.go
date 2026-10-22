package audit

// Kind says how to find the project an event belongs to from the URL
type Kind int

const (
	KindNone Kind = iota
	KindProject
	KindApplication
	KindEnvironment
	KindRelease
	KindJob
	KindGitConnection
	KindRegistry
	KindServer
)

// Rule describes how one route is audited
type Rule struct {
	Action          string
	TargetType      string
	Kind            Kind
	Param           string
	TargetIDParam   string
	TargetNameParam string
	Actor           string
	// the route is used by an Agent, so the event is the Agent's own
	ResponseServer bool
	FromResponse   bool
	// list only fields that are never sensitive
	RequestFields []string
}

const api = "/api/v1"

// Rules lists every audited route
var Rules = map[string]Rule{
	"POST " + api + "/projects":                                {Action: "project.create", TargetType: "project", FromResponse: true},
	"PATCH " + api + "/projects/{projectID}":                   {Action: "project.update", TargetType: "project", Kind: KindProject, Param: "projectID", RequestFields: []string{"name", "slug"}},
	"DELETE " + api + "/projects/{projectID}":                  {Action: "project.delete", TargetType: "project", Kind: KindProject, Param: "projectID"},
	"POST " + api + "/projects/{projectID}/members":            {Action: "member.add", TargetType: "member", Kind: KindProject, Param: "projectID", FromResponse: true, RequestFields: []string{"role"}},
	"DELETE " + api + "/projects/{projectID}/members/{userID}": {Action: "member.remove", TargetType: "member", Kind: KindProject, Param: "projectID", TargetIDParam: "userID"},

	"POST " + api + "/projects/{projectID}/servers/enrollment-tokens": {Action: "enrollment_token.create", TargetType: "server", Kind: KindProject, Param: "projectID", FromResponse: true},
	"DELETE " + api + "/servers/{serverID}":                           {Action: "server.delete", TargetType: "server", Kind: KindServer, Param: "serverID"},
	"POST " + api + "/servers/{serverID}/credential-rotation":         {Action: "server.credential_rotation_request", TargetType: "server", Kind: KindServer, Param: "serverID"},
	"POST " + api + "/servers/{serverID}/containers/{containerID}/actions": {
		Action: "container.action", TargetType: "container", Kind: KindServer, Param: "serverID", TargetIDParam: "containerID", RequestFields: []string{"action"},
	},
	"POST " + api + "/servers/{serverID}/containers/{containerID}/logs": {
		Action: "container.logs_fetch", TargetType: "container", Kind: KindServer, Param: "serverID", TargetIDParam: "containerID",
	},

	"POST " + api + "/projects/{projectID}/git-connections/github": {Action: "git_connection.install_start", TargetType: "git_connection", Kind: KindProject, Param: "projectID"},
	"POST " + api + "/projects/{projectID}/git-connections/gitlab": {Action: "git_connection.create", TargetType: "git_connection", Kind: KindProject, Param: "projectID", FromResponse: true},
	"DELETE " + api + "/git-connections/{connectionID}":            {Action: "git_connection.delete", TargetType: "git_connection", Kind: KindGitConnection, Param: "connectionID"},

	"POST " + api + "/projects/{projectID}/registries": {Action: "registry.create", TargetType: "registry", Kind: KindProject, Param: "projectID", FromResponse: true},
	"DELETE " + api + "/registries/{registryID}":       {Action: "registry.delete", TargetType: "registry", Kind: KindRegistry, Param: "registryID"},

	"PUT " + api + "/projects/{projectID}/secrets/{name}":    {Action: "secret.set", TargetType: "secret", Kind: KindProject, Param: "projectID", TargetNameParam: "name"},
	"DELETE " + api + "/projects/{projectID}/secrets/{name}": {Action: "secret.delete", TargetType: "secret", Kind: KindProject, Param: "projectID", TargetNameParam: "name"},

	"POST " + api + "/projects/{projectID}/applications": {Action: "application.create", TargetType: "application", Kind: KindProject, Param: "projectID", FromResponse: true},
	"PATCH " + api + "/applications/{applicationID}":     {Action: "application.update", TargetType: "application", Kind: KindApplication, Param: "applicationID", RequestFields: []string{"name", "branch", "dockerfile_path", "build_context"}},
	"DELETE " + api + "/applications/{applicationID}":    {Action: "application.delete", TargetType: "application", Kind: KindApplication, Param: "applicationID"},
	"POST " + api + "/applications/{applicationID}/builds": {
		Action: "build.trigger", TargetType: "application", Kind: KindApplication, Param: "applicationID", RequestFields: []string{"commit_sha"},
	},

	"POST " + api + "/applications/{applicationID}/environments": {Action: "environment.create", TargetType: "environment", Kind: KindApplication, Param: "applicationID", FromResponse: true},
	"PATCH " + api + "/environments/{environmentID}":             {Action: "environment.update", TargetType: "environment", Kind: KindEnvironment, Param: "environmentID", RequestFields: []string{"name", "auto_deploy"}},
	"DELETE " + api + "/environments/{environmentID}":            {Action: "environment.delete", TargetType: "environment", Kind: KindEnvironment, Param: "environmentID"},
	"POST " + api + "/environments/{environmentID}/deployments": {
		Action: "deployment.start", TargetType: "environment", Kind: KindEnvironment, Param: "environmentID", RequestFields: []string{"build_job_id"},
	},
	"POST " + api + "/releases/{releaseID}/rollback": {Action: "release.rollback", TargetType: "release", Kind: KindRelease, Param: "releaseID", TargetIDParam: "releaseID"},
	"POST " + api + "/jobs/{jobID}/cancel":           {Action: "job.cancel", TargetType: "job", Kind: KindJob, Param: "jobID", TargetIDParam: "jobID"},

	"POST " + api + "/agent/rotate-credential": {Action: "server.credential_rotate", TargetType: "server", Actor: ActorAgent, ResponseServer: true},

	"POST " + api + "/agent/enroll": {Action: "server.enroll", TargetType: "server", Actor: ActorAgent, ResponseServer: true},

	"POST " + api + "/auth/login": {Action: "user.login", TargetType: "user", FromResponse: true},

	"POST " + api + "/auth/setup": {Action: "user.setup", TargetType: "user", FromResponse: true},
}

// Exempt lists mutating routes that are deliberately not audited, with the reason
var Exempt = map[string]string{
	"POST " + api + "/auth/logout": "signing out is not a security-relevant change and the session is already gone",

	"POST " + api + "/agent/heartbeat":               "machine traffic every 20 seconds",
	"POST " + api + "/agent/containers/snapshot":     "machine traffic",
	"POST " + api + "/agent/jobs/claim":              "machine traffic",
	"POST " + api + "/agent/jobs/{jobID}/heartbeat":  "machine traffic",
	"POST " + api + "/agent/jobs/{jobID}/events":     "machine traffic",
	"POST " + api + "/agent/jobs/{jobID}/complete":   "machine traffic; the job it completes was audited when it was requested",
	"POST " + api + "/runner/heartbeat":              "machine traffic",
	"POST " + api + "/runner/jobs/claim":             "machine traffic",
	"POST " + api + "/runner/jobs/{jobID}/heartbeat": "machine traffic",
	"POST " + api + "/runner/jobs/{jobID}/events":    "machine traffic",
	"POST " + api + "/runner/jobs/{jobID}/complete":  "machine traffic; the build it completes was audited when it was queued",

	"POST " + api + "/webhooks/github":                "recorded by the webhook handler, which knows which applications build",
	"POST " + api + "/webhooks/gitlab/{connectionID}": "recorded by the webhook handler, which knows which applications build",
}
