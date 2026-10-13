package main

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func healthServer(t *testing.T, status int) string {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(status)
	}))
	t.Cleanup(server.Close)
	return strings.TrimPrefix(server.URL, "http://")
}

func TestHealthcheckPassesForAHealthyServer(t *testing.T) {
	t.Setenv("FORGEDECK_HTTP_ADDR", healthServer(t, http.StatusOK))
	if err := healthcheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestHealthcheckFailsForAnUnhealthyOrAbsentServer(t *testing.T) {
	t.Setenv("FORGEDECK_HTTP_ADDR", healthServer(t, http.StatusServiceUnavailable))
	if err := healthcheck(context.Background()); err == nil {
		t.Fatal("a 503 passed")
	}

	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := listener.Addr().String()
	_ = listener.Close() // nothing listens here now
	t.Setenv("FORGEDECK_HTTP_ADDR", addr)
	if err := healthcheck(context.Background()); err == nil {
		t.Fatal("a closed port passed")
	}
}

func TestHealthcheckAcceptsAWildcardListenAddress(t *testing.T) {
	// The server listens on ":port"; the check must still find it on this machine.
	addr := healthServer(t, http.StatusOK)
	_, port, _ := net.SplitHostPort(addr)
	t.Setenv("FORGEDECK_HTTP_ADDR", ":"+port)
	if err := healthcheck(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FORGEDECK_HTTP_ADDR", "0.0.0.0:"+port)
	if err := healthcheck(context.Background()); err != nil {
		t.Fatal(err)
	}
}
