package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// Sign-in users and the login page in front of proxy hosts (#294). They are separate
// from the users of the admin console: they can only sign in to the sites whose login
// page lets their groups in.

// Login modes of a proxy host (HostLogin.Mode).
const (
	LoginPIN   = "pin"   // one shared PIN or password per host, no accounts
	LoginUsers = "users" // sign-in users of the allowed groups
)

// HostLogin is the login page configuration of a proxy host. The PIN hash and its
// version are stored but never serialized: the API only reports PINSet.
type HostLogin struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode"`          // pin | users
	GroupIDs     []int64  `json:"group_ids"`     // users mode: groups allowed in
	SessionHours int      `json:"session_hours"` // how long a sign-in lasts
	ExcludePaths []string `json:"exclude_paths"` // path prefixes that need no sign-in
	PassUser     bool     `json:"pass_user"`     // send X-Corvus-User to the app (users mode)
	PINSet       bool     `json:"pin_set"`       // read-only: a PIN/password is configured

	PINHash    string `json:"-"` // bcrypt of the shared PIN/password
	PINVersion int    `json:"-"` // bumped when the PIN changes; ends PIN sessions
}

// IsZero reports whether no login page was ever configured.
func (l HostLogin) IsZero() bool {
	return !l.Enabled && l.Mode == "" && len(l.GroupIDs) == 0 && l.SessionHours == 0 &&
		len(l.ExcludePaths) == 0 && !l.PassUser && l.PINHash == "" && l.PINVersion == 0
}

// hostLoginStored is HostLogin as kept in proxy_hosts.login, including the secrets.
type hostLoginStored struct {
	Enabled      bool     `json:"enabled,omitempty"`
	Mode         string   `json:"mode,omitempty"`
	GroupIDs     []int64  `json:"group_ids,omitempty"`
	SessionHours int      `json:"session_hours,omitempty"`
	ExcludePaths []string `json:"exclude_paths,omitempty"`
	PassUser     bool     `json:"pass_user,omitempty"`
	PINHash      string   `json:"pin_hash,omitempty"`
	PINVersion   int      `json:"pin_version,omitempty"`
}

func loginToJSON(l HostLogin) string {
	if l.IsZero() {
		return "{}"
	}
	return toJSON(hostLoginStored{l.Enabled, l.Mode, l.GroupIDs, l.SessionHours, l.ExcludePaths, l.PassUser, l.PINHash, l.PINVersion})
}

func loginFromJSON(s string) HostLogin {
	var st hostLoginStored
	_ = json.Unmarshal([]byte(s), &st)
	l := HostLogin{Enabled: st.Enabled, Mode: st.Mode, GroupIDs: st.GroupIDs, SessionHours: st.SessionHours,
		ExcludePaths: st.ExcludePaths, PassUser: st.PassUser, PINHash: st.PINHash, PINVersion: st.PINVersion}
	if l.GroupIDs == nil {
		l.GroupIDs = []int64{}
	}
	if l.ExcludePaths == nil {
		l.ExcludePaths = []string{}
	}
	l.PINSet = l.PINHash != ""
	return l
}

