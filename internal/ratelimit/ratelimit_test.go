package ratelimit

import (
	"testing"
	"time"
)

type clock struct{ t time.Time }

func (c *clock) now() time.Time { return c.t }

func TestLimiterAllowsBurstThenRefills(t *testing.T) {
	c := &clock{t: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	l := New(10*time.Second, 3, c.now)

	for i := range 3 {
		if ok, _ := l.Allow("ip"); !ok {
			t.Fatalf("event %d rejected within burst", i)
		}
	}
	ok, retry := l.Allow("ip")
	if ok || retry <= 0 || retry > 10*time.Second {
		t.Fatalf("over burst: ok=%v retry=%v", ok, retry)
	}

	c.t = c.t.Add(10 * time.Second)
	if ok, _ := l.Allow("ip"); !ok {
		t.Fatal("token not refilled")
	}
}

func TestLimiterKeysAreIndependent(t *testing.T) {
	c := &clock{t: time.Now()}
	l := New(time.Minute, 1, c.now)

	if ok, _ := l.Allow("a"); !ok {
		t.Fatal("a rejected")
	}
	if ok, _ := l.Allow("b"); !ok {
		t.Fatal("b rejected because of a")
	}
}

func TestLimiterEvictsIdleKeys(t *testing.T) {
	c := &clock{t: time.Now()}
	l := New(time.Minute, 1, c.now)
	l.Allow("old")

	c.t = c.t.Add(idleEviction + 2*time.Minute)
	l.Allow("new")

	if _, exists := l.buckets["old"]; exists {
		t.Fatal("idle key not evicted")
	}
}

func TestBlockedAndFailCountOnlyFailures(t *testing.T) {
	now := time.Unix(1_700_000_000, 0)
	l := New(10*time.Second, 3, func() time.Time { return now })

	// Successful work consumes nothing: checking never uses a token.
	for range 10 {
		if blocked, _ := l.Blocked("1.2.3.4"); blocked {
			t.Fatal("blocked before any failure")
		}
	}
	for range 3 {
		l.Fail("1.2.3.4")
	}
	blocked, wait := l.Blocked("1.2.3.4")
	if !blocked || wait <= 0 || wait > 10*time.Second {
		t.Fatalf("after 3 failures: blocked=%v wait=%v", blocked, wait)
	}
	if blocked, _ := l.Blocked("5.6.7.8"); blocked {
		t.Fatal("another key was blocked")
	}

	// Further failures do not extend the block beyond what the bucket holds.
	l.Fail("1.2.3.4")
	l.Fail("1.2.3.4")
	now = now.Add(10 * time.Second)
	if blocked, _ := l.Blocked("1.2.3.4"); blocked {
		t.Fatal("still blocked one refill interval after the last failure")
	}
}
