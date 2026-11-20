package api

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"corvus/control/internal/conncheck"
	"corvus/control/internal/gateway"
)

// Backend pools through the API: clients that only know forward_host/forward_port keep
// working (no extra targets; updates leave stored ones alone), extra_targets adds
// backends and [] removes them, and every backend is validated.
func TestProxyHostExtraTargetsAPI(t *testing.T) {
	e := newFullEnv(t)
	// old shape
	m := zsObj(t, e.must(201, "POST", "/api/proxy-hosts", map[string]any{
		"domains": []string{"old.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080,
	}), "proxy_host")
	if l, ok := m["extra_targets"].([]any); !ok || len(l) != 0 || m["forward_host"] != "app" {
		t.Fatalf("old shape: %v", m)
	}
	// new shape
	m = zsObj(t, e.must(201, "POST", "/api/proxy-hosts", map[string]any{
		"domains": []string{"pool.example.com"}, "forward_scheme": "http", "forward_host": "app1", "forward_port": 8080,
		"extra_targets": []map[string]any{{"forward_host": " APP2 ", "forward_port": 8080}, {"forward_host": "10.0.0.3", "forward_port": 9000}},
	}), "proxy_host")
	pool := id(m)
	l, _ := m["extra_targets"].([]any)
	if len(l) != 2 || l[0].(map[string]any)["forward_host"] != "app2" || l[1].(map[string]any)["forward_port"] != float64(9000) {
		t.Fatalf("new shape: %v", m)
	}
	path := fmt.Sprintf("/api/proxy-hosts/%d", pool)
	// an update without extra_targets (an older client) keeps them
	e.must(200, "PUT", path, map[string]any{"domains": []string{"pool.example.com"}, "forward_scheme": "http", "forward_host": "app1", "forward_port": 8081})
	got := e.must(200, "GET", path, nil)
	if l, _ := got["extra_targets"].([]any); len(l) != 2 || got["forward_port"] != float64(8081) {
		t.Fatalf("old client update: %v", got)
	}
	h, err := e.s.Store.ProxyHostByID(context.Background(), pool)
	if err != nil || len(h.Targets()) != 3 {
		t.Fatalf("stored: %+v %v", h, err)
	}
	// [] leaves a single target
	e.must(200, "PUT", path, map[string]any{"domains": []string{"pool.example.com"}, "forward_scheme": "http", "forward_host": "app1", "forward_port": 8081,
		"extra_targets": []any{}})
	if h, _ := e.s.Store.ProxyHostByID(context.Background(), pool); len(h.ExtraTargets) != 0 {
		t.Fatalf("[] did not clear: %+v", h.ExtraTargets)
	}
	// validation, per backend
	for _, c := range []struct {
		extra []map[string]any
		field string
	}{
		{[]map[string]any{{"forward_host": "a b", "forward_port": 80}}, "extra_targets[0].forward_host"},
		{[]map[string]any{{"forward_host": "b", "forward_port": 0}}, "extra_targets[0].forward_port"},
		{[]map[string]any{{"forward_host": "b", "forward_port": 80}, {"forward_host": "app1", "forward_port": 8081}}, "extra_targets[1].forward_host"},
	} {
		w := e.must(422, "PUT", path, map[string]any{"domains": []string{"pool.example.com"}, "forward_scheme": "http", "forward_host": "app1", "forward_port": 8081,
			"extra_targets": c.extra})
		fields, _ := w["fields"].([]any)
		if len(fields) == 0 || fields[0].(map[string]any)["field"] != c.field {
			t.Errorf("%v: %v, want %s", c.extra, w, c.field)
		}
	}
}

// "Check connection" for a pool checks every backend and answers per backend; a single
// target answers as before.
func TestCheckConnectionPool(t *testing.T) {
	e := newFullEnv(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	open := ln.Addr().(*net.TCPAddr).Port
	closed, _ := net.Listen("tcp", "127.0.0.1:0")
	refused := closed.Addr().(*net.TCPAddr).Port
	closed.Close()
	e.s.ConnCheck = &conncheck.Checker{Timeout: 500 * time.Millisecond}

	single := e.must(200, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "127.0.0.1", "forward_port": open})
	if _, ok := single["targets"]; ok || single["ok"] != true {
		t.Fatalf("single target: %v", single)
	}
	m := e.must(200, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "127.0.0.1", "forward_port": open,
		"extra_targets": []map[string]any{{"forward_host": "127.0.0.1", "forward_port": refused}}})
	ts, _ := m["targets"].([]any)
	if len(ts) != 2 || ts[0].(map[string]any)["ok"] != true || ts[1].(map[string]any)["ok"] != false || ts[1].(map[string]any)["kind"] != "refused" {
		t.Fatalf("per backend: %v", m)
	}
	if m["ok"] != false || m["kind"] != "refused" {
		t.Fatalf("summary of a pool with a failing backend: %v", m)
	}
	m = e.must(200, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "127.0.0.1", "forward_port": open,
		"extra_targets": []map[string]any{{"forward_host": "127.0.0.1", "forward_port": open}}})
	if m["ok"] != true || len(m["targets"].([]any)) != 2 {
		t.Fatalf("all working: %v", m)
	}
	// invalid or internal backends name their field
	w := e.must(422, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "127.0.0.1", "forward_port": open,
		"extra_targets": []map[string]any{{"forward_host": "a b", "forward_port": 80}}})
	if f := w["fields"].([]any)[0].(map[string]any); f["field"] != "extra_targets[0].forward_host" {
		t.Fatalf("invalid backend: %v", w)
	}
	w = e.must(422, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "127.0.0.1", "forward_port": open,
		"extra_targets": []map[string]any{{"forward_host": "10.0.0.1", "forward_port": 80}, {"forward_host": "127.0.0.1", "forward_port": 9180}}})
	if f := w["fields"].([]any)[0].(map[string]any); f["field"] != "extra_targets[1].forward_host" || f["code"] != "internal" {
		t.Fatalf("internal backend: %v", w)
	}
	many := []map[string]any{}
	for i := 0; i < 32; i++ {
		many = append(many, map[string]any{"forward_host": fmt.Sprintf("app%d", i), "forward_port": 80})
	}
	e.must(422, "POST", "/api/proxy-hosts/check", map[string]any{"forward_host": "app", "forward_port": 80, "extra_targets": many})
}

