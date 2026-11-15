package store

import "context"

// DeleteVersion removes one configuration version row. The deployer uses it inside its
// transaction to skip an id whose directory is live (see deploy.Apply); generated ids
// are never handed out again (AUTOINCREMENT / sequences).
func (s *Store) DeleteVersion(ctx context.Context, id int64) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM config_versions WHERE id = ?`, id)
	return err
}

// PruneVersions deletes old configuration version rows: deployed versions beyond the
// newest keep, and failed attempts beyond the newest keepFailed. The active version and
// versions a remaining rollback points to are always kept.
func (s *Store) PruneVersions(ctx context.Context, keep, keepFailed int) (int64, error) {
	res, err := s.q.ExecContext(ctx, `DELETE FROM config_versions
		WHERE status <> 'active'
		AND id NOT IN (SELECT rollback_of FROM config_versions WHERE rollback_of IS NOT NULL)
		AND ((status <> 'failed' AND id NOT IN (SELECT id FROM config_versions WHERE status <> 'failed' ORDER BY id DESC LIMIT ?))
		  OR (status = 'failed' AND id NOT IN (SELECT id FROM config_versions WHERE status = 'failed' ORDER BY id DESC LIMIT ?)))`,
		keep, keepFailed)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
