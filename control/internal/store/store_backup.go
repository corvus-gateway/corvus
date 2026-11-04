package store

import (
	"context"
	"errors"
)

// SnapshotTo writes a consistent copy of the SQLite database to dst (VACUUM INTO), which
// must not exist. PostgreSQL databases are backed up with pg_dump instead.
func (s *Store) SnapshotTo(ctx context.Context, dst string) error {
	if s.dialect != SQLite {
		return errors.New("database snapshots are only available with SQLite; use pg_dump for PostgreSQL")
	}
	_, err := s.db.ExecContext(ctx, `VACUUM INTO ?`, dst)
	return err
}
