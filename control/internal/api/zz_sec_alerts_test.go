package api

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"corvus/control/internal/alerts"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

func zsTarget(t *testing.T, e *fullEnv, chID int64) alerts.Target {
	t.Helper()
	c, err := e.s.Store.AlertChannelByID(context.Background(), chID)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := seal.Open(e.s.DataKey, c.Target)
	if err != nil {
		t.Fatal(err)
	}
	var tg alerts.Target
	if err := json.Unmarshal([]byte(raw), &tg); err != nil {
		t.Fatal(err)
	}
	return tg
}

func TestSecAlertChannels(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")
	e.s.Alerts = &alerts.Engine{Store: e.s.Store, Key: e.s.DataKey}
	e.h = e.s.Handler()

	// validation (all errors are reported together)
	m := e.must(422, "POST", "/api/alerts/channels", map[string]any{"name": "", "kind": "pager", "min_severity": "loud", "events": []string{"nope"}})
	fields := map[string]bool{}
	for _, f := range m["fields"].([]any) {
		fields[f.(map[string]any)["field"].(string)] = true
	}
	for _, f := range []string{"name", "kind", "min_severity", "events"} {
		if !fields[f] {
			t.Errorf("missing error for %s: %v", f, m)
		}
	}
	for _, c := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"name": "s", "kind": "slack", "url": "ftp://x"}, "url"},
		{map[string]any{"name": "s", "kind": "slack", "url": "http://127.0.0.1/hook"}, "url"},
		{map[string]any{"name": "s", "kind": "slack", "url": "https://u:p@hooks.example.com/x"}, "url"},
		{map[string]any{"name": "t", "kind": "telegram", "bot_token": "abc"}, "bot_token"},
		{map[string]any{"name": strings.Repeat("n", 65), "kind": "slack", "url": "https://hooks.example.com/x"}, "name"},
	} {
		m := e.must(422, "POST", "/api/alerts/channels", c.body)
		if zsField(m) != c.field {
			t.Errorf("%v: field %q (%v)", c.body, zsField(m), m)
		}
	}
	if w, _ := e.zsAs(viewer, "POST", "/api/alerts/channels", map[string]any{"name": "v", "kind": "slack", "url": "https://hooks.example.com/x"}); w.Code != 403 {
		t.Fatalf("viewer create: %d", w.Code)
	}
	e.must(400, "POST", "/api/alerts/channels", map[string]any{"name": "x", "color": "red"})

	// create a webhook with a secret (operator)
	w, m := e.zsAs(op, "POST", "/api/alerts/channels", map[string]any{"name": " ops ", "kind": "webhook",
		"url": "https://hooks.example.com/services/abcd1234", "secret": "s3cret", "events": []string{"error_rate", "deploy_failed"},
		"bot_token": "ignored", "username": "ignored"})
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	ch := zsObj(t, m, "channel")
	chID := id(ch)
	if ch["name"] != "ops" || ch["min_severity"] != "warning" || ch["enabled"] != true || len(ch["events"].([]any)) != 2 ||
		ch["target_hint"] != "hooks.example.com/…/1234" {
		t.Fatalf("created: %v", ch)
	}
	if strings.Contains(w.Body.String(), "s3cret") || strings.Contains(w.Body.String(), "abcd1234\"") {
		t.Fatalf("secret leaked: %s", w.Body)
	}
	if tg := zsTarget(t, e, chID); tg.Secret != "s3cret" || tg.BotToken != "" || tg.Username != "" {
		t.Fatalf("target: %+v", tg)
	}
	m = e.must(422, "POST", "/api/alerts/channels", map[string]any{"name": "ops", "kind": "slack", "url": "https://hooks.example.com/x"})
	if zsCode(m) != "duplicate" {
		t.Fatalf("duplicate: %v", m)
	}

	// a telegram channel
	m = e.must(201, "POST", "/api/alerts/channels", map[string]any{"name": "tg", "kind": "telegram", "bot_token": "123:ABC",
		"chat_id": "-100", "min_severity": "critical", "enabled": false, "url": "https://ignored.example.com"})
	tgID := id(zsObj(t, m, "channel"))
	if c := zsObj(t, m, "channel"); c["target_hint"] != "chat -100" || c["enabled"] != false {
		t.Fatalf("telegram: %v", c)
	}
	if tg := zsTarget(t, e, tgID); tg.URL != "" || tg.BotToken != "123:ABC" {
		t.Fatalf("telegram target: %+v", tg)
	}

	// list (viewer)
	w, _ = e.zsAs(viewer, "GET", "/api/alerts/channels", nil)
	var list []map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &list) != nil || len(list) != 2 || strings.Contains(w.Body.String(), "123:ABC") {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}

	// update without target fields keeps the secret and URL
	m = e.must(200, "PUT", fmt.Sprintf("/api/alerts/channels/%d", chID), map[string]any{"name": "ops2", "enabled": false, "min_severity": "info"})
	if c := zsObj(t, m, "channel"); c["name"] != "ops2" || c["enabled"] != false || c["min_severity"] != "info" || c["kind"] != "webhook" {
		t.Fatalf("updated: %v", c)
	}
	if tg := zsTarget(t, e, chID); tg.Secret != "s3cret" || tg.URL != "https://hooks.example.com/services/abcd1234" {
		t.Fatalf("target after update: %+v", tg)
	}
	// changing the kind needs a new destination
	m = e.must(422, "PUT", fmt.Sprintf("/api/alerts/channels/%d", chID), map[string]any{"name": "ops2", "kind": "discord"})
	if zsField(m) != "url" {
		t.Fatalf("kind change without url: %v", m)
	}
	m = e.must(200, "PUT", fmt.Sprintf("/api/alerts/channels/%d", chID), map[string]any{"name": "ops2", "kind": "mattermost",
		"url": "https://chat.example.com/hooks/zzzz", "username": " Corvus ", "channel": "alerts"})
	if tg := zsTarget(t, e, chID); tg.Secret != "" || tg.Username != "Corvus" || tg.Channel != "alerts" {
		t.Fatalf("mattermost target: %+v", tg)
	}
	m = e.must(422, "PUT", fmt.Sprintf("/api/alerts/channels/%d", chID), map[string]any{"name": "tg"})
	if zsCode(m) != "duplicate" {
		t.Fatalf("duplicate rename: %v", m)
	}
	e.must(422, "PUT", fmt.Sprintf("/api/alerts/channels/%d", chID), map[string]any{"name": ""})
	e.must(404, "PUT", "/api/alerts/channels/999", map[string]any{"name": "x"})
	e.must(400, "PUT", "/api/alerts/channels/x", map[string]any{"name": "x"})
	e.must(400, "PUT", fmt.Sprintf("/api/alerts/channels/%d", chID), map[string]any{"zzz": 1})

	// delete
	if w, _ := e.zsAs(viewer, "DELETE", fmt.Sprintf("/api/alerts/channels/%d", tgID), nil); w.Code != 403 {
		t.Fatalf("viewer delete: %d", w.Code)
	}
	m = e.must(200, "DELETE", fmt.Sprintf("/api/alerts/channels/%d", tgID), nil)
	if m["status"] != "ok" {
		t.Fatalf("delete: %v", m)
	}
	e.must(404, "DELETE", fmt.Sprintf("/api/alerts/channels/%d", tgID), nil)
	e.must(400, "DELETE", "/api/alerts/channels/0", nil)

	a := zsAudit(t, e)
	for act, n := range map[string]int{"alert_channel.create": 2, "alert_channel.update": 2, "alert_channel.delete": 1} {
		if zsCount(a, act) != n {
			t.Errorf("audit %s: %d", act, zsCount(a, act))
		}
	}
}