// Upstream errors of a pool host say how many backends it has and whether all failed.
func TestUpstreamErrorsPool(t *testing.T) {
	e := newFullEnv(t)
	mk := func(domain string, extra ...map[string]any) int64 {
		body := map[string]any{"domains": []string{domain}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080}
		if len(extra) > 0 {
			body["extra_targets"] = extra
		}
		return id(zsObj(t, e.must(201, "POST", "/api/proxy-hosts", body), "proxy_host"))
	}
	single := mk("single.example.com")
	pool := mk("pool.example.com", map[string]any{"forward_host": "app2", "forward_port": 8080}, map[string]any{"forward_host": "app3", "forward_port": 8080})
	down := mk("down.example.com", map[string]any{"forward_host": "app2", "forward_port": 8080})
	at := time.Now().Add(2 * time.Second)
	e.s.UpstreamFailures = func() []gateway.UpstreamFailure {
		return []gateway.UpstreamFailure{
			{Server: "single.example.com", At: at, Kind: "refused", Upstream: "10.0.0.5:8080", Message: "connect() failed (111: Connection refused) while connecting to upstream"},
			{Server: "pool.example.com", At: at, Kind: "refused", Upstream: "172.20.0.3:8080", Message: "connect() failed (111: Connection refused) while connecting to upstream"},
			{Server: "down.example.com", At: at, Kind: "dns", Message: "no live upstreams while connecting to upstream"},
		}
	}
	byHost := map[int64]map[string]any{}
	for _, x := range e.zsList("/api/upstream-errors") {
		byHost[int64(x["host_id"].(float64))] = x
	}
	if x := byHost[single]; x == nil || x["backends"] != nil || x["all_down"] != nil {
		t.Fatalf("single: %v", x)
	}
	if x := byHost[pool]; x == nil || x["backends"] != float64(3) || x["all_down"] != nil || x["upstream"] != "172.20.0.3:8080" {
		t.Fatalf("one backend of a pool: %v", x)
	}
	if x := byHost[down]; x == nil || x["backends"] != float64(2) || x["all_down"] != true || x["kind"] != "other" {
		t.Fatalf("whole pool down: %v", x)
	}
}
