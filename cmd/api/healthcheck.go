package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

const healthcheckTimeout = 3 * time.Second

// healthcheck asks the running server on this machine whether it is healthy,
// for container health checks: the production image has no shell, curl or wget.
// It needs no configuration beyond FORGEDECK_HTTP_ADDR, so it works even when
// the rest of the configuration is wrong.
func healthcheck(ctx context.Context) error {
	addr := os.Getenv("FORGEDECK_HTTP_ADDR")
	if addr == "" {
		addr = ":8080"
	}
	host, port, err := net.SplitHostPort(addr)
	if err != nil {
		return fmt.Errorf("FORGEDECK_HTTP_ADDR %q: %w", addr, err)
	}
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}

	ctx, cancel := context.WithTimeout(ctx, healthcheckTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+net.JoinHostPort(host, port)+"/healthz", nil)
	if err != nil {
		return err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("health check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("health check: %s", strings.TrimSpace(resp.Status))
	}
	return nil
}
