// Package testdb gives integration tests an isolated, migrated PostgreSQL database
package testdb

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"io"
	"log/slog"
	"net/url"
	"os"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/jakkayy/ForgeDeck/internal/database"
)

// EnvVar names the admin connection URL used to create test databases.
const EnvVar = "FORGEDECK_TEST_DATABASE_URL"

// New creates a fresh database, applies all migrations, and returns a pool
func New(t testing.TB) *pgxpool.Pool {
	t.Helper()
	dbURL := Create(t)

	migrator, err := database.NewMigrator(dbURL, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatalf("testdb: migrator: %v", err)
	}
	defer func() { _ = migrator.Close() }()
	if err := migrator.Up(context.Background()); err != nil {
		t.Fatalf("testdb: migrate: %v", err)
	}

	pool, err := database.Connect(context.Background(), dbURL)
	if err != nil {
		t.Fatalf("testdb: connect: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// Create makes an empty database and returns its URL without migrating it.
func Create(t testing.TB) string {
	t.Helper()
	adminURL := os.Getenv(EnvVar)
	if adminURL == "" {
		t.Skipf("set %s to run integration tests", EnvVar)
	}
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Fatalf("testdb: connect admin: %v", err)
	}
	defer func() { _ = admin.Close(ctx) }()

	name := "forgedeck_test_" + randomSuffix()
	ident := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+ident); err != nil {
		t.Fatalf("testdb: create database: %v", err)
	}
	t.Cleanup(func() { dropDatabase(t, adminURL, ident) })

	u, err := url.Parse(adminURL)
	if err != nil {
		t.Fatalf("testdb: parse %s: %v", EnvVar, err)
	}
	u.Path = "/" + name
	return u.String()
}

func dropDatabase(t testing.TB, adminURL, ident string) {
	ctx := context.Background()
	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Errorf("testdb: connect admin for cleanup: %v", err)
		return
	}
	defer func() { _ = admin.Close(ctx) }()
	if _, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+ident+" WITH (FORCE)"); err != nil {
		t.Errorf("testdb: drop database: %v", err)
	}
}

func randomSuffix() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
