// Package store is the Corvus configuration database (SQLite by default).
package store

import (
	"context"
	"database/sql"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	_ "modernc.org/sqlite"
)

//go:embed migrations/*.sql
var migrations embed.FS

var ErrNotFound = errors.New("not found")

type querier interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

// Store is bound either to the database or to an open transaction (see WithTx).
type Store struct {
	db      *sql.DB
	q       querier
	tx      bool
	dialect string
}

// Open opens (and creates/migrates) the configuration database. target is either
// a SQLite file path or a PostgreSQL URL (postgres://user:pass@host/db).
func Open(target string) (*Store, error) {
	s := &Store{dialect: SQLite}
	var db *sql.DB
	var err error
	if strings.HasPrefix(target, "postgres://") || strings.HasPrefix(target, "postgresql://") {
		s.dialect = Postgres
		if db, err = sql.Open("pgx", target); err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(10)
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			db.Close()
			return nil, fmt.Errorf("connect to PostgreSQL: %w", err)
		}
	} else {
		dsn := "file:" + target + "?_pragma=foreign_keys(1)&_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)"
		if db, err = sql.Open("sqlite", dsn); err != nil {
			return nil, err
		}
		db.SetMaxOpenConns(1) // SQLite: serialize writers; reads are fast enough for a control plane
	}
	s.db = db
	s.q = s.wrap(db)
	if err := s.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return s, nil
}

func (s *Store) Close() error { return s.db.Close() }

// WithTx runs fn with a Store bound to a single transaction; fn's error rolls it back.
func (s *Store) WithTx(ctx context.Context, fn func(tx *Store) error) error {
	if s.tx {
		return fn(s)
	}
	t, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if err := fn(&Store{db: s.db, q: s.wrap(t), tx: true, dialect: s.dialect}); err != nil {
		t.Rollback()
		return err
	}
	return t.Commit()
}

// BumpConfigRev marks the configuration as changed and returns the new revision.
func (s *Store) BumpConfigRev(ctx context.Context) (int64, error) {
	if _, err := s.q.ExecContext(ctx, `UPDATE meta SET value = CAST(CAST(value AS INTEGER) + 1 AS TEXT) WHERE key = 'config_rev'`); err != nil {
		return 0, err
	}
	return s.ConfigRev(ctx)
}

func (s *Store) ConfigRev(ctx context.Context) (int64, error) {
	var v int64
	err := s.q.QueryRowContext(ctx, `SELECT CAST(value AS INTEGER) FROM meta WHERE key = 'config_rev'`).Scan(&v)
	return v, err
}

func (s *Store) migrate(ctx context.Context) error {
	if _, err := s.q.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY, applied_at TIMESTAMP NOT NULL)`); err != nil {
		return err
	}
	names, err := fs.Glob(migrations, "migrations/*.sql")
	if err != nil {
		return err
	}
	sort.Strings(names)
	for _, name := range names {
		version := strings.TrimSuffix(strings.TrimPrefix(name, "migrations/"), ".sql")
		var n int
		if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = ?`, version).Scan(&n); err != nil {
			return err
		}
		if n > 0 {
			continue
		}
		body, _ := migrations.ReadFile(name)
		tx, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, migrationSQL(s.dialect, string(body))); err != nil {
			tx.Rollback()
			return fmt.Errorf("migration %s: %w", version, err)
		}
		if hook := afterMigration[version]; hook != nil {
			if err := hook(ctx, &Store{db: s.db, q: s.wrap(tx), tx: true, dialect: s.dialect}); err != nil {
				tx.Rollback()
				return fmt.Errorf("migration %s: %w", version, err)
			}
		}
		if _, err := s.wrap(tx).ExecContext(ctx, `INSERT INTO schema_migrations(version, applied_at) VALUES (?, ?)`, version, now()); err != nil {
			tx.Rollback()
			return err
		}
		if err := tx.Commit(); err != nil {
			return err
		}
	}
	return nil
}

func now() time.Time { return time.Now().UTC().Truncate(time.Second) }

func b2i(b bool) int {
	if b {
		return 1
	}
	return 0
}

