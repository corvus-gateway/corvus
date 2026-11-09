package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ------------------------------------------------------------------ IP bans (auto / manual)

type IPBan struct {
	ID        int64      `json:"id"`
	IP        string     `json:"ip"`
	Reason    string     `json:"reason"`
	Source    string     `json:"source"` // auto | manual
	Offenses  int        `json:"offenses"`
	CreatedAt time.Time  `json:"created_at"`
	ExpiresAt *time.Time `json:"expires_at"` // nil = until removed
	CreatedBy *int64     `json:"created_by,omitempty"`
}

const banCols = `id, ip, reason, source, offenses, created_at, expires_at, created_by`

func scanBan(row interface{ Scan(...any) error }) (*IPBan, error) {
	b := &IPBan{}
	var exp sql.NullTime
	var by sql.NullInt64
	if err := row.Scan(&b.ID, &b.IP, &b.Reason, &b.Source, &b.Offenses, &b.CreatedAt, &exp, &by); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if exp.Valid {
		t := exp.Time
		b.ExpiresAt = &t
	}
	if by.Valid {
		b.CreatedBy = &by.Int64
	}
	return b, nil
}

// ActiveBans lists bans that have not expired at now.
func (s *Store) ActiveBans(ctx context.Context, now time.Time) ([]IPBan, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+banCols+` FROM ip_bans WHERE expires_at IS NULL OR expires_at > ? ORDER BY created_at DESC`, now.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []IPBan{}
	for rows.Next() {
		b, err := scanBan(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *b)
	}
	return out, rows.Err()
}

// BanByIP returns the ban row for ip (active or expired).
func (s *Store) BanByIP(ctx context.Context, ip string) (*IPBan, error) {
	return scanBan(s.q.QueryRowContext(ctx, `SELECT `+banCols+` FROM ip_bans WHERE ip = ?`, ip))
}

// UpsertBan creates or renews the ban for b.IP (one row per IP; offenses carried over).
func (s *Store) UpsertBan(ctx context.Context, b *IPBan) error {
	t := now()
	old, err := s.BanByIP(ctx, b.IP)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return err
	}
	if old == nil {
		res, err := s.q.ExecContext(ctx, `INSERT INTO ip_bans(ip, reason, source, offenses, created_at, expires_at, created_by) VALUES (?,?,?,?,?,?,?)`,
			b.IP, b.Reason, b.Source, b.Offenses, t, b.ExpiresAt, b.CreatedBy)
		if err != nil {
			return err
		}
		b.ID, _ = res.LastInsertId()
		b.CreatedAt = t
		return nil
	}
	_, err = s.q.ExecContext(ctx, `UPDATE ip_bans SET reason=?, source=?, offenses=?, created_at=?, expires_at=?, created_by=? WHERE id=?`,
		b.Reason, b.Source, b.Offenses, t, b.ExpiresAt, b.CreatedBy, old.ID)
	b.ID, b.CreatedAt = old.ID, t
	return err
}

// LiftBan ends a ban now (the row is kept so repeat offenders are recognised).
func (s *Store) LiftBan(ctx context.Context, id int64) (*IPBan, error) {
	b, err := scanBan(s.q.QueryRowContext(ctx, `SELECT `+banCols+` FROM ip_bans WHERE id = ?`, id))
	if err != nil {
		return nil, err
	}
	if b.ExpiresAt != nil && !b.ExpiresAt.After(now()) {
		return nil, ErrNotFound // already lifted or expired: nothing to do
	}
	t := now().Add(-time.Second)
	_, err = s.q.ExecContext(ctx, `UPDATE ip_bans SET expires_at = ? WHERE id = ?`, t, id)
	return b, err
}

// MakeBanPermanent removes the end time of a ban.
func (s *Store) MakeBanPermanent(ctx context.Context, id int64) (*IPBan, error) {
	if _, err := s.q.ExecContext(ctx, `UPDATE ip_bans SET expires_at = NULL WHERE id = ?`, id); err != nil {
		return nil, err
	}
	return scanBan(s.q.QueryRowContext(ctx, `SELECT `+banCols+` FROM ip_bans WHERE id = ?`, id))
}

// PruneBans removes rows that expired before cutoff (offense history older than that is forgotten).
func (s *Store) PruneBans(ctx context.Context, cutoff time.Time) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM ip_bans WHERE expires_at IS NOT NULL AND expires_at < ?`, cutoff.UTC())
	return err
}
