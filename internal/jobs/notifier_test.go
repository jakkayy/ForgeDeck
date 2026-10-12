package jobs_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"github.com/jakkayy/ForgeDeck/internal/jobs"
)

func TestNotifierWakesOnlyMatchingWaiters(t *testing.T) {
	f := newFixture(t)
	other := f.addServer(t, "prod-02")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	notifier := jobs.NewNotifier(f.pool, slog.New(slog.DiscardHandler))
	target, unsubscribeTarget := notifier.Subscribe(f.serverID.String())
	defer unsubscribeTarget()
	bystander, unsubscribeBystander := notifier.Subscribe(other.String())
	defer unsubscribeBystander()

	go notifier.Run(ctx)
	// Run wakes everyone once after LISTEN succeeds; drain that first.
	waitFor(t, target, "initial wake-up")
	waitFor(t, bystander, "initial wake-up")

	if _, err := f.svc.Enqueue(ctx, f.agentJob(f.serverID)); err != nil {
		t.Fatal(err)
	}
	waitFor(t, target, "job notification")

	select {
	case <-bystander:
		t.Fatal("waiter for another server was woken")
	case <-time.After(200 * time.Millisecond):
	}
}

func waitFor(t *testing.T, ch <-chan struct{}, what string) {
	t.Helper()
	select {
	case <-ch:
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for %s", what)
	}
}
