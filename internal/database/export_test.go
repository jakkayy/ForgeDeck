package database

import "context"

// DownAll rolls back every migration; test-only, production goes one version at a time.
func (m *Migrator) DownAll(ctx context.Context) error {
	_, err := m.provider.DownTo(ctx, 0)
	return err
}

// UpTo applies migrations through version, to simulate an older installation; test-only.
func (m *Migrator) UpTo(ctx context.Context, version int64) error {
	_, err := m.provider.UpTo(ctx, version)
	return err
}
