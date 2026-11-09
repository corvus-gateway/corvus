package api

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/feeds"
	"corvus/control/internal/geoip"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

func zsRule(name, action string, value string) map[string]any {
	return map[string]any{"name": name, "action": action, "expression": map[string]any{
		"groups": [][]map[string]any{{{"field": "ip.src", "operator": "in", "value": []string{value}}}}}}
}

func TestSecSecurityRulesCRUD(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")

	// validation
	for _, c := range []struct {
		body  map[string]any
		field string
	}{
		{zsRule("", "block", "10.0.0.1"), "name"},
		{zsRule("r", "explode", "10.0.0.1"), "action"},
		{map[string]any{"name": "r", "action": "block", "expression": map[string]any{"groups": [][]map[string]any{}}}, "expression"},
		{map[string]any{"name": "r", "action": "block", "expression": map[string]any{"groups": [][]map[string]any{{{"field": "nope", "operator": "eq", "value": []string{"x"}}}}}}, "expression.groups.0.0"},
		{map[string]any{"name": "r", "action": "block", "scope": "hosts", "host_ids": []int64{99}, "expression": map[string]any{"groups": [][]map[string]any{{{"field": "http.path", "operator": "eq", "value": []string{"/x"}}}}}}, "host_ids"},
		{map[string]any{"name": "r", "action": "challenge", "challenge_difficulty": 99, "expression": map[string]any{"groups": [][]map[string]any{{{"field": "http.path", "operator": "eq", "value": []string{"/x"}}}}}}, "challenge_difficulty"},
	} {
		m := e.must(422, "POST", "/api/security-rules", c.body)
		if zsField(m) != c.field {
			t.Errorf("%v: field %q, want %q", c.body, zsField(m), c.field)
		}
	}
	if w, _ := e.zsAs(viewer, "POST", "/api/security-rules", zsRule("v", "block", "10.0.0.1")); w.Code != 403 {
		t.Fatalf("viewer create: %d", w.Code)
	}

	// create two rules (operator), one scoped to a host
	ph := e.must(201, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{"r.example.com"}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080})
	hostID := id(zsObj(t, ph, "proxy_host"))
	w, m := e.zsAs(op, "POST", "/api/security-rules", zsRule("block office", "block", "10.0.0.0/8"))
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	r1 := zsObj(t, m, "rule")
	id1 := id(r1)
	if r1["enabled"] != true || r1["status_code"].(float64) != 403 || r1["scope"] != "all" || m["version"] == nil {
		t.Fatalf("created: %v", r1)
	}
	scoped := zsRule("log host", "log", "192.0.2.0/24")
	scoped["scope"], scoped["host_ids"], scoped["enabled"] = "hosts", []int64{hostID, hostID}, false
	m = e.must(201, "POST", "/api/security-rules", scoped)
	r2 := zsObj(t, m, "rule")
	id2 := id(r2)
	if r2["enabled"] != false || len(r2["host_ids"].([]any)) != 1 {
		t.Fatalf("scoped: %v", r2)
	}

	// hits from the traffic collector show up in the list
	now := float64(time.Now().Unix())
	e.s.Traffic.Add(&traffic.Record{T: now, H: "r.example.com", U: "/a", S: 403, R: fmt.Sprintf("%d:block", id1), IP: "10.1.1.1"})
	e.s.Traffic.Add(&traffic.Record{T: now, H: "r.example.com", U: "/b", S: 403, R: fmt.Sprintf("%d:block", id1), IP: "10.1.1.2"})
	e.s.Traffic.Add(&traffic.Record{T: now, H: "r.example.com", U: "/c", S: 200, BD: "sqli:log", IP: "10.1.1.3"})
	w, _ = e.zsAs(viewer, "GET", "/api/security-rules", nil)
	if w.Code != 200 {
		t.Fatalf("list: %d", w.Code)
	}
	list := e.zsList("/api/security-rules")
	if len(list) != 2 || list[0]["name"] != "block office" || list[0]["hits_24h"].(float64) != 2 || list[0]["last_hit_at"] == nil ||
		list[1]["hits_24h"].(float64) != 0 {
		t.Fatalf("list with hits: %v", list)
	}

	// events: names filled in, filter by rule
	ev := e.zsList("/api/security-rules/events")
	if len(ev) != 3 {
		t.Fatalf("events: %v", ev)
	}
	named := 0
	for _, x := range ev {
		if x["rule"] == "block office" {
			named++
		}
		if x["rule_id"].(float64) == 0 && x["rule"] != "body:sqli" {
			t.Fatalf("body event renamed: %v", x)
		}
	}
	if named != 2 {
		t.Fatalf("named events: %v", ev)
	}
	if ev := e.zsList(fmt.Sprintf("/api/security-rules/events?rule_id=%d&limit=1", id1)); len(ev) != 1 || ev[0]["rule"] != "block office" {
		t.Fatalf("filtered events: %v", ev)
	}

	// update
	upd := zsRule("block office 2", "block", "10.0.0.0/8")
	upd["status_code"] = 444
	upd["id"], upd["position"], upd["hits_24h"] = 12345, 7, 99 // read-only fields are accepted and ignored
	m = e.must(200, "PUT", fmt.Sprintf("/api/security-rules/%d", id1), upd)
	r1 = zsObj(t, m, "rule")
	if r1["name"] != "block office 2" || r1["status_code"].(float64) != 444 || id(r1) != id1 || r1["enabled"] != true {
		t.Fatalf("updated: %v", r1)
	}
	e.must(422, "PUT", fmt.Sprintf("/api/security-rules/%d", id1), zsRule("x", "block", "not-an-ip"))
	e.must(404, "PUT", "/api/security-rules/999", zsRule("x", "block", "10.0.0.1"))
	e.must(400, "PUT", "/api/security-rules/zz", zsRule("x", "block", "10.0.0.1"))
	e.must(400, "PUT", fmt.Sprintf("/api/security-rules/%d", id1), map[string]any{"unknown": 1})

	// reorder
	e.must(422, "PUT", "/api/security-rules/order", map[string]any{"ids": []int64{id1}})
	e.must(422, "PUT", "/api/security-rules/order", map[string]any{"ids": []int64{id1, 999}})
	e.must(400, "PUT", "/api/security-rules/order", map[string]any{"ids": "x"})
	if w, _ := e.zsAs(viewer, "PUT", "/api/security-rules/order", map[string]any{"ids": []int64{id2, id1}}); w.Code != 403 {
		t.Fatalf("viewer reorder: %d", w.Code)
	}
	e.must(200, "PUT", "/api/security-rules/order", map[string]any{"ids": []int64{id2, id1}})
	if l := e.zsList("/api/security-rules"); id(l[0]) != id2 || id(l[1]) != id1 {
		t.Fatalf("order: %v", l)
	}

	// delete
	e.must(200, "DELETE", fmt.Sprintf("/api/security-rules/%d", id2), nil)
	e.must(404, "DELETE", fmt.Sprintf("/api/security-rules/%d", id2), nil)
	e.must(400, "DELETE", "/api/security-rules/0", nil)
	if l := e.zsList("/api/security-rules"); len(l) != 1 {
		t.Fatalf("after delete: %v", l)
	}
	a := zsAudit(t, e)
	for act, n := range map[string]int{"security_rule.create": 2, "security_rule.update": 1, "security_rule.reorder": 1, "security_rule.delete": 1} {
		if zsCount(a, act) != n {
			t.Errorf("audit %s: %d", act, zsCount(a, act))
		}
	}
}

