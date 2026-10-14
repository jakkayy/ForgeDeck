// Command bench drives a running Control Plane with the benchmark profile in
// docs/12-measurements.md: concurrent dashboard users reading the pages a
// person opens, plus simulated Agents that heartbeat and long-poll for jobs.
// It reads through the public API only. Seed the data first with seed.sql.
//
//	go run ./scripts/bench -url http://127.0.0.1:18090
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"os"
	"sort"
	"sync"
	"sync/atomic"
	"time"
)

const csrfHeader = "X-Requested-With" // see internal/httpx/csrf.go

func main() {
	var (
		base        = flag.String("url", "http://127.0.0.1:8080", "Control Plane base URL")
		email       = flag.String("email", "bench@forgedeck.local", "owner of the seeded data")
		password    = flag.String("password", "bench-password-1234", "password of the owner")
		users       = flag.Int("users", 20, "concurrent dashboard users")
		duration    = flag.Duration("duration", 10*time.Second, "how long each endpoint is driven")
		agents      = flag.Int("agents", 50, "simulated Agents (credentials fda_bench_001…)")
		agentsOnly  = flag.Bool("agents-only", false, "only run the Agents until interrupted")
		mixDuration = flag.Duration("mix", 60*time.Second, "length of the mixed run (0 skips it)")
	)
	flag.Parse()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()

	var agentWG sync.WaitGroup
	var agentErrors, heartbeats, claims atomic.Int64
	for i := 1; i <= *agents; i++ {
		agentWG.Add(1)
		go func(n int) {
			defer agentWG.Done()
			runAgent(ctx, *base, fmt.Sprintf("fda_bench_%03d", n), &heartbeats, &claims, &agentErrors)
		}(i)
	}
	if *agentsOnly {
		fmt.Printf("%d Agents running; interrupt to stop\n", *agents)
		select {}
	}
	time.Sleep(2 * time.Second) // let the Agents settle into their long-polls

	client := newClient(*base, *email, *password)
	ids := discover(client, *base)

	endpoints := []endpoint{
		{"GET /auth/me", "/auth/me"},
		{"GET /overview", "/overview"},
		{"GET /servers (page 1)", "/servers?limit=25"},
		{"GET /servers?q=…&status=online", "/servers?limit=25&q=server-0&status=online"},
		{"GET /projects (page 1)", "/projects?limit=25"},
		{"GET /projects/{id}/applications", "/projects/" + ids.project + "/applications?limit=25"},
		{"GET /projects/{id}/applications?q=…", "/projects/" + ids.project + "/applications?limit=25&q=app-00"},
		{"GET /jobs (page 1)", "/jobs?limit=25"},
		{"GET /jobs?status=failed", "/jobs?limit=25&status=failed"},
		{"GET /jobs?application_id=…", "/jobs?limit=25&application_id=" + ids.application + "&type=build"},
		{"GET /jobs/{id}/logs (100 lines)", "/jobs/" + ids.jobWithLogs + "/logs?limit=100"},
		{"GET /environments/{id}/deployments", "/environments/" + ids.environment + "/deployments?limit=25"},
		{"GET /environments/{id}/releases", "/environments/" + ids.environment + "/releases?limit=25"},
		{"GET /projects/{id}/audit-logs", "/projects/" + ids.project + "/audit-logs?limit=50"},
		{"GET /servers/{id}", "/servers/" + ids.server},
	}

	fmt.Printf("%d concurrent users, %s per endpoint, %d simulated Agents\n\n", *users, *duration, *agents)
	fmt.Printf("%-40s %8s %6s %8s %8s %8s %8s\n", "endpoint", "req/s", "errs", "p50 ms", "p95 ms", "p99 ms", "max ms")
	for _, e := range endpoints {
		report(e.name, drive(client, *base, *users, *duration, []string{e.path}))
	}
	if *mixDuration > 0 {
		paths := make([]string, len(endpoints))
		for i, e := range endpoints {
			paths[i] = e.path
		}
		fmt.Printf("\nmixed: every user cycles through all %d endpoints for %s\n", len(paths), *mixDuration)
		report("mixed", drive(client, *base, *users, *mixDuration, paths))
	}
	fmt.Printf("\nAgents during the run: %d heartbeats, %d empty claims returned, %d errors\n",
		heartbeats.Load(), claims.Load(), agentErrors.Load())
	stop()
	agentWG.Wait()
}

type endpoint struct{ name, path string }

type result struct {
	latencies []time.Duration
	errors    int
	elapsed   time.Duration
}

func newClient(base, email, password string) *http.Client {
	jar, _ := cookiejar.New(nil)
	c := &http.Client{Jar: jar, Timeout: 30 * time.Second, Transport: &http.Transport{MaxIdleConnsPerHost: 256}}
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	req, _ := http.NewRequestWithContext(context.Background(), http.MethodPost, base+"/api/v1/auth/login", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set(csrfHeader, "bench")
	resp, err := c.Do(req)
	if err != nil || resp.StatusCode != http.StatusOK {
		fmt.Fprintf(os.Stderr, "login failed: %v %v\n", err, resp)
		os.Exit(1)
	}
	_ = resp.Body.Close()
	return c
}

func get(c *http.Client, base, path string, into any) (int, error) {
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, base+"/api/v1"+path, nil)
	if err != nil {
		return 0, err
	}
	resp, err := c.Do(req)
	if err != nil {
		return 0, err
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return resp.StatusCode, err
	}
	if into != nil && resp.StatusCode == http.StatusOK {
		_ = json.Unmarshal(data, into)
	}
	return resp.StatusCode, nil
}

