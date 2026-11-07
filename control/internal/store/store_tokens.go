package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// ------------------------------------------------------------------ API tokens

// APIToken is a long-lived Bearer credential. The plaintext is never stored.
type APIToken struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Prefix       string     `json:"prefix"`
	Role         string     `json:"role"`
	CreatedBy    int64      `json:"created_by"`
	CreatorEmail string     `json:"created_by_email"`
	CreatedAt    time.Time  `json:"created_at"`
	LastUsedAt   *time.Time `json:"last_used_at"`
	ExpiresAt    *time.Time `json:"expires_at"`
}

// Expired reports whether the token has an expiry in the past.
func (t *APIToken) Expired(now time.Time) bool {
	return t.ExpiresAt != nil && !now.Before(*t.ExpiresAt)
}

const tokenCols = `t.id, t.name, t.prefix, t.role, t.created_by, COALESCE(u.email, ''), t.created_at, t.last_used_at, t.expires_at`

func scanToken(row interface{ Scan(...any) error }) (*APIToken, error) {
	t := &APIToken{}
	var last, exp sql.NullTime
	if err := row.Scan(&t.ID, &t.Name, &t.Prefix, &t.Role, &t.CreatedBy, &t.CreatorEmail, &t.CreatedAt, &last, &exp); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if last.Valid {
		v := last.Time.UTC()
		t.LastUsedAt = &v
	}
	if exp.Valid {
		v := exp.Time.UTC()
		t.ExpiresAt = &v
	}
	return t, nil
}

func (s *Store) CreateAPIToken(ctx context.Context, name, prefix, hash, role string, createdBy int64, expiresAt *time.Time) (*APIToken, error) {
	var exp any
	if expiresAt != nil {
		exp = expiresAt.UTC().Truncate(time.Second)
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO api_tokens(name, prefix, token_hash, role, created_by, created_at, expires_at) VALUES (?,?,?,?,?,?,?)`,
		name, prefix, hash, role, createdBy, now(), exp)
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.APITokenByID(ctx, id)
}

func (s *Store) APITokenByID(ctx context.Context, id int64) (*APIToken, error) {
	return scanToken(s.q.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens t LEFT JOIN users u ON u.id = t.created_by WHERE t.id = ?`, id))
}

// APITokenByHash looks a token up by the SHA-256 of its plaintext.
func (s *Store) APITokenByHash(ctx context.Context, hash string) (*APIToken, error) {
	return scanToken(s.q.QueryRowContext(ctx, `SELECT `+tokenCols+` FROM api_tokens t LEFT JOIN users u ON u.id = t.created_by WHERE t.token_hash = ?`, hash))
}

func (s *Store) ListAPITokens(ctx context.Context) ([]APIToken, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+tokenCols+` FROM api_tokens t LEFT JOIN users u ON u.id = t.created_by ORDER BY t.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []APIToken{}
	for rows.Next() {
		t, err := scanToken(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *t)
	}
	return out, rows.Err()
}

// TouchAPIToken records use, at most once a minute per token.
func (s *Store) TouchAPIToken(ctx context.Context, id int64) error {
	t := now()
	_, err := s.q.ExecContext(ctx, `UPDATE api_tokens SET last_used_at = ? WHERE id = ? AND (last_used_at IS NULL OR last_used_at < ?)`, t, id, t.Add(-time.Minute))
	return err
}

func (s *Store) DeleteAPIToken(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM api_tokens WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteUserAPITokens removes every API token the user created and returns how many.
func (s *Store) DeleteUserAPITokens(ctx context.Context, userID int64) (int64, error) {
	res, err := s.q.ExecContext(ctx, `DELETE FROM api_tokens WHERE created_by = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}