func TestSecSecurityRulesLimit(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()
	for i := 0; i < 200; i++ {
		r := &store.SecurityRule{Name: fmt.Sprintf("r%d", i), Action: "block", StatusCode: 403, Enabled: true, Scope: "all", HostIDs: []int64{},
			Expression: store.Expression{Groups: [][]store.Condition{{{Field: "ip.src", Operator: "in", Value: []string{"10.0.0.1"}}}}}}
		if err := e.s.Store.SaveSecurityRule(ctx, r); err != nil {
			t.Fatal(err)
		}
	}
	m := e.must(409, "POST", "/api/security-rules", zsRule("one too many", "block", "10.0.0.2"))
	if !strings.Contains(m["message"].(string), "at most 200") {
		t.Fatalf("limit: %v", m)
	}
}

func TestSecSecurityRuleEventsWithoutTraffic(t *testing.T) {
	e := newFullEnv(t)
	e.s.Traffic = nil
	e.h = e.s.Handler()
	// the list works without hit counters, events are empty
	if l := e.zsList("/api/security-rules"); len(l) != 0 {
		t.Fatalf("rules: %v", l)
	}
	if ev := e.zsList("/api/security-rules/events"); len(ev) != 0 {
		t.Fatalf("events: %v", ev)
	}
}

