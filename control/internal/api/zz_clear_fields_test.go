package api

import (
	"fmt"
	"testing"
)

// Optional values can be cleared from the edit forms: a user's name, a Mattermost
// channel's display name and channel, and a webhook's signing secret.
func TestClearOptionalFields(t *testing.T) {
	e := newFullEnv(t)

	// user name: omitted keeps it, "" clears it
	m := e.must(201, "POST", "/api/users", map[string]any{"email": "op@example.com", "name": "Ops", "role": "operator", "password": "Op-passw0rd-long"})
	uid := id(m)
	if uid == 0 {
		uid = id(m["user"].(map[string]any))
	}
	path := fmt.Sprintf("/api/users/%d", uid)
	if m = e.must(200, "PUT", path, map[string]any{"role": "viewer"}); m["name"] != "Ops" {
		t.Fatalf("name kept: %v", m)
	}
	if m = e.must(200, "PUT", path, map[string]any{"name": ""}); m["name"] != "" {
		t.Fatalf("name cleared: %v", m)
	}
	e.must(422, "PUT", path, map[string]any{"name": string(make([]byte, 65))})

	// Mattermost: username and channel are returned (not secret) and can be cleared
	m = e.must(201, "POST", "/api/alerts/channels", map[string]any{"name": "mm", "kind": "mattermost",
		"url": "https://chat.example.com/hooks/abcd", "username": "Corvus", "channel": "alerts"})
	ch := m["channel"].(map[string]any)
	if ch["username"] != "Corvus" || ch["channel"] != "alerts" {
		t.Fatalf("mattermost view: %v", ch)
	}
	mm := id(ch)
	m = e.must(200, "PUT", fmt.Sprintf("/api/alerts/channels/%d", mm), map[string]any{"name": "mm", "username": "", "channel": ""})
	if tg := zsTarget(t, e, mm); tg.Username != "" || tg.Channel != "" || tg.URL != "https://chat.example.com/hooks/abcd" {
		t.Fatalf("cleared mattermost target: %+v", tg)
	}

	// webhook: the secret is never returned, only whether there is one; clear_secret removes it
	m = e.must(201, "POST", "/api/alerts/channels", map[string]any{"name": "wh", "kind": "webhook",
		"url": "https://hooks.example.com/x", "secret": "s3cret"})
	ch = m["channel"].(map[string]any)
	if ch["has_secret"] != true || ch["secret"] != nil {
		t.Fatalf("webhook view: %v", ch)
	}
	wh := id(ch)
	e.must(200, "PUT", fmt.Sprintf("/api/alerts/channels/%d", wh), map[string]any{"name": "wh"})
	if zsTarget(t, e, wh).Secret != "s3cret" {
		t.Fatal("secret kept when omitted")
	}
	m = e.must(200, "PUT", fmt.Sprintf("/api/alerts/channels/%d", wh), map[string]any{"name": "wh", "clear_secret": true})
	if tg := zsTarget(t, e, wh); tg.Secret != "" || tg.URL != "https://hooks.example.com/x" {
		t.Fatalf("secret cleared: %+v", tg)
	}
	if ch = m["channel"].(map[string]any); ch["has_secret"] != nil {
		t.Fatalf("view after clear: %v", ch)
	}
	for _, c := range e.zsList("/api/alerts/channels") {
		if c["name"] == "wh" && c["has_secret"] != nil {
			t.Fatalf("list: %v", c)
		}
	}
}
