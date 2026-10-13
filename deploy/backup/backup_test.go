// Package backup_test runs the backup and restore scripts against a real
// PostgreSQL in Docker. It runs only when FORGEDECK_TEST_DOCKER=1.
package backup_test

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/dockertest"
)

// seeded starts PostgreSQL, applies ForgeDeck's migrations, and adds the data
// whose survival the tests check.
func seeded(t *testing.T) dockertest.Postgres {
	t.Helper()
	dockertest.Require(t)
	pg := dockertest.StartPostgres(t)

	migrator, err := database.NewMigrator(pg.URL, slog.New(slog.DiscardHandler))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = migrator.Close() }()
	if err := migrator.Up(context.Background()); err != nil {
		t.Fatal(err)
	}

	conn, err := pgx.Connect(context.Background(), pg.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(context.Background()) }()
	for _, statement := range []string{
		`INSERT INTO users (email, password_hash) VALUES ('owner@example.com', 'x'), ('dev@example.com', 'x')`,
		`INSERT INTO projects (name, slug) VALUES ('Shop', 'shop')`,
		`INSERT INTO project_secrets (project_id, name, ciphertext, key_id) SELECT id, 'API_KEY', '\xdeadbeef', 'v1' FROM projects`,
		`INSERT INTO audit_logs (actor, action, target_type) VALUES ('owner@example.com', 'project.create', 'project')`,
	} {
		if _, err := conn.Exec(context.Background(), statement); err != nil {
			t.Fatalf("%v\n%s", err, statement)
		}
	}
	return pg
}

// script runs one of the scripts with the given environment and returns its output.
func script(t *testing.T, env []string, name string, args ...string) (string, error) {
	t.Helper()
	cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // test runs the repository's own scripts
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func backupFile(t *testing.T, dir string) string {
	t.Helper()
	files, _ := filepath.Glob(filepath.Join(dir, "forgedeck-*.dump"))
	if len(files) != 1 {
		t.Fatalf("backups in %s: %v", dir, files)
	}
	return files[0]
}

func TestBackupAndRestoreThroughTheContainer(t *testing.T) {
	pg := seeded(t)
	env := []string{"FORGEDECK_PG_CONTAINER=" + pg.Container, "POSTGRES_USER=forgedeck", "POSTGRES_DB=forgedeck"}
	dir := t.TempDir()

	out, err := script(t, env, "./backup.sh", "-d", dir)
	if err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}
	if !strings.Contains(out, "FORGEDECK_SECRET_KEYS") {
		t.Errorf("the backup does not remind about the key:\n%s", out)
	}
	file := backupFile(t, dir)
	if info, err := os.Stat(file); err != nil || info.Mode().Perm() != 0o600 || info.Size() == 0 {
		t.Fatalf("backup file: %v, %v", info, err)
	}
	if _, err := os.Stat(file + ".sha256"); err != nil {
		t.Fatalf("no checksum: %v", err)
	}

	// --verify restores into a temporary database, reports what it found, and leaves nothing behind.
	out, err = script(t, env, "./restore.sh", "--verify", file)
	if err != nil {
		t.Fatalf("verify: %v\n%s", err, out)
	}
	for _, want := range []string{"users          2", "projects       1", "audit_logs     1", "OK: the backup restores cleanly"} {
		if !strings.Contains(out, want) {
			t.Errorf("verify output lacks %q:\n%s", want, out)
		}
	}
	if leftover := dockerSQL(t, pg, `SELECT count(*) FROM pg_database WHERE datname LIKE 'forgedeck_verify_%'`); leftover != "0" {
		t.Errorf("%s temporary databases left behind", leftover)
	}

	// A real restore replaces the database: lose the data, restore, and it is back.
	dockerSQL(t, pg, `TRUNCATE users, projects CASCADE`)
	if got := dockerSQL(t, pg, `SELECT count(*) FROM projects`); got != "0" {
		t.Fatalf("setup: %s projects", got)
	}
	if out, err := script(t, env, "./restore.sh", file); err == nil {
		t.Fatalf("restore without --yes did something:\n%s", out)
	}
	if got := dockerSQL(t, pg, `SELECT count(*) FROM projects`); got != "0" {
		t.Fatalf("a restore without --yes changed the database: %s projects", got)
	}
	if out, err := script(t, env, "./restore.sh", "--yes", file); err != nil {
		t.Fatalf("restore: %v\n%s", err, out)
	}
	if got := dockerSQL(t, pg, `SELECT count(*) FROM projects`); got != "1" {
		t.Fatalf("after the restore: %s projects, want 1", got)
	}
	// The secret is still ciphertext: the backup never held the key.
	if got := dockerSQL(t, pg, `SELECT encode(ciphertext, 'hex') FROM project_secrets`); got != "deadbeef" {
		t.Fatalf("restored ciphertext %q", got)
	}
}

