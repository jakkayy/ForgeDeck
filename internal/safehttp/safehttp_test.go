package safehttp

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestPolicyCheck(t *testing.T) {
	cases := []struct {
		address string
		policy  Policy
		allowed bool
	}{
		{"140.82.112.3:443", Policy{}, true},
		{"[2606:4700::1111]:443", Policy{}, true},
		{"127.0.0.1:80", Policy{}, false},
		{"[::1]:80", Policy{}, false},
		{"127.0.0.1:80", Policy{AllowLoopback: true}, true},
		{"169.254.169.254:80", Policy{AllowPrivate: true, AllowLoopback: true}, false},
		{"10.0.0.5:443", Policy{}, false},
		{"192.168.1.10:443", Policy{AllowPrivate: true}, true},
		{"[fd00::1]:443", Policy{AllowPrivate: true}, true},
		{"0.0.0.0:80", Policy{AllowPrivate: true}, false},
		{"[::ffff:127.0.0.1]:80", Policy{}, false},
	}
	for _, tc := range cases {
		err := tc.policy.check(tc.address)
		if (err == nil) != tc.allowed {
			t.Errorf("%s with %+v: err=%v, want allowed=%v", tc.address, tc.policy, err, tc.allowed)
		}
	}
}

func TestClientBlocksAndDoesNotFollowRedirects(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "http://169.254.169.254/latest/meta-data/", http.StatusFound)
	}))
	defer server.Close()
	ctx := context.Background()

	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, server.URL, nil)
	blocked, err := NewClient(Policy{}, time.Second).Do(req)
	if err == nil {
		_ = blocked.Body.Close()
	}
	if !errors.Is(err, ErrBlockedDestination) {
		t.Fatalf("loopback request: got %v, want ErrBlockedDestination", err)
	}

	resp, err := NewClient(Policy{AllowLoopback: true}, time.Second).Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusFound {
		t.Fatalf("redirect was followed: status %d", resp.StatusCode)
	}
}