// ------------------------------------------------------------------ GeoIP

type zsGeo struct {
	st  geoip.Status
	err error
	n   int
}

func (g *zsGeo) Status() geoip.Status { return g.st }
func (g *zsGeo) Refresh(ctx context.Context) (geoip.Status, error) {
	g.n++
	if _, ok := ctx.Deadline(); !ok {
		return geoip.Status{}, errors.New("no deadline")
	}
	return g.st, g.err
}

func TestSecGeoIP(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")

	// unavailable
	w, m := e.zsAs(viewer, "GET", "/api/security-rules/geoip", nil)
	if w.Code != 200 || m["disabled"] != true || m["attribution"] != geoip.Attribution {
		t.Fatalf("no geoip: %d %v", w.Code, m)
	}
	m = e.must(409, "POST", "/api/security-rules/geoip/refresh", nil)
	if m["error"] != "conflict" {
		t.Fatalf("refresh without geoip: %v", m)
	}

	g := &zsGeo{st: geoip.Status{Available: true, Source: "db-ip", Ranges: 42}}
	e.s.GeoIP = g
	e.h = e.s.Handler()
	w, m = e.zsAs(viewer, "GET", "/api/security-rules/geoip", nil)
	if w.Code != 200 || m["available"] != true || m["ranges"].(float64) != 42 {
		t.Fatalf("status: %v", m)
	}
	if w, _ := e.zsAs(op, "POST", "/api/security-rules/geoip/refresh", nil); w.Code != 403 || g.n != 0 {
		t.Fatalf("operator refresh: %d (%d calls)", w.Code, g.n)
	}
	m = e.must(200, "POST", "/api/security-rules/geoip/refresh", nil)
	if m["source"] != "db-ip" || g.n != 1 {
		t.Fatalf("refresh: %v", m)
	}
	g.err = errors.New("download failed: HTTP 404")
	m = e.must(502, "POST", "/api/security-rules/geoip/refresh", nil)
	if m["error"] != "geoip_failed" || !strings.Contains(m["message"].(string), "HTTP 404") || zsObj(t, m, "status")["source"] != "db-ip" {
		t.Fatalf("failed refresh: %v", m)
	}
	if n := zsCount(zsAudit(t, e), "geoip.refresh"); n != 2 {
		t.Fatalf("audited refreshes: %d", n)
	}
}

// ------------------------------------------------------------------ IP lists

type zsFeeds struct {
	list *store.IPList
	err  error
}

func (f *zsFeeds) Kick(int64)                                                {}
func (f *zsFeeds) Refresh(_ context.Context, _ int64) (*store.IPList, error) { return f.list, f.err }