func toJSON(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// ------------------------------------------------------------------ users & sessions

type User struct {
	ID          int64      `json:"id"`
	Email       string     `json:"email"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	CreatedAt   time.Time  `json:"created_at"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	MFAEnabled  bool       `json:"mfa_enabled"`
	Disabled    bool       `json:"disabled"`
	SSOLinked   bool       `json:"sso_linked,omitempty"` // set by ListUsers only
	hash        string
	totpSecret  string
	recovery    string
}

func (u *User) TOTPSecret() string    { return u.totpSecret }
func (u *User) RecoveryCodes() string { return u.recovery }

func (u *User) PasswordHash() string { return u.hash }

func (s *Store) CountUsers(ctx context.Context) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM users`).Scan(&n)
	return n, err
}

func (s *Store) CreateUser(ctx context.Context, email, name, hash, role string) (*User, error) {
	res, err := s.q.ExecContext(ctx, `INSERT INTO users(email, name, password_hash, role, created_at) VALUES (?,?,?,?,?)`,
		strings.ToLower(email), name, hash, role, now())
	if err != nil {
		return nil, err
	}
	id, _ := res.LastInsertId()
	return s.UserByID(ctx, id)
}

func scanUser(row interface{ Scan(...any) error }) (*User, error) {
	u := &User{}
	var last sql.NullTime
	var mfa, dis int
	if err := row.Scan(&u.ID, &u.Email, &u.Name, &u.hash, &u.Role, &u.CreatedAt, &last, &u.totpSecret, &mfa, &u.recovery, &dis); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if last.Valid {
		u.LastLoginAt = &last.Time
	}
	u.MFAEnabled, u.Disabled = mfa == 1, dis == 1
	return u, nil
}

const userCols = `id, email, name, password_hash, role, created_at, last_login_at, totp_secret, mfa_enabled, recovery_codes, disabled`

// userColsU is userCols for queries joining users as "u".
const userColsU = `u.id, u.email, u.name, u.password_hash, u.role, u.created_at, u.last_login_at, u.totp_secret, u.mfa_enabled, u.recovery_codes, u.disabled`

func (s *Store) UserByID(ctx context.Context, id int64) (*User, error) {
	return scanUser(s.q.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE id = ?`, id))
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.q.QueryRowContext(ctx, `SELECT `+userCols+` FROM users WHERE email = ?`, strings.ToLower(email)))
}

func (s *Store) TouchLogin(ctx context.Context, id int64) error {
	_, err := s.q.ExecContext(ctx, `UPDATE users SET last_login_at = ? WHERE id = ?`, now(), id)
	return err
}

func (s *Store) UpdatePassword(ctx context.Context, id int64, hash string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE users SET password_hash = ? WHERE id = ?`, hash, id)
	return err
}

func (s *Store) CreateSession(ctx context.Context, tokenHash string, userID int64, ttl time.Duration, ip, ua string) error {
	t := now()
	_, err := s.q.ExecContext(ctx, `INSERT INTO sessions(token_hash, user_id, created_at, expires_at, ip, user_agent) VALUES (?,?,?,?,?,?)`,
		tokenHash, userID, t, t.Add(ttl), ip, ua)
	return err
}

// SessionIdle is how long a session may stay unused before it expires.
const SessionIdle = 30 * time.Minute

// SessionUser returns the user for a live session (absolute and idle expiry).
func (s *Store) SessionUser(ctx context.Context, tokenHash string) (*User, error) {
	t := now()
	u, err := scanUser(s.q.QueryRowContext(ctx, `SELECT u.id, u.email, u.name, u.password_hash, u.role, u.created_at, u.last_login_at, u.totp_secret, u.mfa_enabled, u.recovery_codes, u.disabled
		FROM sessions s JOIN users u ON u.id = s.user_id
		WHERE s.token_hash = ? AND s.expires_at > ? AND COALESCE(s.last_seen_at, s.created_at) > ? AND u.disabled = 0`, tokenHash, t, t.Add(-SessionIdle)))
	if err != nil {
		return nil, err
	}
	_, _ = s.q.ExecContext(ctx, `UPDATE sessions SET last_seen_at = ? WHERE token_hash = ? AND COALESCE(last_seen_at, created_at) < ?`, t, tokenHash, t.Add(-time.Minute))
	return u, nil
}

func (s *Store) DeleteSession(ctx context.Context, tokenHash string) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE token_hash = ?`, tokenHash)
	return err
}

