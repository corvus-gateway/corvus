package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"
)

// ------------------------------------------------------------------ meta helpers

func (s *Store) GetMeta(ctx context.Context, key string) (string, error) {
	var v string
	err := s.q.QueryRowContext(ctx, `SELECT value FROM meta WHERE key = ?`, key).Scan(&v)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return v, err
}

func (s *Store) SetMeta(ctx context.Context, key, value string) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO meta(key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value`, key, value)
	return err
}

// ------------------------------------------------------------------ network settings

type NetworkSettings struct {
	TrustedProxies []string `json:"trusted_proxies"`
	RealIPHeader   string   `json:"real_ip_header"`
	MaskIPs        bool     `json:"mask_ips"`
	AdminAllow     []string `json:"admin_allow"` // networks allowed to reach the admin console (empty = any)
}

func (s *Store) Network(ctx context.Context) (NetworkSettings, error) {
	n := NetworkSettings{TrustedProxies: []string{}, RealIPHeader: "X-Forwarded-For"}
	v, err := s.GetMeta(ctx, "network")
	if err != nil || v == "" {
		return n, err
	}
	err = json.Unmarshal([]byte(v), &n)
	if n.TrustedProxies == nil {
		n.TrustedProxies = []string{}
	}
	return n, err
}

func (s *Store) SetNetwork(ctx context.Context, n NetworkSettings) error {
	return s.SetMeta(ctx, "network", toJSON(n))
}

// CloudflareIPs is the snapshot of Cloudflare's published edge ranges.
type CloudflareIPs struct {
	IPv4      []string  `json:"ipv4"`
	IPv6      []string  `json:"ipv6"`
	UpdatedAt time.Time `json:"updated_at"`
	Source    string    `json:"source"` // builtin | remote
}

func (s *Store) CloudflareIPs(ctx context.Context) (*CloudflareIPs, error) {
	v, err := s.GetMeta(ctx, "cloudflare_ips")
	if err != nil || v == "" {
		return nil, err
	}
	var c CloudflareIPs
	return &c, json.Unmarshal([]byte(v), &c)
}

func (s *Store) SetCloudflareIPs(ctx context.Context, c CloudflareIPs) error {
	return s.SetMeta(ctx, "cloudflare_ips", toJSON(c))
}

// ------------------------------------------------------------------ rate limits

type RateLimit struct {
	ID                int64     `json:"id"`
	Name              string    `json:"name"`
	RequestsPerSecond int       `json:"requests_per_second"`
	Burst             int       `json:"burst"`
	MaxConnections    int       `json:"max_connections"`
	StatusCode        int       `json:"status_code"`
	DryRun            bool      `json:"dry_run"`
	ExemptCIDRs       []string  `json:"exempt_cidrs"`
	Enabled           bool      `json:"enabled"`
	Hosts             int       `json:"hosts"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
}