func TestSecIPLists(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")

	m := e.must(201, "POST", "/api/ip-lists", map[string]any{"name": "office", "items": []string{"10.0.0.1", "10.1.0.0/16"}})
	l := zsObj(t, m, "ip_list")
	lid := id(l)
	if l["kind"] != "ip" || len(l["items"].([]any)) != 2 {
		t.Fatalf("created: %v", l)
	}
	m = e.must(422, "POST", "/api/ip-lists", map[string]any{"name": "office", "items": []string{}})
	if zsCode(m) != "duplicate" {
		t.Fatalf("duplicate: %v", m)
	}
	e.must(422, "POST", "/api/ip-lists", map[string]any{"name": "bad", "items": []string{"x.y"}})
	e.must(400, "POST", "/api/ip-lists", map[string]any{"name": "bad", "what": 1})
	e.must(404, "PUT", "/api/ip-lists/999", map[string]any{"name": "x"})
	// a static list edited into a plain list keeps working; the kind follows the stored one
	m = e.must(200, "PUT", fmt.Sprintf("/api/ip-lists/%d", lid), map[string]any{"name": "office2", "items": []string{"10.0.0.2"}})
	if l := zsObj(t, m, "ip_list"); l["name"] != "office2" || l["kind"] != "ip" || len(l["items"].([]any)) != 1 {
		t.Fatalf("updated: %v", l)
	}

	w, _ := e.zsAs(viewer, "GET", "/api/ip-lists", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"office2"`) {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}

	// a static list turned into a feed drops its entries until fetched; back to static
	// it takes the submitted entries again
	m = e.must(200, "PUT", fmt.Sprintf("/api/ip-lists/%d", lid), map[string]any{"name": "office2", "source_url": "https://feeds.example.com/ips.txt",
		"items": []string{"10.9.9.9"}})
	if l := zsObj(t, m, "ip_list"); len(l["items"].([]any)) != 0 || l["source_url"] != "https://feeds.example.com/ips.txt" {
		t.Fatalf("to feed: %v", l)
	}
	m = e.must(200, "PUT", fmt.Sprintf("/api/ip-lists/%d", lid), map[string]any{"name": "office2", "items": []string{"10.0.0.3"}})
	if l := zsObj(t, m, "ip_list"); l["source_url"] != "" || len(l["items"].([]any)) != 1 || l["items"].([]any)[0] != "10.0.0.3/32" {
		t.Fatalf("back to static: %v", l)
	}

	// refresh: feeds unavailable, list without a feed, unknown list
	e.must(503, "POST", fmt.Sprintf("/api/ip-lists/%d/refresh", lid), nil)
	cur, _ := e.s.Store.IPListByID(context.Background(), lid)
	e.s.Feeds = &zsFeeds{list: cur, err: feeds.ErrNoFeed}
	e.h = e.s.Handler()
	m = e.must(409, "POST", fmt.Sprintf("/api/ip-lists/%d/refresh", lid), nil)
	if !strings.Contains(m["message"].(string), "no source URL") {
		t.Fatalf("no feed: %v", m)
	}
	e.s.Feeds = &zsFeeds{err: store.ErrNotFound}
	e.h = e.s.Handler()
	e.must(404, "POST", "/api/ip-lists/999/refresh", nil)
	e.must(400, "POST", "/api/ip-lists/x/refresh", nil)

	// in use by a rule: cannot be deleted
	rule := zsRule("office", "allow", fmt.Sprintf("list:%d", lid))
	rm := e.must(201, "POST", "/api/security-rules", rule)
	m = e.must(409, "DELETE", fmt.Sprintf("/api/ip-lists/%d", lid), nil)
	if !strings.Contains(m["message"].(string), "used by 1 security rule") {
		t.Fatalf("in use: %v", m)
	}
	e.must(200, "DELETE", fmt.Sprintf("/api/security-rules/%d", id(zsObj(t, rm, "rule"))), nil)
	if w, _ := e.zsAs(viewer, "DELETE", fmt.Sprintf("/api/ip-lists/%d", lid), nil); w.Code != 403 {
		t.Fatalf("viewer delete: %d", w.Code)
	}
	e.must(200, "DELETE", fmt.Sprintf("/api/ip-lists/%d", lid), nil)
	e.must(404, "DELETE", fmt.Sprintf("/api/ip-lists/%d", lid), nil)
	e.must(400, "DELETE", "/api/ip-lists/abc", nil)
	if zsCount(zsAudit(t, e), "ip_list.delete") != 1 {
		t.Fatal("delete not audited")
	}
}
