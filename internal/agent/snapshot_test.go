package agent_test

import (
	"context"
	"log/slog"
	"net/http"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/agent"
	"github.com/jakkayy/ForgeDeck/internal/agentproto"
	"github.com/jakkayy/ForgeDeck/internal/servers"
)

type fakeEngine struct {
	containers chan []agentproto.Container
	current    []agentproto.Container
}

func (f *fakeEngine) ListContainers(context.Context) ([]agentproto.Container, error) {
	select {
	case next := <-f.containers:
		f.current = next
	default:
	}
	return f.current, nil
}

func TestSnapshotReporterSendsOnStartAndTrigger(t *testing.T) {
	_, owner, enrolled, client := enrolledClient(t)
	engine := &fakeEngine{containers: make(chan []agentproto.Container, 1), current: []agentproto.Container{
		{ID: "aaaaaaaaaaaa1111", Name: "api", Image: "api:1", State: "running"},
	}}
	reporter := agent.NewSnapshotReporter(client, engine, slog.New(slog.DiscardHandler))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = reporter.Run(ctx) }()

	path := "/api/v1/servers/" + enrolled.ServerID.String() + "/containers"
	waitForContainers := func(want string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			var list servers.ContainerList
			owner.Do(http.MethodGet, path, nil).RequireStatus(http.StatusOK).Decode(&list)
			if len(list.Containers) == 1 && list.Containers[0].Name == want {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("container %q never reported: %+v", want, list)
			}
			time.Sleep(50 * time.Millisecond)
		}
	}

	waitForContainers("api")
	engine.containers <- []agentproto.Container{{ID: "bbbbbbbbbbbb2222", Name: "worker", Image: "w:1", State: "running"}}
	reporter.Trigger()
	waitForContainers("worker")
}
