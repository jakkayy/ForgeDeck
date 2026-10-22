package database_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

// TestMigrationsRoundTrip proves each Down reverses its Up.
func TestMigrationsRoundTrip(t *testing.T) {
	dbURL := testdb.Create(t)
	migrator, err := database.NewMigrator(dbURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	ctx := context.Background()

	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("first Up: %v", err)
	}
	if err := migrator.DownAll(ctx); err != nil {
		t.Fatalf("DownAll: %v", err)
	}
	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("second Up: %v", err)
	}
}

// TestDataSurvivesUpgradingFromAnOlderSchema keeps data when a new version migrates on start.
func TestDataSurvivesUpgradingFromAnOlderSchema(t *testing.T) {
	dbURL := testdb.Create(t)
	migrator, err := database.NewMigrator(dbURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("NewMigrator: %v", err)
	}
	t.Cleanup(func() { _ = migrator.Close() })
	ctx := context.Background()

	// schema at version 14, before registries, environments, deployments and secrets.
	const olderVersion = 14
	if err := migrator.UpTo(ctx, olderVersion); err != nil {
		t.Fatalf("UpTo(%d): %v", olderVersion, err)
	}
	conn, err := pgx.Connect(ctx, dbURL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()
	for _, statement := range []string{
		`INSERT INTO users (email, password_hash) VALUES ('owner@example.com', 'hash')`,
		`INSERT INTO projects (name, slug) VALUES ('Shop', 'shop')`,
		`INSERT INTO servers (project_id, name) SELECT id, 'prod-01' FROM projects`,
		`INSERT INTO agents (server_id, credential_hash) SELECT id, '\x01020304' FROM servers`,
		`INSERT INTO jobs (project_id, type, worker_kind, status, payload, finished_at)
			SELECT id, 'build', 'runner', 'succeeded', '{"commit_sha":"abc"}', now() FROM projects`,
		`INSERT INTO job_log_chunks (job_id, attempt_number, sequence_number, stream, content) SELECT id, 1, 1, 'stdout', 'built' FROM jobs`,
		`INSERT INTO git_connections (project_id, provider, display_name, base_url, token_ciphertext, token_key_id, webhook_ciphertext, webhook_key_id)
			SELECT id, 'gitlab', 'g', 'https://gitlab.example.com', '\x00', 'v1', '\x00', 'v1' FROM projects`,
		`INSERT INTO applications (project_id, name, git_connection_id, repository_id, repository_full_name, branch)
			SELECT project_id, 'API', id, '1', 'acme/api', 'main' FROM git_connections`,
	} {
		if _, err := conn.Exec(ctx, statement); err != nil {
			t.Fatalf("%v\n%s", err, statement)
		}
	}

	if err := migrator.Up(ctx); err != nil {
		t.Fatalf("Up from version %d: %v", olderVersion, err)
	}

	count := func(sql string) int {
		t.Helper()
		var n int
		if err := conn.QueryRow(ctx, sql).Scan(&n); err != nil {
			t.Fatalf("%v\n%s", err, sql)
		}
		return n
	}
	for table, want := range map[string]int{"users": 1, "projects": 1, "servers": 1, "agents": 1, "jobs": 1, "job_log_chunks": 1, "applications": 1} {
		if got := count("SELECT count(*) FROM " + table); got != want {
			t.Errorf("%s: %d rows after the upgrade, want %d", table, got, want)
		}
	}
	if got := count(`SELECT count(*) FROM agents WHERE credential_rotated_at IS NOT NULL AND rotation_requested_at IS NULL AND previous_credential_hash IS NULL`); got != 1 {
		t.Errorf("the existing Agent did not get usable rotation defaults")
	}
	if got := count(`SELECT count(*) FROM jobs WHERE logs_expired_at IS NULL`); got != 1 {
		t.Errorf("an existing job's logs were marked expired by the upgrade")
	}
	if got := count(`SELECT count(*) FROM applications WHERE registry_connection_id IS NULL AND image_repository IS NULL`); got != 1 {
		t.Errorf("an existing application did not keep working without an image destination")
	}
	for _, table := range []string{"registry_connections", "environments", "deployments", "releases", "project_secrets", "audit_logs"} {
		if got := count("SELECT count(*) FROM " + table); got != 0 {
			t.Errorf("%s has %d rows right after the upgrade", table, got)
		}
	}
}
