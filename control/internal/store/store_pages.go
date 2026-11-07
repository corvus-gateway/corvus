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

// ------------------------------------------------------------------ unknown hosts

// Unknown host modes: what the HTTP default server answers for a host name that no
// proxy or redirection host serves.
const (
	UnknownNotFound = "not_found" // built-in 404 page (default)
	UnknownClose    = "close"     // close the connection without a response (nginx 444)
	UnknownRedirect = "redirect"  // 302 to RedirectURL
)

// UnknownHosts is the global unknown host setting.
type UnknownHosts struct {
	Mode        string `json:"mode"`
	RedirectURL string `json:"redirect_url"`
}

func (s *Store) UnknownHosts(ctx context.Context) (UnknownHosts, error) {
	u := UnknownHosts{Mode: UnknownNotFound}
	v, err := s.GetMeta(ctx, "unknown_hosts")
	if err != nil || v == "" {
		return u, err
	}
	err = json.Unmarshal([]byte(v), &u)
	if u.Mode == "" {
		u.Mode = UnknownNotFound
	}
	return u, err
}

func (s *Store) SetUnknownHosts(ctx context.Context, u UnknownHosts) error {
	return s.SetMeta(ctx, "unknown_hosts", toJSON(u))
}
