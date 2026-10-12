// Package database owns PostgreSQL connectivity and schema migrations.
package database

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log/slog"

	_ "github.com/jackc/pgx/v5/stdlib" // registers the "pgx" database/sql driver
	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"

	"github.com/jakkayy/ForgeDeck/migrations"
)

// Migrator applies the embedded migrations. A PostgreSQL advisory lock keeps
// concurrently starting Control Plane processes from migrating at once.
type Migrator struct {
	db       *sql.DB
	provider *goose.Provider
	logger   *slog.Logger
}

// NewMigrator opens a dedicated connection for migrations.
func NewMigrator(databaseURL string, logger *slog.Logger) (*Migrator, error) {
	db, err := sql.Open("pgx", databaseURL)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	locker, err := lock.NewPostgresSessionLocker()
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("create migration lock: %w", err)
	}
	provider, err := goose.NewProvider(goose.DialectPostgres, db, migrations.FS,
		goose.WithSessionLocker(locker),
		goose.WithDisableGlobalRegistry(true),
	)
	if err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("load migrations: %w", err)
	}
	return &Migrator{db: db, provider: provider, logger: logger}, nil
}

// Close releases the migration connection.
func (m *Migrator) Close() error { return m.db.Close() }

// Up applies all pending migrations.
func (m *Migrator) Up(ctx context.Context) error {
	results, err := m.provider.Up(ctx)
	if err != nil {
		return fmt.Errorf("migrate up: %w", err)
	}
	for _, r := range results {
		m.logger.Info("migration applied", "version", r.Source.Version, "file", r.Source.Path, "duration_ms", r.Duration.Milliseconds())
	}
	return nil
}

// Down rolls back the most recent migration.
func (m *Migrator) Down(ctx context.Context) error {
	r, err := m.provider.Down(ctx)
	if err != nil {
		return fmt.Errorf("migrate down: %w", err)
	}
	m.logger.Info("migration rolled back", "version", r.Source.Version, "file", r.Source.Path)
	return nil
}

// PrintStatus writes one line per migration with its applied state.
func (m *Migrator) PrintStatus(ctx context.Context, w io.Writer) error {
	statuses, err := m.provider.Status(ctx)
	if err != nil {
		return fmt.Errorf("migration status: %w", err)
	}
	for _, s := range statuses {
		applied := "pending"
		if s.State == goose.StateApplied {
			applied = "applied " + s.AppliedAt.Format("2006-01-02 15:04:05")
		}
		if _, err := fmt.Fprintf(w, "%05d  %-40s %s\n", s.Source.Version, s.Source.Path, applied); err != nil {
			return err
		}
	}
	return nil
}
