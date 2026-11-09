package api

import (
	"context"
	"encoding/json"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/history"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

func TestSecRetentionWithoutHistory(t *testing.T) {
	e := newFullEnv(t)
	op := e.as("operator")

	m := e.must(200, "GET", "/api/settings/retention", nil)
	set := zsObj(t, m, "settings")
	if set["log_mode"] != "notable" || set["log_days"].(float64) != 14 || m["usage"] != nil {
		t.Fatalf("defaults: %v", m)
	}
	if w, _ := e.zsAs(op, "GET", "/api/settings/retention", nil); w.Code != 403 {
		t.Fatalf("operator get: %d", w.Code)
	}
	if w, _ := e.zsAs(op, "PUT", "/api/settings/retention", map[string]any{"log_mode": "all"}); w.Code != 403 {
		t.Fatalf("operator put: %d", w.Code)
	}
	for _, bad := range []map[string]any{
		{"log_mode": "some", "log_days": 1, "log_max_mb": 100, "stats_days": 1, "events_days": 1},
		{"log_mode": "all", "log_days": 0, "log_max_mb": 100, "stats_days": 1, "events_days": 1},
		{"log_mode": "all", "log_days": 1, "log_max_mb": 10, "stats_days": 1, "events_days": 1},
		{"log_mode": "all", "log_days": 1, "log_max_mb": 100, "stats_days": 500, "events_days": 1},
	} {
		if m := e.must(422, "PUT", "/api/settings/retention", bad); zsField(m) != "retention" {
			t.Errorf("%v: %v", bad, m)
		}
	}
	e.must(400, "PUT", "/api/settings/retention", map[string]any{"days": 1})

	in := map[string]any{"log_mode": "off", "log_days": 7, "log_max_mb": 200, "stats_days": 10, "events_days": 20}
	m = e.must(200, "PUT", "/api/settings/retention", in)
	if s := zsObj(t, m, "settings"); s["log_mode"] != "off" || s["events_days"].(float64) != 20 {
		t.Fatalf("put: %v", m)
	}
	if got := history.Load(context.Background(), e.s.Store); got.LogMode != "off" || got.LogDays != 7 || got.LogMaxMB != 200 {
		t.Fatalf("stored: %+v", got)
	}
	if zsCount(zsAudit(t, e), "retention.update") != 1 {
		t.Fatal("not audited")
	}

	// archives: nothing without the history service
	w, _ := e.zsAs(op, "GET", "/api/traffic/archives", nil)
	if w.Code != 200 || strings.TrimSpace(w.Body.String()) != "[]" {
		t.Fatalf("archives: %d %s", w.Code, w.Body)
	}
	e.must(404, "GET", "/api/traffic/archives/access-2026-01-01.jsonl", nil)
}

func TestSecRetentionWithHistory(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")

	// Apply cleans up in the background after the request; keep its directory outside
	// t.TempDir so a late write cannot fail the test's cleanup.
	dir, err := os.MkdirTemp("", "corvus-history-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(dir) })
	h := &history.Service{Dir: dir, Store: e.s.Store, Collector: e.s.Traffic}
	if err := h.Open(context.Background()); err != nil {
		t.Fatal(err)
	}
	e.s.History = h
	e.h = e.s.Handler()

	today := time.Now().UTC().Format("2006-01-02")
	plain := "access-" + today + ".jsonl"
	gz := "access-2000-01-01.jsonl.gz"
	if err := os.WriteFile(filepath.Join(dir, "logs", plain), []byte(`{"h":"a.example"}`+"\n"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", gz), []byte("\x1f\x8bfake"), 0o640); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "logs", "notes.txt"), []byte("x"), 0o640); err != nil {
		t.Fatal(err)
	}

	m := e.must(200, "GET", "/api/settings/retention", nil)
	u := zsObj(t, m, "usage")
	if u["log_files"].(float64) != 2 || u["oldest_log"] != gz || u["db_bytes"].(float64) <= 0 {
		t.Fatalf("usage: %v", u)
	}

	// archives (operator; viewers may not download logs)
	if w, _ := e.zsAs(viewer, "GET", "/api/traffic/archives", nil); w.Code != 403 {
		t.Fatalf("viewer archives: %d", w.Code)
	}
	w, _ := e.zsAs(op, "GET", "/api/traffic/archives", nil)
	var arch []map[string]any
	if w.Code != 200 || json.Unmarshal(w.Body.Bytes(), &arch) != nil || len(arch) != 2 || arch[0]["name"] != plain {
		t.Fatalf("archives: %d %s", w.Code, w.Body)
	}

	w, _ = e.zsAs(op, "GET", "/api/traffic/archives/"+plain, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/x-ndjson" || !strings.Contains(w.Body.String(), "a.example") ||
		!strings.Contains(w.Header().Get("Content-Disposition"), plain) {
		t.Fatalf("download: %d %v %s", w.Code, w.Header(), w.Body)
	}
	w, _ = e.zsAs(op, "GET", "/api/traffic/archives/"+gz, nil)
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/gzip" {
		t.Fatalf("download gz: %d %v", w.Code, w.Header())
	}
	for _, name := range []string{"notes.txt", "access-1999-01-01.jsonl", "..%2Fcorvus.db"} {
		if w, _ := e.zsAs(op, "GET", "/api/traffic/archives/"+name, nil); w.Code != 404 {
			t.Errorf("%s: %d", name, w.Code)
		}
	}
	if n := zsCount(zsAudit(t, e), "traffic.log_download"); n != 2 {
		t.Fatalf("audited downloads: %d", n)
	}

	// saving applies the settings to the running service
	m = e.must(200, "PUT", "/api/settings/retention", map[string]any{"log_mode": "all", "log_days": 30, "log_max_mb": 1024, "stats_days": 30, "events_days": 30})
	if zsObj(t, m, "settings")["log_mode"] != "all" {
		t.Fatalf("put: %v", m)
	}
}

// ------------------------------------------------------------------ metrics (what TestMetricsEndpoint leaves out)

func TestSecMetricsGatewayAndCertificates(t *testing.T) {
	e := newFullEnv(t)
	tok, _ := e.createToken("prom", "viewer", 0)
	e.s.Gateway = func() (bool, time.Time) { return true, time.Now() }
	na := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := e.s.Store.CreateCertificate(context.Background(), &store.Certificate{Name: "site", Kind: "custom", Domains: []string{"a.example"}, NotAfter: na}); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Store.CreateCertificate(context.Background(), &store.Certificate{Name: "pending", Kind: "acme", Domains: []string{"b.example"}, Status: "pending"}); err != nil {
		t.Fatal(err)
	}
	e.must(201, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{"m.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080})
	e.h = e.s.Handler()
	now := float64(time.Now().Unix())
	for _, r := range []string{"9:block", "3:log", "9:block"} {
		e.s.Traffic.Add(&traffic.Record{T: now, H: "m.example.com", S: 403, R: r})
	}

	body := e.recordBody(tok)
	if i, j := strings.Index(body, `rule_id="3"} 1`), strings.Index(body, `rule_id="9"} 2`); i < 0 || j < i {
		t.Errorf("rule hits missing or unsorted:\n%s", body)
	}
	for _, want := range []string{
		"corvus_gateway_up 1\n",
		`corvus_certificate_expiry_seconds{certificate="site",id="1"} 1.893553445e+09`,
	} {
		if !strings.Contains(body, want) {
			t.Errorf("missing %q in:\n%s", want, body)
		}
	}
	if strings.Contains(body, `certificate="pending"`) {
		t.Error("certificate without expiry exported")
	}
	if strings.Contains(body, "corvus_config_version 0\n") {
		t.Error("config version not reported after a deployment")
	}
}

func TestSecMetricsFormatValue(t *testing.T) {
	for v, want := range map[float64]string{1: "1", 0.5: "0.5", 1e21: "1e+21", math.Inf(1): "+Inf", math.Inf(-1): "-Inf"} {
		if got := formatValue(v); got != want {
			t.Errorf("formatValue(%v) = %q, want %q", v, got, want)
		}
	}
	if got := formatValue(math.NaN()); got != "NaN" {
		t.Errorf("NaN: %q", got)
	}
	if b2f(true) != 1 || b2f(false) != 0 {
		t.Error("b2f")
	}
}