func TestBackupAndRestoreWithAConnectionURL(t *testing.T) {
	// pg_dump refuses a server newer than itself, so the test needs a client of version 17 or later.
	version, err := exec.CommandContext(t.Context(), "pg_dump", "--version").Output()
	if err != nil {
		t.Skip("pg_dump is not installed on this machine")
	}
	if fields := strings.Fields(string(version)); len(fields) < 3 || major(fields[2]) < 17 {
		t.Skipf("pg_dump %q is older than the test server (PostgreSQL 17)", strings.TrimSpace(string(version)))
	}
	pg := seeded(t)
	env := []string{"FORGEDECK_DATABASE_URL=" + pg.URL}
	dir := t.TempDir()

	if out, err := script(t, env, "./backup.sh", "-d", dir); err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}
	if out, err := script(t, env, "./restore.sh", "--verify", backupFile(t, dir)); err != nil || !strings.Contains(out, "users          2") {
		t.Fatalf("verify: %v\n%s", err, out)
	}
}

func TestDamagedBackupsAreRefused(t *testing.T) {
	pg := seeded(t)
	env := []string{"FORGEDECK_PG_CONTAINER=" + pg.Container}
	dir := t.TempDir()
	if out, err := script(t, env, "./backup.sh", "-d", dir); err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}
	file := backupFile(t, dir)
	data, err := os.ReadFile(file)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, data[:len(data)/2], 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := script(t, env, "./restore.sh", "--verify", file)
	if err == nil || !strings.Contains(out, "checksum mismatch") {
		t.Fatalf("a truncated backup was accepted: %v\n%s", err, out)
	}

	// Without the checksum file the damage is still caught, by PostgreSQL, and nothing is left behind.
	if err := os.Remove(file + ".sha256"); err != nil {
		t.Fatal(err)
	}
	out, err = script(t, env, "./restore.sh", "--verify", file)
	if err == nil {
		t.Fatalf("a truncated backup restored:\n%s", out)
	}
	if leftover := dockerSQL(t, pg, `SELECT count(*) FROM pg_database WHERE datname LIKE 'forgedeck_verify_%'`); leftover != "0" {
		t.Errorf("%s temporary databases left behind after a failed verify", leftover)
	}
}

func TestOldBackupsArePruned(t *testing.T) {
	pg := seeded(t)
	env := []string{"FORGEDECK_PG_CONTAINER=" + pg.Container}
	dir := t.TempDir()
	for _, name := range []string{"forgedeck-20200101T000000Z.dump", "forgedeck-20200102T000000Z.dump", "forgedeck-20200103T000000Z.dump"} {
		for _, file := range []string{name, name + ".sha256"} {
			if err := os.WriteFile(filepath.Join(dir, file), []byte("old"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	if out, err := script(t, env, "./backup.sh", "-d", dir, "-k", "2"); err != nil {
		t.Fatalf("backup: %v\n%s", err, out)
	}
	dumps, _ := filepath.Glob(filepath.Join(dir, "forgedeck-*.dump"))
	sums, _ := filepath.Glob(filepath.Join(dir, "forgedeck-*.dump.sha256"))
	if len(dumps) != 2 || len(sums) != 2 {
		t.Fatalf("kept %d dumps and %d checksums, want 2 and 2: %v", len(dumps), len(sums), dumps)
	}
	if _, err := os.Stat(filepath.Join(dir, "forgedeck-20200101T000000Z.dump")); err == nil {
		t.Error("the oldest backup was kept")
	}
	if _, err := script(t, env, "./backup.sh", "-d", dir, "-k", "abc"); err == nil {
		t.Error("a non-numeric -k was accepted")
	}
}

func TestScriptsRefuseToGuessTheDatabase(t *testing.T) {
	for _, name := range []string{"./backup.sh", "./restore.sh"} {
		args := []string{}
		if name == "./restore.sh" {
			args = []string{"--verify", os.Args[0]}
		}
		cmd := exec.CommandContext(t.Context(), name, args...) //nolint:gosec // test runs the repository's own scripts
		cmd.Env = []string{"PATH=" + os.Getenv("PATH")}
		out, err := cmd.CombinedOutput()
		if err == nil || !strings.Contains(string(out), "FORGEDECK_PG_CONTAINER") {
			t.Errorf("%s with no target: %v\n%s", name, err, out)
		}
	}
}

func dockerSQL(t *testing.T, pg dockertest.Postgres, statement string) string {
	t.Helper()
	out, err := exec.CommandContext(t.Context(), "docker", "exec", pg.Container, "psql", "-U", "forgedeck", "-d", "forgedeck", "-tAc", statement).CombinedOutput() //nolint:gosec // test
	if err != nil {
		t.Fatalf("%s: %v\n%s", statement, err, out)
	}
	return strings.TrimSpace(string(out))
}

// major returns the major version number of "17.2" or "16.4.1".
func major(version string) int {
	n := 0
	for _, r := range version {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	return n
}
