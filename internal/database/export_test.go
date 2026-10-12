package database

import "context"

// DownAll rolls back every migration. Test-only: production rollbacks go one
// version at a time through Down.
func (m *Migrator) DownAll(ctx context.Context) error {
	_, err := m.provider.DownTo(ctx, 0)
	return err
}