func (s *Store) DeleteUserSessions(ctx context.Context, userID int64) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE user_id = ?`, userID)
	return err
}

func (s *Store) PurgeExpiredSessions(ctx context.Context) error {
	_, err := s.q.ExecContext(ctx, `DELETE FROM sessions WHERE expires_at <= ?`, now())
	return err
}

// ------------------------------------------------------------------ certificates

type Certificate struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Domains   []string  `json:"domains"`
	NotAfter  time.Time `json:"not_after"`
	CreatedAt time.Time `json:"created_at"`
	Status    string    `json:"status"` // valid | pending | error
	LastError string    `json:"last_error"`
	AutoRenew bool      `json:"auto_renew"`
	Challenge string    `json:"challenge,omitempty"`
	// DNSCredentialID is the saved DNS credential used for DNS-01 issuance and renewals.
	DNSCredentialID *int64   `json:"dns_credential_id,omitempty"`
	ACME            ACMEInfo `json:"-"`
}

// ACMEInfo is the issuance configuration of an ACME certificate.
type ACMEInfo struct {
	Email       string `json:"email,omitempty"`
	Challenge   string `json:"challenge,omitempty"` // http-01 | dns-01
	DNSProvider string `json:"dns_provider,omitempty"`
	Credentials string `json:"credentials,omitempty"` // encrypted JSON (data key); legacy, see DNSCredentialID
	Staging     bool   `json:"staging,omitempty"`
}

func (s *Store) CreateCertificate(ctx context.Context, c *Certificate) error {
	c.CreatedAt = now()
	if c.Status == "" {
		c.Status = "valid"
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO certificates(name, kind, domains, not_after, created_at, status, last_error, auto_renew, acme, dns_credential_id) VALUES (?,?,?,?,?,?,?,?,?,?)`,
		c.Name, c.Kind, toJSON(c.Domains), c.NotAfter.UTC(), c.CreatedAt, c.Status, c.LastError, b2i(c.AutoRenew), toJSON(c.ACME), c.DNSCredentialID)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) ListCertificates(ctx context.Context) ([]Certificate, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, name, kind, domains, not_after, created_at, status, last_error, auto_renew, acme, dns_credential_id FROM certificates ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Certificate{}
	for rows.Next() {
		var c Certificate
		var domains, acme string
		var auto int
		var cred sql.NullInt64
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &domains, &c.NotAfter, &c.CreatedAt, &c.Status, &c.LastError, &auto, &acme, &cred); err != nil {
			return nil, err
		}
		if cred.Valid {
			c.DNSCredentialID = &cred.Int64
		}
		_ = json.Unmarshal([]byte(domains), &c.Domains)
		_ = json.Unmarshal([]byte(acme), &c.ACME)
		c.AutoRenew = auto == 1
		c.Challenge = c.ACME.Challenge
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) CertificateByID(ctx context.Context, id int64) (*Certificate, error) {
	list, err := s.ListCertificates(ctx)
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

func (s *Store) DeleteCertificate(ctx context.Context, id int64) error {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_hosts WHERE certificate_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("certificate is used by %d proxy host(s)", n)
	}
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM redirection_hosts WHERE certificate_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("certificate is used by %d redirection host(s)", n)
	}
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM streams WHERE certificate_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("certificate is used by %d stream(s)", n)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM certificates WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ access queues

