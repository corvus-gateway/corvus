package store

import (
	"context"
	"encoding/json"
)

// ------------------------------------------------------------------ custom block / error pages

// CustomPages are operator-supplied HTML pages served by the gateway: the block page
// (security rules, bans, attack patterns) and the upstream error page (502/503/504).
// An empty value means the built-in page.
type CustomPages struct {
	Block string `json:"block,omitempty"`
	Error string `json:"error,omitempty"`
}

func (s *Store) CustomPages(ctx context.Context) (CustomPages, error) {
	var p CustomPages
	v, err := s.GetMeta(ctx, "custom_pages")
	if err != nil || v == "" {
		return p, err
	}
	err = json.Unmarshal([]byte(v), &p)
	return p, err
}

func (s *Store) SetCustomPages(ctx context.Context, p CustomPages) error {
	return s.SetMeta(ctx, "custom_pages", toJSON(p))
}
