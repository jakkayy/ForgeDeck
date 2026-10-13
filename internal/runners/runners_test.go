package runners_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/runners"
	"github.com/jakkayy/ForgeDeck/internal/store"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

func TestRunnerLifecycle(t *testing.T) {
	svc := runners.NewService(store.New(testdb.New(t)), time.Now)
	ctx := context.Background()

	runner, credential, err := svc.Create(ctx, "build-01")
	if err != nil || !strings.HasPrefix(credential, runners.CredentialPrefix) {
		t.Fatalf("Create: %v %q", err, credential)
	}
	if _, _, err := svc.Create(ctx, "build-01"); err == nil {
		t.Fatal("duplicate name accepted")
	}
	if _, _, err := svc.Create(ctx, "Build 01"); err == nil {
		t.Fatal("invalid name accepted")
	}

	got, err := svc.Authenticate(ctx, credential)
	if err != nil || got.ID != runner.ID {
		t.Fatalf("Authenticate: %+v %v", got, err)
	}
	if err := svc.RecordHeartbeat(ctx, runner.ID, "0.3.0"); err != nil {
		t.Fatal(err)
	}

	if err := svc.Delete(ctx, "build-01"); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Authenticate(ctx, credential); !errors.Is(err, runners.ErrInvalidCredential) {
		t.Fatalf("after delete: %v", err)
	}
	if err := svc.Delete(ctx, "build-01"); !errors.Is(err, runners.ErrNotFound) {
		t.Fatalf("delete twice: %v", err)
	}
}