func TestSecAlertChannelTestAndHistory(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	e.s.Alerts = &alerts.Engine{Store: e.s.Store, Key: e.s.DataKey}
	e.h = e.s.Handler()
	ctx := context.Background()

	// A channel pointing at the host itself (stored directly: the API refuses such URLs).
	// Delivery is refused before any connection is made.
	sealed, err := seal.Seal(e.s.DataKey, `{"url":"http://127.0.0.1:9/hook"}`)
	if err != nil {
		t.Fatal(err)
	}
	c := &store.AlertChannel{Name: "local", Kind: "webhook", Target: sealed, Enabled: true, MinSeverity: "info"}
	if err := e.s.Store.SaveAlertChannel(ctx, c); err != nil {
		t.Fatal(err)
	}
	// and one whose target cannot be unsealed
	bad := &store.AlertChannel{Name: "broken", Kind: "slack", Target: "garbage", Enabled: true, MinSeverity: "info"}
	if err := e.s.Store.SaveAlertChannel(ctx, bad); err != nil {
		t.Fatal(err)
	}

	if w, _ := e.zsAs(viewer, "POST", fmt.Sprintf("/api/alerts/channels/%d/test", c.ID), nil); w.Code != 403 {
		t.Fatalf("viewer test: %d", w.Code)
	}
	m := e.must(200, "POST", fmt.Sprintf("/api/alerts/channels/%d/test", c.ID), nil)
	if m["ok"] != false || !strings.Contains(m["error"].(string), "not allowed") {
		t.Fatalf("test: %v", m)
	}
	m = e.must(200, "POST", fmt.Sprintf("/api/alerts/channels/%d/test", bad.ID), nil)
	if m["ok"] != false || m["error"] == "" {
		t.Fatalf("test broken: %v", m)
	}
	e.must(404, "POST", "/api/alerts/channels/999/test", nil)
	e.must(400, "POST", "/api/alerts/channels/abc/test", nil)

	// the tests are in the history (newest first), limit is honoured and bounded
	w, _ := e.zsAs(viewer, "GET", "/api/alerts/history", nil)
	var hist []map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &hist) != nil || len(hist) != 2 {
		t.Fatalf("history: %d %s", w.Code, w.Body)
	}
	if hist[0]["rule"] != "test" || hist[1]["results"].([]any)[0].(map[string]any)["channel"] != "local" {
		t.Fatalf("history: %v", hist)
	}
	if h := e.zsList("/api/alerts/history?limit=1"); len(h) != 1 {
		t.Fatalf("limit 1: %v", h)
	}
	if h := e.zsList("/api/alerts/history?limit=100000"); len(h) != 2 {
		t.Fatalf("limit out of range: %v", h)
	}
	if n := zsCount(zsAudit(t, e), "alert_channel.test"); n != 2 {
		t.Fatalf("audited tests: %d", n)
	}

	// nothing is firing
	if a := e.zsList("/api/alerts/active"); len(a) != 0 {
		t.Fatalf("active: %v", a)
	}
}

