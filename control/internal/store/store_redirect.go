package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// RedirectionHost answers its domains with a redirect to another domain (NPM "redirection host").
type RedirectionHost struct {
	ID             int64     `json:"id"`
	Domains        []string  `json:"domains"`
	ForwardScheme  string    `json:"forward_scheme"` // auto (keep the request's scheme) | http | https
	ForwardDomain  string    `json:"forward_domain"` // host[:port][/path]
	HTTPCode       int       `json:"http_code"`      // 301 | 302 | 303 | 307 | 308
	PreservePath   bool      `json:"preserve_path"`  // append the request URI (path + query)
	CertificateID  *int64    `json:"certificate_id"`
	SSLForced      bool      `json:"ssl_forced"`
	HTTP2          bool      `json:"http2"`
	BlockExploits  bool      `json:"block_exploits"`
	AdvancedConfig string    `json:"advanced_config"`
	Enabled        bool      `json:"enabled"`
	Notes          string    `json:"notes"` // free text for people; not used by the gateway
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

const rhCols = `id, domains, forward_scheme, forward_domain, http_code, preserve_path, certificate_id, ssl_forced, http2, block_exploits, advanced_config, enabled, notes, created_at, updated_at`

func scanRedirectionHost(row interface{ Scan(...any) error }) (*RedirectionHost, error) {
	h := &RedirectionHost{}
	var domains string
	var pp, ssl, h2, be, en int
	var cert sql.NullInt64
	if err := row.Scan(&h.ID, &domains, &h.ForwardScheme, &h.ForwardDomain, &h.HTTPCode, &pp, &cert, &ssl, &h2, &be, &h.AdvancedConfig, &en, &h.Notes, &h.CreatedAt, &h.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	_ = json.Unmarshal([]byte(domains), &h.Domains)
	h.PreservePath, h.SSLForced, h.HTTP2, h.BlockExploits, h.Enabled = pp == 1, ssl == 1, h2 == 1, be == 1, en == 1
	if cert.Valid {
		h.CertificateID = &cert.Int64
	}
	return h, nil
}

func (s *Store) ListRedirectionHosts(ctx context.Context) ([]RedirectionHost, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+rhCols+` FROM redirection_hosts ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RedirectionHost{}
	for rows.Next() {
		h, err := scanRedirectionHost(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *h)
	}
	return out, rows.Err()
}

func (s *Store) RedirectionHostByID(ctx context.Context, id int64) (*RedirectionHost, error) {
	return scanRedirectionHost(s.q.QueryRowContext(ctx, `SELECT `+rhCols+` FROM redirection_hosts WHERE id = ?`, id))
}

func (s *Store) SaveRedirectionHost(ctx context.Context, h *RedirectionHost) error {
	t := now()
	if h.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO redirection_hosts(domains, forward_scheme, forward_domain, http_code, preserve_path, certificate_id, ssl_forced, http2, block_exploits, advanced_config, enabled, notes, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?)`,
			toJSON(h.Domains), h.ForwardScheme, h.ForwardDomain, h.HTTPCode, b2i(h.PreservePath), h.CertificateID, b2i(h.SSLForced), b2i(h.HTTP2),
			b2i(h.BlockExploits), h.AdvancedConfig, b2i(h.Enabled), h.Notes, t, t)
		if err != nil {
			return err
		}
		h.ID, _ = res.LastInsertId()
		h.CreatedAt, h.UpdatedAt = t, t
		return nil
	}
	res, err := s.q.ExecContext(ctx, `UPDATE redirection_hosts SET domains=?, forward_scheme=?, forward_domain=?, http_code=?, preserve_path=?, certificate_id=?, ssl_forced=?, http2=?, block_exploits=?, advanced_config=?, enabled=?, notes=?, updated_at=? WHERE id=?`,
		toJSON(h.Domains), h.ForwardScheme, h.ForwardDomain, h.HTTPCode, b2i(h.PreservePath), h.CertificateID, b2i(h.SSLForced), b2i(h.HTTP2),
		b2i(h.BlockExploits), h.AdvancedConfig, b2i(h.Enabled), h.Notes, t, h.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	h.UpdatedAt = t
	return nil
}

func (s *Store) DeleteRedirectionHost(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM redirection_hosts WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