// SigninUser is a person who can sign in to the login page of proxy hosts.
type SigninUser struct {
	ID             int64      `json:"id"`
	Username       string     `json:"username"`
	TOTPRequired   bool       `json:"totp_required"`
	TOTPEnrolled   bool       `json:"totp_enrolled"` // read-only
	Enabled        bool       `json:"enabled"`
	Note           string     `json:"note"`
	GroupIDs       []int64    `json:"group_ids"`
	LastLoginAt    *time.Time `json:"last_login_at"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
	SessionVersion int        `json:"-"`
	PasswordHash   string     `json:"-"`
	TOTPSecret     string     `json:"-"` // encrypted with the data key
}

// SigninGroup is a named set of sign-in users; proxy hosts allow groups.
type SigninGroup struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	UserIDs   []int64   `json:"user_ids"`
	Hosts     int       `json:"hosts"` // read-only: proxy hosts that allow the group
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

const suCols = `id, username, password_hash, totp_required, totp_secret, session_version, enabled, note, last_login_at, created_at, updated_at`

func scanSigninUser(row interface{ Scan(...any) error }) (*SigninUser, error) {
	u := &SigninUser{}
	var req, en int
	var last sql.NullTime
	if err := row.Scan(&u.ID, &u.Username, &u.PasswordHash, &req, &u.TOTPSecret, &u.SessionVersion, &en, &u.Note, &last, &u.CreatedAt, &u.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	u.TOTPRequired, u.Enabled, u.TOTPEnrolled = req == 1, en == 1, u.TOTPSecret != ""
	if last.Valid {
		t := last.Time
		u.LastLoginAt = &t
	}
	u.GroupIDs = []int64{}
	return u, nil
}

// memberships returns user ID → group IDs.
func (s *Store) signinMemberships(ctx context.Context) (map[int64][]int64, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT group_id, user_id FROM signin_group_members ORDER BY group_id, user_id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[int64][]int64{}
	for rows.Next() {
		var g, u int64
		if err := rows.Scan(&g, &u); err != nil {
			return nil, err
		}
		out[u] = append(out[u], g)
	}
	return out, rows.Err()
}

func (s *Store) ListSigninUsers(ctx context.Context) ([]SigninUser, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+suCols+` FROM signin_users ORDER BY username`)
	if err != nil {
		return nil, err
	}
	out := []SigninUser{}
	for rows.Next() {
		u, err := scanSigninUser(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, *u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	m, err := s.signinMemberships(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		if g := m[out[i].ID]; g != nil {
			out[i].GroupIDs = g
		}
	}
	return out, nil
}

func (s *Store) signinUserWhere(ctx context.Context, where string, arg any) (*SigninUser, error) {
	u, err := scanSigninUser(s.q.QueryRowContext(ctx, `SELECT `+suCols+` FROM signin_users WHERE `+where, arg))
	if err != nil {
		return nil, err
	}
	m, err := s.signinMemberships(ctx)
	if err != nil {
		return nil, err
	}
	if g := m[u.ID]; g != nil {
		u.GroupIDs = g
	}
	return u, nil
}

func (s *Store) SigninUserByID(ctx context.Context, id int64) (*SigninUser, error) {
	return s.signinUserWhere(ctx, "id = ?", id)
}

// SigninUserByName looks a user up by name (case-insensitive: names are stored lower-case).
func (s *Store) SigninUserByName(ctx context.Context, name string) (*SigninUser, error) {
	return s.signinUserWhere(ctx, "username = ?", strings.ToLower(name))
}

// SaveSigninUser creates or updates a user and replaces its group memberships. The
// password hash, TOTP secret and session version are written as given.
func (s *Store) SaveSigninUser(ctx context.Context, u *SigninUser) error {
	t := now()
	u.Username = strings.ToLower(u.Username)
	if u.SessionVersion == 0 {
		u.SessionVersion = 1
	}
	if u.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO signin_users(username, password_hash, totp_required, totp_secret, session_version, enabled, note, last_login_at, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?)`, u.Username, u.PasswordHash, b2i(u.TOTPRequired), u.TOTPSecret, u.SessionVersion, b2i(u.Enabled), u.Note, u.LastLoginAt, t, t)
		if err != nil {
			return err
		}
		u.ID, _ = res.LastInsertId()
		u.CreatedAt = t
	} else {
		res, err := s.q.ExecContext(ctx, `UPDATE signin_users SET username=?, password_hash=?, totp_required=?, totp_secret=?, session_version=?, enabled=?, note=?, updated_at=? WHERE id=?`,
			u.Username, u.PasswordHash, b2i(u.TOTPRequired), u.TOTPSecret, u.SessionVersion, b2i(u.Enabled), u.Note, t, u.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	u.UpdatedAt = t
	u.TOTPEnrolled = u.TOTPSecret != ""
	if _, err := s.q.ExecContext(ctx, `DELETE FROM signin_group_members WHERE user_id = ?`, u.ID); err != nil {
		return err
	}
	if u.GroupIDs == nil {
		u.GroupIDs = []int64{}
	}
	for _, g := range u.GroupIDs {
		if _, err := s.q.ExecContext(ctx, `INSERT INTO signin_group_members(group_id, user_id) VALUES (?,?)`, g, u.ID); err != nil {
			return err
		}
	}
	return nil
}

// TouchSigninUser records a successful sign-in.
func (s *Store) TouchSigninUser(ctx context.Context, id int64) error {
	_, err := s.q.ExecContext(ctx, `UPDATE signin_users SET last_login_at = ? WHERE id = ?`, now(), id)
	return err
}

func (s *Store) DeleteSigninUser(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM signin_users WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.q.ExecContext(ctx, `DELETE FROM signin_group_members WHERE user_id = ?`, id)
	return err
}

// ------------------------------------------------------------------ groups

func (s *Store) ListSigninGroups(ctx context.Context) ([]SigninGroup, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, name, created_at, updated_at FROM signin_groups ORDER BY name`)
	if err != nil {
		return nil, err
	}
	out := []SigninGroup{}
	for rows.Next() {
		var g SigninGroup
		if err := rows.Scan(&g.ID, &g.Name, &g.CreatedAt, &g.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		g.UserIDs = []int64{}
		out = append(out, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	m, err := s.signinMemberships(ctx)
	if err != nil {
		return nil, err
	}
	used, err := s.signinGroupUse(ctx)
	if err != nil {
		return nil, err
	}
	for i := range out {
		for uid, gs := range m {
			for _, g := range gs {
				if g == out[i].ID {
					out[i].UserIDs = append(out[i].UserIDs, uid)
				}
			}
		}
		sortIDs(out[i].UserIDs)
		out[i].Hosts = used[out[i].ID]
	}
	return out, nil
}

// signinGroupUse counts, per group, the proxy hosts whose login page allows it.
func (s *Store) signinGroupUse(ctx context.Context) (map[int64]int, error) {
	hosts, err := s.ListProxyHosts(ctx)
	if err != nil {
		return nil, err
	}
	out := map[int64]int{}
	for _, h := range hosts {
		for _, g := range h.Login.GroupIDs {
			out[g]++
		}
	}
	return out, nil
}

func (s *Store) SigninGroupByID(ctx context.Context, id int64) (*SigninGroup, error) {
	list, err := s.ListSigninGroups(ctx)
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

// SaveSigninGroup creates or renames a group and replaces its members.
func (s *Store) SaveSigninGroup(ctx context.Context, g *SigninGroup) error {
	t := now()
	if g.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO signin_groups(name, created_at, updated_at) VALUES (?,?,?)`, g.Name, t, t)
		if err != nil {
			return err
		}
		g.ID, _ = res.LastInsertId()
		g.CreatedAt = t
	} else {
		res, err := s.q.ExecContext(ctx, `UPDATE signin_groups SET name=?, updated_at=? WHERE id=?`, g.Name, t, g.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	g.UpdatedAt = t
	if _, err := s.q.ExecContext(ctx, `DELETE FROM signin_group_members WHERE group_id = ?`, g.ID); err != nil {
		return err
	}
	if g.UserIDs == nil {
		g.UserIDs = []int64{}
	}
	for _, u := range g.UserIDs {
		if _, err := s.q.ExecContext(ctx, `INSERT INTO signin_group_members(group_id, user_id) VALUES (?,?)`, g.ID, u); err != nil {
			return err
		}
	}
	return nil
}

// DeleteSigninGroup deletes a group that no proxy host uses.
func (s *Store) DeleteSigninGroup(ctx context.Context, id int64) error {
	used, err := s.signinGroupUse(ctx)
	if err != nil {
		return err
	}
	if n := used[id]; n > 0 {
		return fmt.Errorf("the group is allowed on %d proxy host(s)", n)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM signin_groups WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	_, err = s.q.ExecContext(ctx, `DELETE FROM signin_group_members WHERE group_id = ?`, id)
	return err
}

func sortIDs(ids []int64) {
	for i := 1; i < len(ids); i++ {
		for j := i; j > 0 && ids[j] < ids[j-1]; j-- {
			ids[j], ids[j-1] = ids[j-1], ids[j]
		}
	}
}
