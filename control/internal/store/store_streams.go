package store

import (
	"context"
	"database/sql"
	"errors"
	"time"
)

// Stream forwards a TCP and/or UDP port to a forward host (NPM "stream").
type Stream struct {
	ID            int64     `json:"id"`
	IncomingPort  int       `json:"incoming_port"`
	TCP           bool      `json:"tcp_forwarding"`
	UDP           bool      `json:"udp_forwarding"`
	ForwardHost   string    `json:"forward_host"`
	ForwardPort   int       `json:"forward_port"`
	CertificateID *int64    `json:"certificate_id"` // TLS termination (TCP only); nil = plain
	Enabled       bool      `json:"enabled"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
}

const streamCols = `id, incoming_port, tcp_forwarding, udp_forwarding, forward_host, forward_port, certificate_id, enabled, created_at, updated_at`

func scanStream(row interface{ Scan(...any) error }) (*Stream, error) {
	st := &Stream{}
	var tcp, udp, en int
	var cert sql.NullInt64
	if err := row.Scan(&st.ID, &st.IncomingPort, &tcp, &udp, &st.ForwardHost, &st.ForwardPort, &cert, &en, &st.CreatedAt, &st.UpdatedAt); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	st.TCP, st.UDP, st.Enabled = tcp == 1, udp == 1, en == 1
	if cert.Valid {
		st.CertificateID = &cert.Int64
	}
	return st, nil
}

func (s *Store) ListStreams(ctx context.Context) ([]Stream, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+streamCols+` FROM streams ORDER BY incoming_port, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Stream{}
	for rows.Next() {
		st, err := scanStream(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *st)
	}
	return out, rows.Err()
}

func (s *Store) StreamByID(ctx context.Context, id int64) (*Stream, error) {
	return scanStream(s.q.QueryRowContext(ctx, `SELECT `+streamCols+` FROM streams WHERE id = ?`, id))
}

func (s *Store) SaveStream(ctx context.Context, st *Stream) error {
	t := now()
	if st.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO streams(incoming_port, tcp_forwarding, udp_forwarding, forward_host, forward_port, certificate_id, enabled, created_at, updated_at)
			VALUES (?,?,?,?,?,?,?,?,?)`,
			st.IncomingPort, b2i(st.TCP), b2i(st.UDP), st.ForwardHost, st.ForwardPort, st.CertificateID, b2i(st.Enabled), t, t)
		if err != nil {
			return err
		}
		st.ID, _ = res.LastInsertId()
		st.CreatedAt, st.UpdatedAt = t, t
		return nil
	}
	res, err := s.q.ExecContext(ctx, `UPDATE streams SET incoming_port=?, tcp_forwarding=?, udp_forwarding=?, forward_host=?, forward_port=?, certificate_id=?, enabled=?, updated_at=? WHERE id=?`,
		st.IncomingPort, b2i(st.TCP), b2i(st.UDP), st.ForwardHost, st.ForwardPort, st.CertificateID, b2i(st.Enabled), t, st.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	st.UpdatedAt = t
	return nil
}

func (s *Store) DeleteStream(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM streams WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
