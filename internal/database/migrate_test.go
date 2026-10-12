package database_test

import (
	"context"
	"io"
	"log/slog"
	"testing"

	"github.com/jakkayy/ForgeDeck/internal/database"
	"github.com/jakkayy/ForgeDeck/internal/testdb"
)

// TestMigrationsRoundTrip applies every migration, rolls all of them back,
// and applies them again, proving each Down reverses its Up.
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