type Waitroom struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Enabled   bool      `json:"enabled"`
	Capacity  int       `json:"capacity"`
	ActiveTTL int       `json:"active_ttl"`
	GrantTTL  int       `json:"grant_ttl"`
	QueueTTL  int       `json:"queue_ttl"`
	Refresh   int       `json:"refresh"`
	AdmitRate int       `json:"admit_rate"`
	FailMode  string    `json:"fail_mode"`
	MaxQueue  int       `json:"max_queue"`
	MaxNewIP  int       `json:"max_new_per_ip"`
	BindIP    bool      `json:"bind_ip"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const wrCols = `id, name, enabled, capacity, active_ttl, grant_ttl, queue_ttl, refresh, admit_rate, fail_mode, max_queue, max_new_per_ip, bind_ip, created_at, updated_at`

func scanWaitroom(row interface{ Scan(...any) error }) (*Waitroom, error) {
	w := &Waitroom{}
	var en, bind int
	if err := row.Scan(&w.ID, &w.Name, &en, &w.Capacity, &w.ActiveTTL, &w.GrantTTL, &w.QueueTTL, &w.Refresh, &w.AdmitRate, &w.FailMode, &w.MaxQueue, &w.MaxNewIP, &bind, &w.CreatedAt, &w.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	w.Enabled, w.BindIP = en == 1, bind == 1
	return w, nil
}

func (s *Store) ListWaitrooms(ctx context.Context) ([]Waitroom, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+wrCols+` FROM waitrooms ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Waitroom{}
	for rows.Next() {
		w, err := scanWaitroom(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *w)
	}
	return out, rows.Err()
}

func (s *Store) WaitroomByID(ctx context.Context, id int64) (*Waitroom, error) {
	return scanWaitroom(s.q.QueryRowContext(ctx, `SELECT `+wrCols+` FROM waitrooms WHERE id = ?`, id))
}

func (s *Store) SaveWaitroom(ctx context.Context, w *Waitroom) error {
	t := now()
	if w.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO waitrooms(name, enabled, capacity, active_ttl, grant_ttl, queue_ttl, refresh, admit_rate, fail_mode, max_queue, max_new_per_ip, bind_ip, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, w.Name, b2i(w.Enabled), w.Capacity, w.ActiveTTL, w.GrantTTL, w.QueueTTL, w.Refresh, w.AdmitRate, w.FailMode, w.MaxQueue, w.MaxNewIP, b2i(w.BindIP), t, t)
		if err != nil {
			return err
		}
		w.ID, _ = res.LastInsertId()
		w.CreatedAt, w.UpdatedAt = t, t
		return nil
	}
	res, err := s.q.ExecContext(ctx, `UPDATE waitrooms SET name=?, enabled=?, capacity=?, active_ttl=?, grant_ttl=?, queue_ttl=?, refresh=?, admit_rate=?, fail_mode=?, max_queue=?, max_new_per_ip=?, bind_ip=?, updated_at=? WHERE id=?`,
		w.Name, b2i(w.Enabled), w.Capacity, w.ActiveTTL, w.GrantTTL, w.QueueTTL, w.Refresh, w.AdmitRate, w.FailMode, w.MaxQueue, w.MaxNewIP, b2i(w.BindIP), t, w.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	w.UpdatedAt = t
	return nil
}

