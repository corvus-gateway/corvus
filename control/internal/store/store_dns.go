package store

import (
	"context"
	"fmt"
	"time"
)

// ------------------------------------------------------------------ saved DNS credentials

// DNSCredential is a saved DNS provider credential for DNS-01 certificates. Secret is
// the credential JSON encrypted with the data key and is never serialized.
type DNSCredential struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Provider  string    `json:"provider"`
	Secret    string    `json:"-"`
	UsedBy    int       `json:"used_by"` // certificates that reference it
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

func (s *Store) ListDNSCredentials(ctx context.Context) ([]DNSCredential, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT d.id, d.name, d.provider, d.secret, d.created_at, d.updated_at,
		(SELECT COUNT(*) FROM certificates c WHERE c.dns_credential_id = d.id) FROM dns_credentials d ORDER BY d.id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []DNSCredential{}
	for rows.Next() {
		var c DNSCredential
		if err := rows.Scan(&c.ID, &c.Name, &c.Provider, &c.Secret, &c.CreatedAt, &c.UpdatedAt, &c.UsedBy); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) DNSCredentialByID(ctx context.Context, id int64) (*DNSCredential, error) {
	list, err := s.ListDNSCredentials(ctx)
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

func (s *Store) CreateDNSCredential(ctx context.Context, c *DNSCredential) error {
	c.CreatedAt = now()
	c.UpdatedAt = c.CreatedAt
	res, err := s.q.ExecContext(ctx, `INSERT INTO dns_credentials(name, provider, secret, created_at, updated_at) VALUES (?,?,?,?,?)`,
		c.Name, c.Provider, c.Secret, c.CreatedAt, c.UpdatedAt)
	if err != nil {
		return err
	}
	c.ID, _ = res.LastInsertId()
	return nil
}

// UpdateDNSCredential saves name, provider and secret of an existing credential.
func (s *Store) UpdateDNSCredential(ctx context.Context, c *DNSCredential) error {
	c.UpdatedAt = now()
	res, err := s.q.ExecContext(ctx, `UPDATE dns_credentials SET name=?, provider=?, secret=?, updated_at=? WHERE id=?`,
		c.Name, c.Provider, c.Secret, c.UpdatedAt, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteDNSCredential removes a credential that no certificate uses.
func (s *Store) DeleteDNSCredential(ctx context.Context, id int64) error {
	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM certificates WHERE dns_credential_id = ?`, id).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return fmt.Errorf("DNS credential is used by %d certificate(s)", n)
	}
	res, err := s.q.ExecContext(ctx, `DELETE FROM dns_credentials WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetCertificateDNSCredential points a certificate at a saved credential and stores its
// ACME info (used to drop the legacy per-certificate secret once it has been moved).
func (s *Store) SetCertificateDNSCredential(ctx context.Context, certID, credID int64, info ACMEInfo) error {
	_, err := s.q.ExecContext(ctx, `UPDATE certificates SET dns_credential_id=?, acme=? WHERE id=?`, credID, toJSON(info), certID)
	return err
}
