package servers_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/apitest"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

const apiContainer = "aaaaaaaaaaaa1111"

func TestContainerActionQueuesAgentJob(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	dev, devID := env.NewUser("dev@example.com")
	projectID := owner.CreateProject("Infra")
	env.AddMember(projectID, devID, "developer")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	agent.ReportContainers(t, apiContainer)

	var queued servers.QueuedJob
	dev.Do(http.MethodPost, "/api/v1/servers/"+agent.ServerID.String()+"/containers/"+apiContainer+"/actions",
		map[string]string{"action": "restart"}).RequireStatus(http.StatusAccepted).Decode(&queued)
	if queued.Status != "queued" {
		t.Fatalf("queued %+v", queued)
	}

	wait := 0
	var job agentproto.Job
	agent.Do(http.MethodPost, "/api/v1/agent/jobs/claim", agentproto.ClaimRequest{WaitSeconds: &wait}).
		RequireStatus(http.StatusOK).Decode(&job)
	var payload agentproto.ContainerActionPayload
	if err := json.Unmarshal(job.Payload, &payload); err != nil {
		t.Fatal(err)
	}
	if job.ID != queued.JobID || job.Type != agentproto.JobContainerAction ||
		payload.ContainerID != apiContainer || payload.Action != "restart" {
		t.Fatalf("job %+v payload %+v", job, payload)
	}
}

func TestContainerActionRules(t *testing.T) {
	env := apitest.New(t)
	owner, _ := env.NewUser("owner@example.com")
	viewer, viewerID := env.NewUser("viewer@example.com")
	outsider, _ := env.NewUser("outsider@example.com")
	projectID := owner.CreateProject("Infra")
	env.AddMember(projectID, viewerID, "viewer")
	agent := env.EnrollAgent(owner, projectID, "prod-01")
	agent.ReportContainers(t, apiContainer)
	base := "/api/v1/servers/" + agent.ServerID.String() + "/containers/"
	restart := map[string]string{"action": "restart"}

	viewer.Do(http.MethodPost, base+apiContainer+"/actions", restart).RequireStatus(http.StatusForbidden)
	outsider.Do(http.MethodPost, base+apiContainer+"/actions", restart).RequireStatus(http.StatusNotFound)
	owner.Do(http.MethodPost, base+"ffffffffffff9999/actions", restart).RequireStatus(http.StatusNotFound)
	owner.Do(http.MethodPost, base+apiContainer+"/actions", map[string]string{"action": "rm -rf"}).
		RequireStatus(http.StatusUnprocessableEntity)
}
