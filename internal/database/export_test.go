package database

import "context"

// DownAll rolls back every migration. Test-only: production rollbacks go one
// version at a time through Down.
func (m *Migrator) DownAll(ctx context.Context) error {
	_, err := m.provider.DownTo(ctx, 0)
	return err
}

// UpTo applies migrations up to and including version. Test-only: it lets a
// test stop at an older schema, the way an installation that has not been
// upgraded yet looks.
func (m *Migrator) UpTo(ctx context.Context, version int64) error {
	_, err := m.provider.UpTo(ctx, version)
	return err
}