func TestSecAlertRules(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")

	w, m := e.zsAs(viewer, "GET", "/api/alerts/rules", nil)
	if w.Code != 200 {
		t.Fatalf("get: %d", w.Code)
	}
	set := zsObj(t, m, "settings")
	if set["cooldown_s"].(float64) != 900 || set["language"] != "en" || len(m["rules"].([]any)) != len(alerts.Catalog) {
		t.Fatalf("defaults: %v", m)
	}

	for _, bad := range []map[string]any{
		{"cooldown_s": 10, "language": "en"},
		{"cooldown_s": 600, "language": "fr"},
		{"cooldown_s": 600, "language": "en", "public_url": "ftp://x"},
		{"cooldown_s": 600, "language": "en", "rules": map[string]any{"nope": map[string]any{"enabled": true, "severity": "info"}}},
		{"cooldown_s": 600, "language": "en", "rules": map[string]any{"error_rate": map[string]any{"enabled": true, "severity": "loud"}}},
		{"cooldown_s": 600, "language": "en", "rules": map[string]any{"error_rate": map[string]any{"enabled": true, "severity": "info", "window_s": 5}}},
	} {
		m := e.must(422, "PUT", "/api/alerts/rules", bad)
		if zsField(m) != "settings" {
			t.Errorf("%v: %v", bad, m)
		}
	}
	e.must(400, "PUT", "/api/alerts/rules", map[string]any{"cooldown": 1})
	if w, _ := e.zsAs(viewer, "PUT", "/api/alerts/rules", map[string]any{"cooldown_s": 600, "language": "ko"}); w.Code != 403 {
		t.Fatalf("viewer put: %d", w.Code)
	}

	// rules omitted entirely: defaults stay
	w, m = e.zsAs(op, "PUT", "/api/alerts/rules", map[string]any{"cooldown_s": 600, "language": "ko", "send_resolved": false,
		"public_url": " https://corvus.example.com/ "})
	if w.Code != 200 {
		t.Fatalf("put: %d %s", w.Code, w.Body)
	}
	set = zsObj(t, m, "settings")
	if set["cooldown_s"].(float64) != 600 || set["language"] != "ko" || set["public_url"] != "https://corvus.example.com" || set["send_resolved"] != false {
		t.Fatalf("saved: %v", set)
	}
	if r := zsObj(t, zsObj(t, set, "rules"), "error_rate"); r["enabled"] != true {
		t.Fatalf("default rule lost: %v", r)
	}

	// one rule tuned
	m = e.must(200, "PUT", "/api/alerts/rules", map[string]any{"cooldown_s": 600, "language": "en",
		"rules": map[string]any{"error_rate": map[string]any{"enabled": false, "severity": "warning", "threshold": 10, "window_s": 600}}})
	r := zsObj(t, zsObj(t, zsObj(t, m, "settings"), "rules"), "error_rate")
	if r["enabled"] != false || r["threshold"].(float64) != 10 {
		t.Fatalf("tuned: %v", r)
	}
	got := alerts.Load(context.Background(), e.s.Store)
	if got.Rules["error_rate"].Enabled || got.Language != "en" {
		t.Fatalf("stored: %+v", got.Rules["error_rate"])
	}
	if zsCount(zsAudit(t, e), "alert_rules.update") != 2 {
		t.Fatal("not audited")
	}
}
