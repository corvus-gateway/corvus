package store

import (
	"context"
	"encoding/json"
	"time"

	"corvus/control/internal/hints"
)

type AlertChannel struct {
	ID          int64     `json:"id"`
	Name        string    `json:"name"`
	Kind        string    `json:"kind"`
	Target      string    `json:"-"` // sealed JSON
	TargetHint  string    `json:"target_hint"`
	Enabled     bool      `json:"enabled"`
	MinSeverity string    `json:"min_severity"`
	Events      []string  `json:"events"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

func (s *Store) ListAlertChannels(ctx context.Context) ([]AlertChannel, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, name, kind, target, target_hint, enabled, min_severity, events, created_at, updated_at FROM alert_channels ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertChannel{}
	for rows.Next() {
		var c AlertChannel
		var en int
		var ev string
		if err := rows.Scan(&c.ID, &c.Name, &c.Kind, &c.Target, &c.TargetHint, &en, &c.MinSeverity, &ev, &c.CreatedAt, &c.UpdatedAt); err != nil {
			return nil, err
		}
		c.Enabled = en == 1
		_ = json.Unmarshal([]byte(ev), &c.Events)
		if c.Events == nil {
			c.Events = []string{}
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (s *Store) AlertChannelByID(ctx context.Context, id int64) (*AlertChannel, error) {
	list, err := s.ListAlertChannels(ctx)
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

func (s *Store) SaveAlertChannel(ctx context.Context, c *AlertChannel) error {
	t := now()
	if c.Events == nil {
		c.Events = []string{}
	}
	if c.ID == 0 {
		res, err := s.q.ExecContext(ctx, `INSERT INTO alert_channels(name, kind, target, target_hint, enabled, min_severity, events, created_at, updated_at) VALUES (?,?,?,?,?,?,?,?,?)`,
			c.Name, c.Kind, c.Target, c.TargetHint, b2i(c.Enabled), c.MinSeverity, toJSON(c.Events), t, t)
		if err != nil {
			return err
		}
		c.ID, _ = res.LastInsertId()
		c.CreatedAt, c.UpdatedAt = t, t
		return nil
	}
	res, err := s.q.ExecContext(ctx, `UPDATE alert_channels SET name=?, kind=?, target=?, target_hint=?, enabled=?, min_severity=?, events=?, updated_at=? WHERE id=?`,
		c.Name, c.Kind, c.Target, c.TargetHint, b2i(c.Enabled), c.MinSeverity, toJSON(c.Events), t, c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	c.UpdatedAt = t
	return nil
}

func (s *Store) DeleteAlertChannel(ctx context.Context, id int64) error {
	res, err := s.q.ExecContext(ctx, `DELETE FROM alert_channels WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeliveryResult records one channel's delivery outcome.
type DeliveryResult struct {
	ChannelID int64  `json:"channel_id"`
	Channel   string `json:"channel"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	// Hint classifies Error (internal/hints); computed when read, not stored.
	Hint string `json:"hint,omitempty"`
}

type AlertRecord struct {
	ID       int64            `json:"id"`
	TS       time.Time        `json:"ts"`
	Rule     string           `json:"rule"`
	Severity string           `json:"severity"`
	Status   string           `json:"status"`
	Key      string           `json:"key"`
	Title    string           `json:"title"`
	Message  string           `json:"message"`
	Results  []DeliveryResult `json:"results"`
}

func (s *Store) AddAlert(ctx context.Context, a *AlertRecord) error {
	a.TS = now()
	if a.Results == nil {
		a.Results = []DeliveryResult{}
	}
	res, err := s.q.ExecContext(ctx, `INSERT INTO alert_history(ts, rule, severity, status, alert_key, title, message, results) VALUES (?,?,?,?,?,?,?,?)`,
		a.TS, a.Rule, a.Severity, a.Status, a.Key, a.Title, a.Message, toJSON(a.Results))
	if err != nil {
		return err
	}
	a.ID, _ = res.LastInsertId()
	// keep the history bounded
	_, _ = s.q.ExecContext(ctx, `DELETE FROM alert_history WHERE id <= ?`, a.ID-5000)
	return nil
}

func (s *Store) ListAlerts(ctx context.Context, limit int) ([]AlertRecord, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT id, ts, rule, severity, status, alert_key, title, message, results FROM alert_history ORDER BY id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AlertRecord{}
	for rows.Next() {
		var a AlertRecord
		var res string
		if err := rows.Scan(&a.ID, &a.TS, &a.Rule, &a.Severity, &a.Status, &a.Key, &a.Title, &a.Message, &res); err != nil {
			return nil, err
		}
		_ = json.Unmarshal([]byte(res), &a.Results)
		for i := range a.Results {
			a.Results[i].Hint = hints.Net(a.Results[i].Error)
		}
		out = append(out, a)
	}
	return out, rows.Err()
}

// CountAudit counts audit entries with action since t.
func (s *Store) CountAudit(ctx context.Context, action string, since time.Time) (int, error) {
	var n int
	err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM audit_log WHERE action = ? AND ts >= ?`, action, since.UTC().Truncate(time.Second)).Scan(&n)
	return n, err
}

// FailedVersionsSince returns rejected configuration versions created after id.
func (s *Store) FailedVersionsSince(ctx context.Context, afterID int64) ([]ConfigVersion, error) {
	rows, err := s.q.QueryContext(ctx, `SELECT `+verCols+` FROM config_versions v LEFT JOIN users u ON u.id = v.created_by WHERE v.status = 'failed' AND v.id > ? ORDER BY v.id`, afterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ConfigVersion
	for rows.Next() {
		v, err := scanVersion(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *v)
	}
	return out, rows.Err()
}

func (s *Store) MaxVersionID(ctx context.Context) (int64, error) {
	var id int64
	err := s.q.QueryRowContext(ctx, `SELECT COALESCE(MAX(id), 0) FROM config_versions`).Scan(&id)
	return id, err
}