func (s *Store) ListRateLimits(ctx context.Context) ([]RateLimit, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT r.id, r.name, r.requests_per_second, r.burst, r.max_connections, r.status_code, r.dry_run, r.exempt_cidrs, r.enabled, r.created_at, r.updated_at,
		(SELECT COUNT(*) FROM proxy_hosts p WHERE p.rate_limit_id = r.id) FROM rate_limits r ORDER BY r.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RateLimit{}
	for rows.Next() {
		var r RateLimit
		var dry, en int
		var ex string
		if err := rows.Scan(&r.ID, &r.Name, &r.RequestsPerSecond, &r.Burst, &r.MaxConnections, &r.StatusCode, &dry, &ex, &en, &r.CreatedAt, &r.UpdatedAt, &r.Hosts); err != nil {
			return nil, err
		}
		r.DryRun, r.Enabled = dry == 1, en == 1
		_ = json.Unmarshal([]byte(ex), &r.ExemptCIDRs)
		if r.ExemptCIDRs == nil {
			r.ExemptCIDRs = []string{}
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Store) RateLimitByID(ctx context.Context, id int64) (*RateLimit, error) {
	list, err := s.ListRateLimits(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

func (s *Store) SaveRateLimit(ctx context.Context, r *RateLimit) error {
	t := now()
	if r.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO rate_limits(name, requests_per_second, burst, max_connections, status_code, dry_run, exempt_cidrs, enabled, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`, r.Name, r.RequestsPerSecond, r.Burst, r.MaxConnections, r.StatusCode, b2i(r.DryRun), toJSON(r.ExemptCIDRs), b2i(r.Enabled), t, t)
		if err != nil {
			return err
		}
		r.ID, _ = res.LastInsertId()
		r.CreatedAt, r.UpdatedAt = t, t
		return nil
	}
	res, err := s.q.ExecContext(ctx, `UPDATE rate_limits SET name=?, requests_per_second=?, burst=?, max_connections=?, status_code=?, dry_run=?, exempt_cidrs=?, enabled=?, updated_at=? WHERE id=?`,
		r.Name, r.RequestsPerSecond, r.Burst, r.MaxConnections, r.StatusCode, b2i(r.DryRun), toJSON(r.ExemptCIDRs), b2i(r.Enabled), t, r.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	r.UpdatedAt = t
	return nil
}

func (s *Store) DeleteRateLimit(ctx context.Context, id int64) error {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_hosts WHERE rate_limit_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("rate limit policy is used by %d proxy host(s)", n)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM rate_limits WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ access lists

type AccessListUser struct {
	Username     string `json:"username"`
	PasswordHash string `json:"-"`
}

type AccessList struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
	// Legacy NPM-style IP rules: converted to security rules, always empty now (not exposed).
	Satisfy   string           `json:"-"`
	Allow     []string         `json:"-"`
	Deny      []string         `json:"-"`
	Users     []AccessListUser `json:"users"`
	Hosts     int              `json:"hosts"`
	CreatedAt time.Time        `json:"created_at"`
	UpdatedAt time.Time        `json:"updated_at"`
}

func (s *Store) ListAccessLists(ctx context.Context) ([]AccessList, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT a.id, a.name, a.satisfy, a.allow, a.deny, a.created_at, a.updated_at,
		(SELECT COUNT(*) FROM proxy_hosts p WHERE p.access_list_id = a.id) FROM access_lists a ORDER BY a.id`)
	if err != nil {
		return nil, err
	}
	out := []AccessList{}
	for rows.Next() {
		var a AccessList
		var allow, deny string
		if err := rows.Scan(&a.ID, &a.Name, &a.Satisfy, &allow, &deny, &a.CreatedAt, &a.UpdatedAt, &a.Hosts); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(allow), &a.Allow)
		_ = json.Unmarshal([]byte(deny), &a.Deny)
		if a.Allow == nil {
			a.Allow = []string{}
		}
		if a.Deny == nil {
			a.Deny = []string{}
		}
		out = append(out, a)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		users, err := s.accessListUsers(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Users = users
	}
	return out, nil
}

func (s *Store) accessListUsers(ctx context.Context, id int64) ([]AccessListUser, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT username, password_hash FROM access_list_users WHERE access_list_id = ? ORDER BY username`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AccessListUser{}
	for rows.Next() {
		var u AccessListUser
		if err := rows.Scan(&u.Username, &u.PasswordHash); err != nil {
			return nil, err
		}
		out = append(out, u)
	}
	return out, rows.Err()
}

func (s *Store) AccessListByID(ctx context.Context, id int64) (*AccessList, error) {
	list, err := s.ListAccessLists(ctx)
	if err != nil {
		return nil, err
	}
	for i := range list {
		if list[i].ID == id {
			return &list[i], nil
		}
	}
	return nil, ErrNotFound
}

// SaveAccessList stores the list and replaces its users (hashes must already be set).
func (s *Store) SaveAccessList(ctx context.Context, a *AccessList) error {
	return s.WithTx(ctx, func(tx *Store) error {
		t := now()
		if a.ID == 0 {
			res, err := tx.q.ExecContext(ctx, `INSERT INTO access_lists(name, satisfy, allow, deny, created_at, updated_at) VALUES (?,?,?,?,?,?)`,
				a.Name, a.Satisfy, toJSON(a.Allow), toJSON(a.Deny), t, t)
			if err != nil {
				return err
			}
			a.ID, _ = res.LastInsertId()
			a.CreatedAt = t
		} else {
			res, err := tx.q.ExecContext(ctx, `UPDATE access_lists SET name=?, satisfy=?, allow=?, deny=?, updated_at=? WHERE id=?`,
				a.Name, a.Satisfy, toJSON(a.Allow), toJSON(a.Deny), t, a.ID)
			if err != nil {
				return err
			}
			if n, _ := res.RowsAffected(); n == 0 {
				return ErrNotFound
			}
		}
		a.UpdatedAt = t
		if _, err := tx.q.ExecContext(ctx, `DELETE FROM access_list_users WHERE access_list_id = ?`, a.ID); err != nil {
			return err
		}
		for _, u := range a.Users {
			if _, err := tx.q.ExecContext(ctx, `INSERT INTO access_list_users(access_list_id, username, password_hash) VALUES (?,?,?)`, a.ID, u.Username, u.PasswordHash); err != nil {
				return err
			}
		}
		return nil
	})
}

func (s *Store) DeleteAccessList(ctx context.Context, id int64) error {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_hosts WHERE access_list_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("access list is used by %d proxy host(s)", n)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM access_lists WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ certificates (ACME state)

func (s *Store) UpdateCertificateState(ctx context.Context, id int64, status, lastError string, domains []string, notAfter time.Time) error {
	_, err := s.q.ExecContext(ctx, `UPDATE certificates SET status=?, last_error=?, domains=?, not_after=? WHERE id=?`,
		status, lastError, toJSON(domains), notAfter.UTC(), id)
	return err
}

// ReplaceCustomCertificate records a new certificate/key pair for an uploaded
// certificate (same ID, so hosts keep using it). ACME certificates are not touched.
func (s *Store) ReplaceCustomCertificate(ctx context.Context, id int64, name string, domains []string, notAfter time.Time) error {
	res, err := s.q.ExecContext(ctx, `UPDATE certificates SET name=?, domains=?, not_after=?, status='valid', last_error='' WHERE id=? AND kind='custom'`,
		name, toJSON(domains), notAfter.UTC(), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) SetCertificateStatus(ctx context.Context, id int64, status, lastError string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE certificates SET status=?, last_error=? WHERE id=?`, status, lastError, id)
	return err
}

// ------------------------------------------------------------------ ACME accounts

type ACMEAccount struct {
	Email        string
	Staging      bool
	KeyPEM       string // encrypted
	Registration string // JSON
}

func (s *Store) ACMEAccount(ctx context.Context, email string, staging bool) (*ACMEAccount, error) {
	a := &ACMEAccount{Email: email, Staging: staging}
	err := s.q.QueryRowContext(ctx, `SELECT key_pem, registration FROM acme_accounts WHERE email = ? AND staging = ?`, email, b2i(staging)).Scan(&a.KeyPEM, &a.Registration)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return a, err
}

func (s *Store) SaveACMEAccount(ctx context.Context, a *ACMEAccount) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO acme_accounts(email, staging, key_pem, registration) VALUES (?,?,?,?)
		ON CONFLICT(email, staging) DO UPDATE SET key_pem = excluded.key_pem, registration = excluded.registration`,
		a.Email, b2i(a.Staging), a.KeyPEM, a.Registration)
	return err
}

// ------------------------------------------------------------------ MFA

func (s *Store) SetTOTPSecret(ctx context.Context, userID int64, encSecret string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE users SET totp_secret = ? WHERE id = ?`, encSecret, userID)
	return err
}

func (s *Store) SetMFA(ctx context.Context, userID int64, enabled bool, recoveryHashes []string) error {
	if recoveryHashes == nil {
		recoveryHashes = []string{}
	}
	secretSQL := ""
	if !enabled {
		secretSQL = ", totp_secret = ''"
	}
	_, err := s.q.ExecContext(ctx, `UPDATE users SET mfa_enabled = ?, recovery_codes = ?`+secretSQL+` WHERE id = ?`, b2i(enabled), toJSON(recoveryHashes), userID)
	return err
}

// ReplaceRecoveryCodes stores hashes only when the stored value is still old (compare and
// swap); it reports whether it did.
func (s *Store) ReplaceRecoveryCodes(ctx context.Context, userID int64, old string, hashes []string) (bool, error) {
	res, err := s.q.ExecContext(ctx, `UPDATE users SET recovery_codes = ? WHERE id = ? AND recovery_codes = ?`, toJSON(hashes), userID, old)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}

func (s *Store) SetRecoveryCodes(ctx context.Context, userID int64, hashes []string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE users SET recovery_codes = ? WHERE id = ?`, toJSON(hashes), userID)
	return err
}

// ------------------------------------------------------------------ users (RBAC)

// Roles in ascending privilege.
var roleRank = map[string]int{"viewer": 1, "operator": 2, "admin": 3}

func ValidRole(r string) bool { return roleRank[r] > 0 }

// RoleAtLeast reports whether role has at least the privileges of min.
func RoleAtLeast(role, min string) bool { return roleRank[role] >= roleRank[min] && roleRank[min] > 0 }

func (s *Store) ListUsers(ctx context.Context) ([]User, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+userCols+` FROM users ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []User{}
	for rows.Next() {
		u, err := scanUser(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *u)
	}
	return out, rows.Err()
}

func (s *Store) UpdateUser(ctx context.Context, id int64, name, role string, disabled bool) error {
	res, err := s.q.ExecContext(ctx, `UPDATE users SET name = ?, role = ?, disabled = ? WHERE id = ?`, name, role, b2i(disabled), id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *Store) DeleteUser(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// CountActiveAdmins counts enabled administrators, optionally excluding one user.
func (s *Store) CountActiveAdmins(ctx context.Context, exceptID int64) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users WHERE role = 'admin' AND disabled = 0 AND id <> ?`, exceptID).Scan(&n)
	return n, err
}

// ErrLastAdmin: the change would leave no active administrator.
var ErrLastAdmin = errors.New("at least one active administrator is required")

// KeepingAdmin runs fn in a transaction and rolls it back when no active administrator
// remains afterwards. The administrator rows are locked first (PostgreSQL; SQLite has a
// single writer), so two changes at the same time cannot both remove "the other" admin.
func (s *Store) KeepingAdmin(ctx context.Context, fn func(tx *Store) error) error {
	return s.WithTx(ctx, func(tx *Store) error {
		if tx.dialect == Postgres {
			rows, err := tx.q.QueryContext(ctx, `SELECT id FROM users WHERE role = 'admin' AND disabled = 0 FOR UPDATE`)
			if err != nil {
				return err
			}
			rows.Close()
		}
		if err := fn(tx); err != nil {
			return err
		}
		n, err := tx.CountActiveAdmins(ctx, 0)
		if err != nil {
			return err
		}
		if n == 0 {
			return ErrLastAdmin
		}
		return nil
	})
}

// ClaimTOTPStep atomically records step as the user's last used TOTP step. It returns
// false when this step (or a later one) was already used — a replayed code (H11).
func (s *Store) ClaimTOTPStep(ctx context.Context, userID, step int64) (bool, error) {
	res, err := s.q.ExecContext(ctx, `UPDATE users SET mfa_last_step = ? WHERE id = ? AND mfa_last_step < ?`, step, userID, step)
	if err != nil {
		return false, err
	}
	n, err := res.RowsAffected()
	return n == 1, err
}
