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
