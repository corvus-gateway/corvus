package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"
)

// ------------------------------------------------------------------ security rules

// Condition is one test in a rule expression.
type Condition struct {
	Field    string   `json:"field"`
	Name     string   `json:"name,omitempty"` // header name for http.header
	Operator string   `json:"operator"`
	Value    []string `json:"value"`
	CI       bool     `json:"ci,omitempty"`
}

// Expression is in disjunctive normal form: any group whose conditions all match.
type Expression struct {
	Groups [][]Condition `json:"groups"`
}

type SecurityRule struct {
	ID            int64      `json:"id"`
	Name          string     `json:"name"`
	Description   string     `json:"description"`
	Enabled       bool       `json:"enabled"`
	Position      int        `json:"position"`
	Action        string     `json:"action"`
	StatusCode    int        `json:"status_code"`
	SkipBasicAuth bool       `json:"skip_basic_auth"`
	Scope         string     `json:"scope"`
	HostIDs       []int64    `json:"host_ids"`
	Expression    Expression `json:"expression"`
	Hits24h       int64      `json:"hits_24h"`
	LastHitAt     *time.Time `json:"last_hit_at"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

func (s *Store) ListSecurityRules(ctx context.Context) ([]SecurityRule, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, name, description, enabled, position, action, status_code, skip_basic_auth, scope, expression, created_at, updated_at
		FROM security_rules ORDER BY position, id`)
	if err != nil {
		return nil, err
	}
	out := []SecurityRule{}
	for rows.Next() {
		var r SecurityRule
		var en, skip int
		var expr string
		if err := rows.Scan(&r.ID, &r.Name, &r.Description, &en, &r.Position, &r.Action, &r.StatusCode, &skip, &r.Scope, &expr, &r.CreatedAt, &r.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		r.Enabled, r.SkipBasicAuth = en == 1, skip == 1
		_ = json.Unmarshal([]byte(expr), &r.Expression)
		if r.Expression.Groups == nil {
			r.Expression.Groups = [][]Condition{}
		}
		r.HostIDs = []int64{}
		out = append(out, r)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	hrows, err := s.q.QueryContext(ctx, `SELECT rule_id, host_id FROM security_rule_hosts ORDER BY host_id`)
	if err != nil {
		return nil, err
	}
	defer hrows.Close()
	idx := map[int64]int{}
	for i := range out {
		idx[out[i].ID] = i
	}
	for hrows.Next() {
		var rid, hid int64
		if err := hrows.Scan(&rid, &hid); err != nil {
			return nil, err
		}
		if i, ok := idx[rid]; ok {
			out[i].HostIDs = append(out[i].HostIDs, hid)
		}
	}
	return out, hrows.Err()
}

func (s *Store) SecurityRuleByID(ctx context.Context, id int64) (*SecurityRule, error) {
	list, err := s.ListSecurityRules(ctx)
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

// SaveSecurityRule inserts (at the bottom of the list) or updates a rule and its host scope.
func (s *Store) SaveSecurityRule(ctx context.Context, r *SecurityRule) error {
	return s.WithTx(ctx, func(tx *Store) error { return tx.saveSecurityRule(ctx, r) })
}

func (s *Store) saveSecurityRule(ctx context.Context, r *SecurityRule) error {
	t := now()
	if r.ID == 0 {
		var maxPos int
		if err := s.q.QueryRowContext(ctx, `SELECT COALESCE(MAX(position), 0) FROM security_rules`).Scan(&maxPos); err != nil {
			return err
		}
		r.Position = maxPos + 1
		res, err := s.q.ExecContext(ctx, `INSERT INTO security_rules(name, description, enabled, position, action, status_code, skip_basic_auth, scope, expression, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?)`, r.Name, r.Description, b2i(r.Enabled), r.Position, r.Action, r.StatusCode, b2i(r.SkipBasicAuth), r.Scope, toJSON(r.Expression), t, t)
		if err != nil {
			return err
		}
		r.ID, _ = res.LastInsertId()
		r.CreatedAt = t
	} else {
		res, err := s.q.ExecContext(ctx, `UPDATE security_rules SET name=?, description=?, enabled=?, action=?, status_code=?, skip_basic_auth=?, scope=?, expression=?, updated_at=? WHERE id=?`,
			r.Name, r.Description, b2i(r.Enabled), r.Action, r.StatusCode, b2i(r.SkipBasicAuth), r.Scope, toJSON(r.Expression), t, r.ID)
		if err != nil {
			return err
		}
		if n, _ := res.RowsAffected(); n == 0 {
			return ErrNotFound
		}
	}
	r.UpdatedAt = t
	if _, err := s.q.ExecContext(ctx, `DELETE FROM security_rule_hosts WHERE rule_id = ?`, r.ID); err != nil {
		return err
	}
	if r.Scope == "hosts" {
		for _, h := range r.HostIDs {
			if _, err := s.q.ExecContext(ctx, `INSERT INTO security_rule_hosts(rule_id, host_id) VALUES (?,?)`, r.ID, h); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) DeleteSecurityRule(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM security_rules WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ReorderSecurityRules sets positions 1..n in the given order; ids must be exactly all rules.
func (s *Store) ReorderSecurityRules(ctx context.Context, ids []int64) error {
	return s.WithTx(ctx, func(tx *Store) error {
		var n int
		if err := tx.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM security_rules`).Scan(&n); err != nil {
			return err
		}
		seen := map[int64]bool{}
		for _, id := range ids {
			seen[id] = true
		}
		if len(ids) != n || len(seen) != n {
			return fmt.Errorf("order must list every rule exactly once")
		}
		for i, id := range ids {
			res, err := tx.q.ExecContext(ctx, `UPDATE security_rules SET position = ? WHERE id = ?`, i+1, id)
			if err != nil {
				return err
			}
			if c, _ := res.RowsAffected(); c == 0 {
				return fmt.Errorf("unknown rule %d", id)
			}
		}
		return nil
	})
}

// ------------------------------------------------------------------ IP lists

// List kinds.
const (
	ListKindIP        = "ip"
	ListKindUserAgent = "user_agent"
)

// IPList is a named list referenced by security rules ("list:<id>"): IP addresses/CIDRs
// (kind "ip") or User-Agent substrings (kind "user_agent"). With SourceURL set, Items are
// fetched periodically by the feeds service and are read-only.
type IPList struct {
	ID           int64      `json:"id"`
	Name         string     `json:"name"`
	Description  string     `json:"description"`
	Kind         string     `json:"kind"`
	Items        []string   `json:"items"`
	SourceURL    string     `json:"source_url"`
	RefreshS     int        `json:"refresh_s"`
	FetchedAt    *time.Time `json:"fetched_at"`
	CheckedAt    *time.Time `json:"checked_at"`
	FetchError   string     `json:"fetch_error"`
	FetchWarning string     `json:"fetch_warning"`
	Rules        int        `json:"rules"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func (s *Store) ListIPLists(ctx context.Context) ([]IPList, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, name, description, kind, items, source_url, refresh_s, fetched_at, checked_at, fetch_error, fetch_warning, created_at, updated_at
		FROM ip_lists ORDER BY name`)
	if err != nil {
		return nil, err
	}
	out := []IPList{}
	for rows.Next() {
		var l IPList
		var items string
		var fetched, checked sql.NullTime
		if err := rows.Scan(&l.ID, &l.Name, &l.Description, &l.Kind, &items, &l.SourceURL, &l.RefreshS, &fetched, &checked, &l.FetchError, &l.FetchWarning, &l.CreatedAt, &l.UpdatedAt); err != nil {
			rows.Close()
			return nil, err
		}
		if fetched.Valid {
			t := fetched.Time
			l.FetchedAt = &t
		}
		if checked.Valid {
			t := checked.Time
			l.CheckedAt = &t
		}
		_ = json.Unmarshal([]byte(items), &l.Items)
		if l.Items == nil {
			l.Items = []string{}
		}
		out = append(out, l)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(out) > 0 {
		rules, err := s.ListSecurityRules(ctx)
		if err != nil {
			return nil, err
		}
		use := ListUsage(rules)
		for i := range out {
			out[i].Rules = use[out[i].ID]
		}
	}
	return out, nil
}

// ListUsage counts, per IP list id, how many rules reference it ("list:<id>").
func ListUsage(rules []SecurityRule) map[int64]int {
	use := map[int64]int{}
	for _, r := range rules {
		refs := map[int64]bool{}
		for _, g := range r.Expression.Groups {
			for _, c := range g {
				for _, v := range c.Value {
					if id, ok := ListRef(v); ok {
						refs[id] = true
					}
				}
			}
		}
		for id := range refs {
			use[id]++
		}
	}
	return use
}

// ListRef parses an IP list reference value "list:<id>".
func ListRef(v string) (int64, bool) {
	if !strings.HasPrefix(v, "list:") {
		return 0, false
	}
	id, err := strconv.ParseInt(v[5:], 10, 64)
	return id, err == nil && id > 0
}

func (s *Store) IPListByID(ctx context.Context, id int64) (*IPList, error) {
	list, err := s.ListIPLists(ctx)
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

// SaveIPList inserts or updates a list's definition and items (the fetch status is
// kept; see SetIPListFetch).
func (s *Store) SaveIPList(ctx context.Context, l *IPList) error {
	t := now()
	if l.Kind == "" {
		l.Kind = ListKindIP
	}
	if l.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO ip_lists(name, description, kind, items, source_url, refresh_s, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?)`,
			l.Name, l.Description, l.Kind, toJSON(l.Items), l.SourceURL, l.RefreshS, t, t)
		if err != nil {
			return err
		}
		l.ID, _ = res.LastInsertId()
		l.CreatedAt, l.UpdatedAt = t, t
		return nil
	}
	res, err := s.q.ExecContext(ctx, `UPDATE ip_lists SET name=?, description=?, kind=?, items=?, source_url=?, refresh_s=?, updated_at=? WHERE id=?`,
		l.Name, l.Description, l.Kind, toJSON(l.Items), l.SourceURL, l.RefreshS, t, l.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	l.UpdatedAt = t
	return nil
}

// ListFetch is the outcome of one feed fetch.
type ListFetch struct {
	Items     []string // new entries; nil keeps the stored ones
	CheckedAt time.Time
	FetchedAt *time.Time // set on success
	Error     string
	Warning   string
}

// SetIPListFetch records a feed fetch (and, when f.Items is set, the new entries).
// A failed fetch (FetchedAt nil) keeps the entries, the last success time and warning.
func (s *Store) SetIPListFetch(ctx context.Context, id int64, f ListFetch) error {
	var res sql.Result
	var err error
	switch {
	case f.Items != nil:
		res, err = s.q.ExecContext(ctx, `UPDATE ip_lists SET items=?, fetched_at=?, checked_at=?, fetch_error=?, fetch_warning=? WHERE id=?`,
			toJSON(f.Items), f.FetchedAt, f.CheckedAt, f.Error, f.Warning, id)
	case f.FetchedAt != nil:
		res, err = s.q.ExecContext(ctx, `UPDATE ip_lists SET fetched_at=?, checked_at=?, fetch_error=?, fetch_warning=? WHERE id=?`,
			f.FetchedAt, f.CheckedAt, f.Error, f.Warning, id)
	default:
		res, err = s.q.ExecContext(ctx, `UPDATE ip_lists SET checked_at=?, fetch_error=? WHERE id=?`, f.CheckedAt, f.Error, id)
	}
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ResetIPListFetch clears the fetch status (the feed URL changed or was removed).
func (s *Store) ResetIPListFetch(ctx context.Context, id int64) error {
	_, err := s.q.ExecContext(ctx, `UPDATE ip_lists SET fetched_at=NULL, checked_at=NULL, fetch_error='', fetch_warning='' WHERE id=?`, id)
	return err
}

func (s *Store) DeleteIPList(ctx context.Context, id int64) error {
	rules, err := s.ListSecurityRules(ctx)
	if err != nil {
		return err
	}
	if n := ListUsage(rules)[id]; n > 0 {
		return fmt.Errorf("list is used by %d security rule(s)", n)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM ip_lists WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// MigrateAccessListIPRules converts the IP allow/deny of NPM-style access lists into
// security rules scoped to the hosts that use each list, then clears allow/deny so
// access lists are Basic-auth only. Idempotent (meta flag); runs in one transaction.
func (s *Store) MigrateAccessListIPRules(ctx context.Context) (int, error) {
	if v, _ := s.GetMeta(ctx, "access_rules_migrated"); v == "1" {
		return 0, nil
	}
	created := 0
	err := s.WithTx(ctx, func(tx *Store) error {
		lists, err := tx.ListAccessLists(ctx)
		if err != nil {
			return err
		}
		for _, a := range lists {
			hostIDs, err := tx.hostsUsingAccessList(ctx, a.ID)
			if err != nil {
				return err
			}
			if len(hostIDs) > 0 {
				for _, r := range AccessListRules(a.Name, a.Satisfy, a.Allow, a.Deny, len(a.Users) > 0) {
					r.Scope, r.HostIDs = "hosts", hostIDs
					if err := tx.saveSecurityRule(ctx, &r); err != nil {
						return err
					}
					created++
				}
			}
			if len(a.Users) == 0 { // IP-only list: fully replaced by the rules above
				if _, err := tx.q.ExecContext(ctx, `UPDATE proxy_hosts SET access_list_id = NULL WHERE access_list_id = ?`, a.ID); err != nil {
					return err
				}
				if _, err := tx.q.ExecContext(ctx, `DELETE FROM access_lists WHERE id = ?`, a.ID); err != nil {
					return err
				}
				continue
			}
			if _, err := tx.q.ExecContext(ctx, `UPDATE access_lists SET allow = '[]', deny = '[]', satisfy = 'all' WHERE id = ?`, a.ID); err != nil {
				return err
			}
		}
		if _, err := tx.BumpConfigRev(ctx); err != nil { // re-render with the new rules
			return err
		}
		return tx.SetMeta(ctx, "access_rules_migrated", "1")
	})
	return created, err
}

func (s *Store) hostsUsingAccessList(ctx context.Context, id int64) ([]int64, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id FROM proxy_hosts WHERE access_list_id = ? ORDER BY id`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []int64
	for rows.Next() {
		var h int64
		if err := rows.Scan(&h); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}

// AccessListRules translates NPM access-list IP semantics (nginx allow/deny + satisfy)
// into security rules (scope left to the caller):
//   - allow IPs → allow rule; with "satisfy any" and users, it also skips Basic auth
//   - deny IPs ("all" = everyone) → block rule, except "satisfy any" with users,
//     where a password still grants access, so no block rule is generated.
func AccessListRules(name, satisfy string, allow, deny []string, hasUsers bool) []SecurityRule {
	var out []SecurityRule
	label := name + " (converted from access list)"
	if len(allow) > 0 {
		out = append(out, SecurityRule{Name: label + " – allow", Enabled: true, Action: "allow", StatusCode: 403,
			SkipBasicAuth: satisfy == "any" && hasUsers,
			Expression:    Expression{Groups: [][]Condition{{{Field: "ip.src", Operator: "in", Value: allow}}}}})
	}
	if len(deny) > 0 && !(satisfy == "any" && hasUsers) {
		var cidrs []string
		all := false
		for _, d := range deny {
			if d == "all" {
				all = true
			} else {
				cidrs = append(cidrs, d)
			}
		}
		cond := Condition{Field: "ip.src", Operator: "in", Value: cidrs}
		if all {
			cond = Condition{Field: "ip.src", Operator: "in", Value: []string{"0.0.0.0/0", "::/0"}}
		}
		out = append(out, SecurityRule{Name: label + " – block", Enabled: true, Action: "block", StatusCode: 403,
			Expression: Expression{Groups: [][]Condition{{cond}}}})
	}
	return out
}
