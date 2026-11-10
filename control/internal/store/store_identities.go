package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strings"
)

// ------------------------------------------------------------------ SSO identities

// NormIssuer is the form an OIDC issuer is stored and compared in (no trailing slash).
func NormIssuer(issuer string) string { return strings.TrimRight(strings.TrimSpace(issuer), "/") }

// UserByIdentity returns the user linked to the identity provider account (issuer, subject).
func (s *Store) UserByIdentity(ctx context.Context, issuer, subject string) (*User, error) {
	return scanUser(s.q.QueryRowContext(ctx, `SELECT `+userColsU+` FROM user_identities i JOIN users u ON u.id = i.user_id
		WHERE i.issuer = ? AND i.subject = ?`, NormIssuer(issuer), subject))
}

// IdentitySubject returns the subject user is linked to at issuer (ErrNotFound if none).
func (s *Store) IdentitySubject(ctx context.Context, userID int64, issuer string) (string, error) {
	var sub string
	err := s.q.QueryRowContext(ctx, `SELECT subject FROM user_identities WHERE user_id = ? AND issuer = ?`, userID, NormIssuer(issuer)).Scan(&sub)
	if errors.Is(err, sql.ErrNoRows) {
		return "", ErrNotFound
	}
	return sub, err
}

// LinkIdentity links the identity provider account (issuer, subject) to a user. A
// subject already linked to someone, or a user already linked at the issuer, is a
// unique-constraint error (IsUnique).
func (s *Store) LinkIdentity(ctx context.Context, userID int64, issuer, subject string) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO user_identities(issuer, subject, user_id, created_at) VALUES (?,?,?,?)`,
		NormIssuer(issuer), subject, userID, now())
	return err
}

// DeleteUserIdentities removes every SSO link of a user.
func (s *Store) DeleteUserIdentities(ctx context.Context, userID int64) (int64, error) {
	res, err := s.q.ExecContext(ctx, `DELETE FROM user_identities WHERE user_id = ?`, userID)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// linkedUsers returns the ids of users with at least one SSO link.
func (s *Store) linkedUsers(ctx context.Context) (map[int64]bool, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT DISTINCT user_id FROM user_identities`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64]bool{}
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out[id] = true
	}
	return out, rows.Err()
}

// backfillIdentities links users to the identity provider accounts they already
// signed in with (successful SSO sign-ins are audited with issuer and subject), so
// the upgrade does not cut existing SSO users off. The most recent sign-in wins.
func backfillIdentities(ctx context.Context, s *Store) error {
	rows, err := s.q.QueryContext(ctx, `SELECT a.user_id, a.detail FROM audit_log a JOIN users u ON u.id = a.user_id
		WHERE a.action = 'auth.sso_login' ORDER BY a.id DESC`)
	if err != nil {
		return err
	}
	type link struct {
		uid      int64
		iss, sub string
	}
	var links []link
	for rows.Next() {
		var uid int64
		var detail string
		if err := rows.Scan(&uid, &detail); err != nil {
			rows.Close()
			return err
		}
		var d struct {
			Issuer string `json:"issuer"`
			Sub    string `json:"sub"`
		}
		if json.Unmarshal([]byte(detail), &d) != nil || NormIssuer(d.Issuer) == "" || d.Sub == "" || len(d.Sub) > MaxSubject {
			continue
		}
		links = append(links, link{uid, d.Issuer, d.Sub})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	for _, l := range links {
		// earlier (older) rows lose against links already made from newer ones
		if _, err := s.q.ExecContext(ctx, `INSERT INTO user_identities(issuer, subject, user_id, created_at) VALUES (?,?,?,?) ON CONFLICT DO NOTHING`,
			NormIssuer(l.iss), l.sub, l.uid, now()); err != nil {
			return err
		}
	}
	return nil
}

// MaxSubject is the longest subject accepted (OIDC: at most 255 ASCII characters).
const MaxSubject = 512

// afterMigration runs Go code in the transaction of a migration (once, like the SQL).
var afterMigration = map[string]func(context.Context, *Store) error{
	"0029_user_identities": backfillIdentities,
}
