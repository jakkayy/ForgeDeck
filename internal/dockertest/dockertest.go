// Package dockertest helps tests that need a real Docker daemon. They run
// only when FORGEDECK_TEST_DOCKER=1.
package dockertest

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"testing"
	"time"
)

// Require skips the test unless Docker tests are enabled.
func Require(t testing.TB) {
	t.Helper()
	if os.Getenv("FORGEDECK_TEST_DOCKER") != "1" {
		t.Skip("set FORGEDECK_TEST_DOCKER=1 to run against a Docker daemon")
	}
}

// RemoveImage deletes an image when the test ends, ignoring errors.
func RemoveImage(t testing.TB, ref string) {
	t.Helper()
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "docker", "image", "rm", "--force", ref).Run()
	})
}

// StartRegistry runs a throwaway registry on a free local port and returns
// its host:port. Docker allows plain HTTP to localhost registries.
func StartRegistry(t testing.TB) string {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	name := fmt.Sprintf("forgedeck-test-registry-%d", port)
	//nolint:gosec // test helper; the arguments are fixed apart from a local port
	out, err := exec.CommandContext(context.Background(), "docker", "run", "--detach", "--rm", "--name", name,
		"--publish", fmt.Sprintf("127.0.0.1:%d:5000", port), "registry:3").CombinedOutput()
	if err != nil {
		t.Fatalf("start registry: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "docker", "rm", "--force", name).Run() //nolint:gosec // see above
	})

	deadline := time.Now().Add(15 * time.Second)
	for {
		if conn, err := (&net.Dialer{}).DialContext(context.Background(), "tcp", fmt.Sprintf("127.0.0.1:%d", port)); err == nil {
			_ = conn.Close()
			return fmt.Sprintf("localhost:%d", port)
		}
		if time.Now().After(deadline) {
			t.Fatal("registry did not start")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// Postgres is a throwaway PostgreSQL server in a container.
type Postgres struct {
	// Container is the container's name, for `docker exec`.
	Container string
	// URL reaches the database "forgedeck" from this machine.
	URL string
}

// StartPostgres runs PostgreSQL 17 in a container with a database named
// "forgedeck" and returns once it accepts connections.
func StartPostgres(t testing.TB) Postgres {
	t.Helper()
	listener, err := (&net.ListenConfig{}).Listen(context.Background(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	_ = listener.Close()

	name := fmt.Sprintf("forgedeck-test-postgres-%d", port)
	//nolint:gosec // test helper; the arguments are fixed apart from a local port
	out, err := exec.CommandContext(context.Background(), "docker", "run", "--detach", "--rm", "--name", name,
		"--env", "POSTGRES_USER=forgedeck", "--env", "POSTGRES_PASSWORD=backup-test", "--env", "POSTGRES_DB=forgedeck",
		"--publish", fmt.Sprintf("127.0.0.1:%d:5432", port), "postgres:17-alpine").CombinedOutput()
	if err != nil {
		t.Fatalf("start postgres: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.CommandContext(context.Background(), "docker", "rm", "--force", name).Run() //nolint:gosec // see above
	})

	deadline := time.Now().Add(60 * time.Second)
	for {
		ready := exec.CommandContext(context.Background(), "docker", "exec", name, "pg_isready", "-U", "forgedeck", "-d", "forgedeck").Run() //nolint:gosec // see above
		if ready == nil {
			// pg_isready answers during the image's own bootstrap restart; a real query does not.
			if exec.CommandContext(context.Background(), "docker", "exec", name, "psql", "-U", "forgedeck", "-d", "forgedeck", "-tAc", "SELECT 1").Run() == nil { //nolint:gosec // see above
				break
			}
		}
		if time.Now().After(deadline) {
			t.Fatal("postgres did not start")
		}
		time.Sleep(300 * time.Millisecond)
	}
	return Postgres{Container: name, URL: fmt.Sprintf("postgres://forgedeck:backup-test@127.0.0.1:%d/forgedeck?sslmode=disable", port)}
}