type idList struct {
	Items []struct {
		ID string `json:"id"`
	} `json:"items"`
}

// firstID is the ID of the first item; the benchmark cannot run without one.
func firstID(l idList, what string) string {
	if len(l.Items) == 0 {
		fmt.Fprintf(os.Stderr, "no %s: seed the database first\n", what)
		os.Exit(1)
	}
	return l.Items[0].ID
}

type seededIDs struct{ project, application, environment, server, jobWithLogs string }

// discover finds one of each kind of thing the benchmark reads by ID.
func discover(c *http.Client, base string) seededIDs {
	var ids seededIDs
	var l idList
	mustGet(c, base, "/projects?limit=1", &l)
	ids.project = firstID(l, "projects")
	l = idList{}
	mustGet(c, base, "/projects/"+ids.project+"/applications?limit=1", &l)
	ids.application = firstID(l, "applications")
	l = idList{}
	mustGet(c, base, "/applications/"+ids.application+"/environments", &l)
	ids.environment = firstID(l, "environments")
	l = idList{}
	mustGet(c, base, "/servers?limit=1", &l)
	ids.server = firstID(l, "servers")
	l = idList{}
	mustGet(c, base, "/jobs?limit=1&type=build&status=succeeded", &l) // the newest jobs carry the log lines
	ids.jobWithLogs = firstID(l, "jobs")
	return ids
}

func mustGet(c *http.Client, base, path string, into any) {
	if status, err := get(c, base, path, into); err != nil || status != http.StatusOK {
		fmt.Fprintf(os.Stderr, "GET %s: %d %v\n", path, status, err)
		os.Exit(1)
	}
}

// drive runs users concurrent requesters on one signed-in client (one session:
// login is rate limited, and the per-request cost is the same), each requesting
// paths in turn as fast as the server answers, for d.
func drive(c *http.Client, base string, users int, d time.Duration, paths []string) result {
	var mu sync.Mutex
	var res result
	var wg sync.WaitGroup
	deadline := time.Now().Add(d)
	start := time.Now()
	for u := 0; u < users; u++ {
		wg.Add(1)
		go func(offset int) {
			defer wg.Done()
			for i := offset; time.Now().Before(deadline); i++ {
				path := paths[i%len(paths)]
				t0 := time.Now()
				status, err := get(c, base, path, nil)
				took := time.Since(t0)
				mu.Lock()
				res.latencies = append(res.latencies, took)
				if err != nil || status != http.StatusOK {
					res.errors++
				}
				mu.Unlock()
			}
		}(u)
	}
	wg.Wait()
	res.elapsed = time.Since(start)
	return res
}

func report(name string, r result) {
	sort.Slice(r.latencies, func(i, j int) bool { return r.latencies[i] < r.latencies[j] })
	pct := func(p float64) float64 {
		if len(r.latencies) == 0 {
			return 0
		}
		idx := min(int(float64(len(r.latencies))*p), len(r.latencies)-1)
		return float64(r.latencies[idx].Microseconds()) / 1000
	}
	fmt.Printf("%-40s %8.0f %6d %8.1f %8.1f %8.1f %8.1f\n", name,
		float64(len(r.latencies))/r.elapsed.Seconds(), r.errors, pct(0.50), pct(0.95), pct(0.99), pct(1))
}

// runAgent behaves like a real Agent: a heartbeat every 20 seconds and a
// 25-second long-poll for jobs, with the credential in the Authorization header.
func runAgent(ctx context.Context, base, credential string, heartbeats, claims, errs *atomic.Int64) {
	c := &http.Client{Timeout: 40 * time.Second}
	post := func(path string, body any) (int, error) {
		data, _ := json.Marshal(body)
		req, _ := http.NewRequestWithContext(ctx, http.MethodPost, base+"/api/v1"+path, bytes.NewReader(data))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+credential)
		resp, err := c.Do(req)
		if err != nil {
			return 0, err
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
		return resp.StatusCode, nil
	}
	heartbeat := func() {
		status, err := post("/agent/heartbeat", map[string]any{
			"agent_version": "0.1.0", "protocol_version": 1, "docker": map[string]any{"available": true, "version": "27.3.1"},
			"resources": map[string]any{"cpu_percent": 10, "memory_percent": 20, "disk_percent": 30},
		})
		if ctx.Err() != nil {
			return
		}
		if err != nil || status != http.StatusOK {
			errs.Add(1)
			return
		}
		heartbeats.Add(1)
	}
	go func() {
		t := time.NewTicker(20 * time.Second)
		defer t.Stop()
		heartbeat()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				heartbeat()
			}
		}
	}()
	for ctx.Err() == nil {
		status, err := post("/agent/jobs/claim", map[string]any{"wait_seconds": 25})
		switch {
		case ctx.Err() != nil:
			return
		case err != nil || (status != http.StatusNoContent && status != http.StatusOK):
			errs.Add(1)
			time.Sleep(time.Second)
		default:
			claims.Add(1)
		}
	}
}