func (s *Store) DeleteWaitroom(ctx context.Context, id int64) error {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_hosts WHERE waitroom_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("access queue is used by %d proxy host(s)", n)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM waitrooms WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ proxy hosts

type ProxyHost struct {
	ID             int64     `json:"id"`
	Domains        []string  `json:"domains"`
	ForwardScheme  string    `json:"forward_scheme"`
	ForwardHost    string    `json:"forward_host"`
	ForwardPort    int       `json:"forward_port"`
	Websocket      bool      `json:"websocket"`
	CertificateID  *int64    `json:"certificate_id"`
	SSLForced      bool      `json:"ssl_forced"`
	HTTP2          bool      `json:"http2"`
	WaitroomID     *int64    `json:"waitroom_id"`
	WaitroomPath   string    `json:"waitroom_path"`
	AdvancedConfig string    `json:"advanced_config"`
	Enabled        bool      `json:"enabled"`
	ClientIP       string    `json:"client_ip"` // direct | cloudflare | trusted_proxies
	RateLimitID    *int64    `json:"rate_limit_id"`
	AccessListID   *int64    `json:"access_list_id"`
	BlockExploits  bool      `json:"block_exploits"`
	BodyInspection string    `json:"body_inspection"` // off | log | block
	BodyMaxKB      int       `json:"body_max_kb"`
	BasicAuthUsers []string  `json:"basic_auth_users,omitempty"` // read-only, filled by the API
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`

	SecurityHeaders string `json:"security_headers"` // off | basic | strict

	// "Under attack" mode: off | on (challenge every request) | auto (active while the
	// control plane sees an attack, see internal/underattack).
	UnderAttack       string            `json:"under_attack"`
	UnderAttackConfig UnderAttackConfig `json:"under_attack_config"`

	MaxUploadMB      *int `json:"max_upload_mb"`      // nil = global default (100 MB), 0 = unlimited
	UpstreamTimeoutS *int `json:"upstream_timeout_s"` // upstream read/send timeout; nil = default (60 s, WebSocket 1 h)
	Gzip             bool `json:"gzip"`               // compress text responses

	Notes string `json:"notes"` // free text for people; not used by the gateway

	// Custom locations: path prefixes routed to their own upstream (NPM "custom locations").
	Locations []Location `json:"locations"`
}

// Location routes a path prefix of a proxy host to its own upstream. It keeps the host's
// protections (security rules, bans, rate limit, Basic auth, access queue).
type Location struct {
	Path           string `json:"path"` // prefix, e.g. /api; "/" replaces the host's forward host
	ForwardScheme  string `json:"forward_scheme"`
	ForwardHost    string `json:"forward_host"`
	ForwardPort    int    `json:"forward_port"`
	ForwardPath    string `json:"forward_path"`    // optional: replaces the matched prefix
	AdvancedConfig string `json:"advanced_config"` // optional nginx directives for this location
}

// UnderAttackConfig holds the parameters of a host's "under attack" mode. Zero values
// are replaced with the defaults by validate.ProxyHost.
type UnderAttackConfig struct {
	Mode       string `json:"mode"`       // challenge type: js | cookie
	Difficulty int    `json:"difficulty"` // proof-of-work bits (js)
	TTL        int    `json:"ttl_s"`      // pass cookie lifetime, seconds
	Bind       string `json:"bind"`       // ip | ip_ua
	// Auto mode: activate when, over the last WindowS seconds, the host served more
	// than RPS requests per second, or at least BlockedPct percent of its requests were
	// blocked or rate-limited (0 = ratio not used); stay active until DurationS seconds
	// after the last round in which a threshold was exceeded.
	RPS        float64  `json:"rps"`
	BlockedPct *float64 `json:"blocked_pct"` // nil = default
	WindowS    int      `json:"window_s"`
	DurationS  int      `json:"duration_s"`
}

const phCols = `id, domains, forward_scheme, forward_host, forward_port, websocket, certificate_id, ssl_forced, http2, waitroom_id, waitroom_path, advanced_config, enabled, client_ip, rate_limit_id, access_list_id, block_exploits, body_inspection, body_max_kb, security_headers, under_attack, under_attack_config, max_upload_mb, upstream_timeout_s, gzip, locations, notes, created_at, updated_at`

func scanProxyHost(row interface{ Scan(...any) error }) (*ProxyHost, error) {
	h := &ProxyHost{}
	var domains, uaCfg, locs string
	var ws, ssl, h2, en, be, gz int
	var cert, wr, rl, al, mu, ut sql.NullInt64
	if err := row.Scan(&h.ID, &domains, &h.ForwardScheme, &h.ForwardHost, &h.ForwardPort, &ws, &cert, &ssl, &h2, &wr, &h.WaitroomPath, &h.AdvancedConfig, &en,
		&h.ClientIP, &rl, &al, &be, &h.BodyInspection, &h.BodyMaxKB, &h.SecurityHeaders, &h.UnderAttack, &uaCfg, &mu, &ut, &gz, &locs, &h.Notes, &h.CreatedAt, &h.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(domains), &h.Domains)
	_ = json.Unmarshal([]byte(uaCfg), &h.UnderAttackConfig)
	_ = json.Unmarshal([]byte(locs), &h.Locations)
	if h.Locations == nil {
		h.Locations = []Location{}
	}
	h.Websocket, h.SSLForced, h.HTTP2, h.Enabled = ws == 1, ssl == 1, h2 == 1, en == 1
	if cert.Valid {
		h.CertificateID = &cert.Int64
	}
	if wr.Valid {
		h.WaitroomID = &wr.Int64
	}
	if rl.Valid {
		h.RateLimitID = &rl.Int64
	}
	if al.Valid {
		h.AccessListID = &al.Int64
	}
	h.BlockExploits, h.Gzip = be == 1, gz == 1
	if mu.Valid {
		v := int(mu.Int64)
		h.MaxUploadMB = &v
	}
	if ut.Valid {
		v := int(ut.Int64)
		h.UpstreamTimeoutS = &v
	}
	return h, nil
}

func (s *Store) ListProxyHosts(ctx context.Context) ([]ProxyHost, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+phCols+` FROM proxy_hosts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProxyHost{}
	for rows.Next() {
		h, err := scanProxyHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

func (s *Store) ProxyHostByID(ctx context.Context, id int64) (*ProxyHost, error) {
	return scanProxyHost(s.q.QueryRowContext(ctx, `SELECT `+phCols+` FROM proxy_hosts WHERE id = ?`, id))
}

func (s *Store) SaveProxyHost(ctx context.Context, h *ProxyHost) error {
	t := now()
	if h.SecurityHeaders == "" {
		h.SecurityHeaders = "off"
	}
	if h.UnderAttack == "" {
		h.UnderAttack = "off"
	}
	uaCfg := toJSON(h.UnderAttackConfig)
	if h.Locations == nil {
		h.Locations = []Location{}
	}
	locs := toJSON(h.Locations)
	if h.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO proxy_hosts(domains, forward_scheme, forward_host, forward_port, websocket, certificate_id, ssl_forced, http2, waitroom_id, waitroom_path, advanced_config, enabled, client_ip, rate_limit_id, access_list_id, block_exploits, body_inspection, body_max_kb, security_headers, under_attack, under_attack_config, max_upload_mb, upstream_timeout_s, gzip, locations, notes, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			toJSON(h.Domains), h.ForwardScheme, h.ForwardHost, h.ForwardPort, b2i(h.Websocket), h.CertificateID, b2i(h.SSLForced), b2i(h.HTTP2),
			h.WaitroomID, h.WaitroomPath, h.AdvancedConfig, b2i(h.Enabled), h.ClientIP, h.RateLimitID, h.AccessListID, b2i(h.BlockExploits), h.BodyInspection, h.BodyMaxKB, h.SecurityHeaders, h.UnderAttack, uaCfg, h.MaxUploadMB, h.UpstreamTimeoutS, b2i(h.Gzip), locs, h.Notes, t, t)
		if err != nil {
			return err
		}
		h.ID, _ = res.LastInsertId()
		h.CreatedAt, h.UpdatedAt = t, t
		return nil
	}
	res, err := s.q.ExecContext(ctx, `UPDATE proxy_hosts SET domains=?, forward_scheme=?, forward_host=?, forward_port=?, websocket=?, certificate_id=?, ssl_forced=?, http2=?, waitroom_id=?, waitroom_path=?, advanced_config=?, enabled=?, client_ip=?, rate_limit_id=?, access_list_id=?, block_exploits=?, body_inspection=?, body_max_kb=?, security_headers=?, under_attack=?, under_attack_config=?, max_upload_mb=?, upstream_timeout_s=?, gzip=?, locations=?, notes=?, updated_at=? WHERE id=?`,
		toJSON(h.Domains), h.ForwardScheme, h.ForwardHost, h.ForwardPort, b2i(h.Websocket), h.CertificateID, b2i(h.SSLForced), b2i(h.HTTP2),
		h.WaitroomID, h.WaitroomPath, h.AdvancedConfig, b2i(h.Enabled), h.ClientIP, h.RateLimitID, h.AccessListID, b2i(h.BlockExploits), h.BodyInspection, h.BodyMaxKB, h.SecurityHeaders, h.UnderAttack, uaCfg, h.MaxUploadMB, h.UpstreamTimeoutS, b2i(h.Gzip), locs, h.Notes, t, h.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	h.UpdatedAt = t
	return nil
}

func (s *Store) DeleteProxyHost(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM proxy_hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ------------------------------------------------------------------ config versions

type ConfigVersion struct {
	ID         int64     `json:"id"`
	CreatedAt  time.Time `json:"created_at"`
	CreatedBy  *int64    `json:"created_by"`
	Email      string    `json:"email"`
	Status     string    `json:"status"`
	Message    string    `json:"message"`
	TestOutput string    `json:"test_output"`
	RollbackOf *int64    `json:"rollback_of"`
	Dir        string    `json:"dir"`
	ConfigRev  int64     `json:"config_rev"`
}

func (s *Store) CreateVersion(ctx context.Context, v *ConfigVersion) error {
	v.CreatedAt = now()
	if v.Status == "" {
		v.Status = "pending"
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO config_versions(created_at, created_by, status, message, test_output, rollback_of, dir, config_rev) VALUES (?,?,?,?,?,?,?,?)`,
		v.CreatedAt, v.CreatedBy, v.Status, v.Message, v.TestOutput, v.RollbackOf, v.Dir, v.ConfigRev)
	if err != nil {
		return err
	}
	v.ID, err = res.LastInsertId()
	return err
}

func (s *Store) SetVersionDir(ctx context.Context, id int64, dir string) error {
	_, err := s.q.ExecContext(ctx, `UPDATE config_versions SET dir = ? WHERE id = ?`, dir, id)
	return err
}

// FinishVersion records the outcome; on success every other active version becomes superseded.
func (s *Store) FinishVersion(ctx context.Context, id int64, ok bool, output string) error {
	return s.WithTx(ctx, func(tx *Store) error {
		status := "failed"
		if ok {
			status = "active"
			if _, err := tx.q.ExecContext(ctx, `UPDATE config_versions SET status='superseded' WHERE status='active' AND id <> ?`, id); err != nil {
				return err
			}
		}
		_, err := tx.q.ExecContext(ctx, `UPDATE config_versions SET status=?, test_output=? WHERE id=?`, status, output, id)
		return err
	})
}

const verCols = `v.id, v.created_at, v.created_by, COALESCE(u.email, ''), v.status, v.message, v.test_output, v.rollback_of, v.dir, v.config_rev`

func scanVersion(row interface{ Scan(...any) error }) (*ConfigVersion, error) {
	var v ConfigVersion
	var by, rb sql.NullInt64
	if err := row.Scan(&v.ID, &v.CreatedAt, &by, &v.Email, &v.Status, &v.Message, &v.TestOutput, &rb, &v.Dir, &v.ConfigRev); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if by.Valid {
		v.CreatedBy = &by.Int64
	}
	if rb.Valid {
		v.RollbackOf = &rb.Int64
	}
	return &v, nil
}

func (s *Store) ListVersions(ctx context.Context, limit, offset int) ([]ConfigVersion, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+verCols+` FROM config_versions v LEFT JOIN users u ON u.id = v.created_by ORDER BY v.id DESC LIMIT ? OFFSET ?`, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ConfigVersion{}
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

// CountVersions returns the number of recorded configuration versions (for paging).
func (s *Store) CountVersions(ctx context.Context) (int64, error) {
	var n int64
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM config_versions`).Scan(&n)
	return n, err
}

func (s *Store) VersionByID(ctx context.Context, id int64) (*ConfigVersion, error) {
	return scanVersion(s.q.QueryRowContext(ctx, `SELECT `+verCols+` FROM config_versions v LEFT JOIN users u ON u.id = v.created_by WHERE v.id = ?`, id))
}

func (s *Store) ActiveVersion(ctx context.Context) (*ConfigVersion, error) {
	return scanVersion(s.q.QueryRowContext(ctx, `SELECT `+verCols+` FROM config_versions v LEFT JOIN users u ON u.id = v.created_by WHERE v.status = 'active' ORDER BY v.id DESC LIMIT 1`))
}

// VersionDirsInUse returns the directories referenced by the newest n versions plus the active one.
func (s *Store) VersionDirsInUse(ctx context.Context, n int) (map[string]bool, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT dir FROM config_versions WHERE dir <> '' AND (status = 'active' OR id IN (SELECT id FROM config_versions WHERE status <> 'failed' ORDER BY id DESC LIMIT ?))`, n)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[string]bool{}
	for rows.Next() {
		var d string
		if err := rows.Scan(&d); err != nil {
			return nil, err
		}
		out[d] = true
	}
	return out, rows.Err()
}

// ------------------------------------------------------------------ audit

type AuditEntry struct {
	ID     int64           `json:"id"`
	TS     time.Time       `json:"ts"`
	UserID *int64          `json:"user_id"`
	Email  string          `json:"email"`
	Action string          `json:"action"`
	Target string          `json:"target"`
	Detail json.RawMessage `json:"detail"`
	IP     string          `json:"ip"`
}

// Size limits of an audit entry: longer targets are cut, larger details are replaced
// by a truncated preview (the stored detail stays valid JSON).
const (
	AuditTargetMax = 256
	AuditDetailMax = 32 << 10 // room for a whole host (create/update entries)
)

func (s *Store) Audit(ctx context.Context, userID *int64, action, target string, detail any, ip string) error {
	_, err := s.q.ExecContext(ctx, `INSERT INTO audit_log(ts, user_id, action, target, detail, ip) VALUES (?,?,?,?,?,?)`,
		now(), userID, action, truncUTF8(target, AuditTargetMax), auditJSON(detail), ip)
	return err
}

func auditJSON(detail any) string {
	j := toJSON(detail)
	if len(j) <= AuditDetailMax {
		return j
	}
	for n := AuditDetailMax / 2; ; n /= 2 { // escaping can grow the preview: shrink until it fits
		out := toJSON(map[string]any{"truncated": true, "size": len(j), "preview": truncUTF8(j, n)})
		if len(out) <= AuditDetailMax || n < 64 {
			return out
		}
	}
}

// truncUTF8 cuts s to at most max bytes without splitting a character ("…" marks a cut).
func truncUTF8(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := max - len("…")
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut] + "…"
}

// Audit log retention: entries older than AuditRetention are removed, and at most
// AuditMaxRows of the newest entries are kept.
const (
	AuditRetention = 365 * 24 * time.Hour
	AuditMaxRows   = 500000
)

// PruneAudit removes audit entries older than before and all but the newest keep
// entries; it returns the number of rows removed.
func (s *Store) PruneAudit(ctx context.Context, before time.Time, keep int64) (int64, error) {
	res, err := s.q.ExecContext(ctx, `DELETE FROM audit_log WHERE ts < ?`, before.UTC().Truncate(time.Second))
	if err != nil {
		return 0, err
	}
	n, _ := res.RowsAffected()
	var maxID sql.NullInt64
	if err := s.q.QueryRowContext(ctx, `SELECT MAX(id) FROM audit_log`).Scan(&maxID); err != nil {
		return n, err
	}
	if maxID.Valid && maxID.Int64 > keep {
		res, err := s.q.ExecContext(ctx, `DELETE FROM audit_log WHERE id <= ?`, maxID.Int64-keep)
		if err != nil {
			return n, err
		}
		m, _ := res.RowsAffected()
		n += m
	}
	return n, nil
}

func (s *Store) ListAudit(ctx context.Context, limit int) ([]AuditEntry, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT a.id, a.ts, a.user_id, COALESCE(u.email, ''), a.action, a.target, a.detail, a.ip
		FROM audit_log a LEFT JOIN users u ON u.id = a.user_id ORDER BY a.id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AuditEntry{}
	for rows.Next() {
		var e AuditEntry
		var uid sql.NullInt64
		var detail string
		if err := rows.Scan(&e.ID, &e.TS, &uid, &e.Email, &e.Action, &e.Target, &detail, &e.IP); err != nil {
			return nil, err
		}
		if uid.Valid {
			e.UserID = &uid.Int64
		}
		e.Detail = json.RawMessage(detail)
		out = append(out, e)
	}
	return out, rows.Err()
}
