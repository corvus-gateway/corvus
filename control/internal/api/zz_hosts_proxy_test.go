package api

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/underattack"
)

// ------------------------------------------------------------------ helpers (zzh*)

// zzhHost is a minimal valid proxy host body for domain.
func zzhHost(domain string) map[string]any {
	return map[string]any{"domains": []string{domain}, "forward_scheme": "http", "forward_host": "app", "forward_port": 8080}
}

// zzhWith returns a copy of base with the extra keys set.
func zzhWith(base map[string]any, kv ...any) map[string]any {
	out := map[string]any{}
	for k, v := range base {
		out[k] = v
	}
	for i := 0; i+1 < len(kv); i += 2 {
		out[kv[i].(string)] = kv[i+1]
	}
	return out
}

// zzhObj returns m[key] as an object.
func zzhObj(t *testing.T, m map[string]any, key string) map[string]any {
	t.Helper()
	o, ok := m[key].(map[string]any)
	if !ok {
		t.Fatalf("no object %q in %v", key, m)
	}
	return o
}

// zzhFields returns "field:code" for each validation error of a 422 answer.
func zzhFields(m map[string]any) []string {
	var out []string
	l, _ := m["fields"].([]any)
	for _, f := range l {
		fm, _ := f.(map[string]any)
		out = append(out, fmt.Sprintf("%v:%v", fm["field"], fm["code"]))
	}
	return out
}

// zzhHasField fails unless the 422 answer m reports field (with code, when code != "").
func zzhHasField(t *testing.T, m map[string]any, field, code string) {
	t.Helper()
	if m["error"] != "validation" {
		t.Fatalf("not a validation error: %v", m)
	}
	for _, f := range zzhFields(m) {
		if f == field+":"+code || (code == "" && strings.HasPrefix(f, field+":")) {
			return
		}
	}
	t.Fatalf("field %s:%s not reported in %v", field, code, zzhFields(m))
}

// zzhExpect sends a request as the admin and fails unless the status is want.
func (e *fullEnv) zzhExpect(want int, method, path string, body any) map[string]any {
	e.t.Helper()
	w, m := e.do(method, path, body, withCookie(e.cookie), withCSRF())
	if w.Code != want {
		e.t.Fatalf("%s %s: status %d, want %d: %s", method, path, w.Code, want, w.Body.String())
	}
	return m
}

// zzhAs sends a request with another session cookie.
func (e *fullEnv) zzhAs(cookie string, method, path string, body any) (int, map[string]any) {
	e.t.Helper()
	w, m := e.do(method, path, body, withCookie(cookie), withCSRF())
	return w.Code, m
}

// zzhCreateHost creates a proxy host and returns its id.
func (e *fullEnv) zzhCreateHost(body map[string]any) int64 {
	e.t.Helper()
	m := e.zzhExpect(201, "POST", "/api/proxy-hosts", body)
	return id(zzhObj(e.t, m, "proxy_host"))
}

func (e *fullEnv) zzhAudited(action string) bool {
	e.t.Helper()
	for _, a := range auditActions(e.t, e.testEnv) {
		if a == action {
			return true
		}
	}
	return false
}

func zzhPath(prefix string, id int64) string { return fmt.Sprintf("%s/%d", prefix, id) }

// zzhBreakGateway stores a proxy host the fake gateway rejects (bypassing the API), so
// every following deployment fails with config_rejected.
func (e *fullEnv) zzhBreakGateway() {
	e.t.Helper()
	h := &store.ProxyHost{Domains: []string{"broken.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80,
		Enabled: true, AdvancedConfig: "REJECT on;", BodyInspection: "log", BodyMaxKB: 1024, SecurityHeaders: "off", ClientIP: "direct",
		UnderAttack: "off", WaitroomPath: "/"}
	if err := e.s.Store.SaveProxyHost(context.Background(), h); err != nil {
		e.t.Fatal(err)
	}
}

// ------------------------------------------------------------------ proxy hosts

func TestZZHProxyHostCRUD(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()

	if code, l := e.callList("GET", "/api/proxy-hosts"); code != 200 || len(l) != 0 {
		t.Fatalf("empty list: %d %v", code, l)
	}

	m := e.zzhExpect(201, "POST", "/api/proxy-hosts", zzhWith(zzhHost(" App.Example.com "), "notes", "first", "websocket", true))
	ph := zzhObj(t, m, "proxy_host")
	hid := id(ph)
	if ph["body_inspection"] != "log" || ph["http2"] != true || ph["gzip"] != true || ph["enabled"] != true ||
		ph["security_headers"] != "off" || ph["client_ip"] != "direct" || ph["notes"] != "first" || ph["under_attack"] != "off" {
		t.Fatalf("defaults not applied: %v", ph)
	}
	if d := ph["domains"].([]any); len(d) != 1 || d[0] != "app.example.com" {
		t.Fatalf("domain not normalized: %v", d)
	}
	if v := zzhObj(t, m, "version"); v["status"] != "active" {
		t.Fatalf("version not active: %v", v)
	}
	if m["certificate"] != nil {
		t.Fatalf("certificate without request: %v", m["certificate"])
	}

	got := e.zzhExpect(200, "GET", zzhPath("/api/proxy-hosts", hid), nil)
	if got["forward_host"] != "app" || got["websocket"] != true {
		t.Fatalf("get: %v", got)
	}
	if code, l := e.callList("GET", "/api/proxy-hosts"); code != 200 || len(l) != 1 {
		t.Fatalf("list: %d %v", code, l)
	}
	if code, m := e.call("GET", "/api/proxy-hosts/999", nil); code != 404 || m["error"] != "not_found" {
		t.Fatalf("get missing: %d %v", code, m)
	}
	if code, m := e.call("GET", "/api/proxy-hosts/abc", nil); code != 400 || m["error"] != "bad_request" {
		t.Fatalf("get bad id: %d %v", code, m)
	}

	// update: omitted notes are kept, explicit values replace
	upd := zzhWith(zzhHost("app.example.com"), "enabled", false, "http2", false, "gzip", false, "max_upload_mb", 0, "upstream_timeout_s", 30)
	m = e.zzhExpect(200, "PUT", zzhPath("/api/proxy-hosts", hid), upd)
	ph = zzhObj(t, m, "proxy_host")
	if ph["enabled"] != false || ph["http2"] != false || ph["gzip"] != false || ph["notes"] != "first" ||
		ph["max_upload_mb"] != float64(0) || ph["upstream_timeout_s"] != float64(30) {
		t.Fatalf("update: %v", ph)
	}
	stored, _ := e.s.Store.ProxyHostByID(ctx, hid)
	if stored.Enabled || stored.Notes != "first" || stored.UpstreamTimeoutS == nil || *stored.UpstreamTimeoutS != 30 {
		t.Fatalf("stored after update: %+v", stored)
	}
	e.zzhExpect(404, "PUT", "/api/proxy-hosts/999", zzhHost("x.example.com"))
	e.zzhExpect(400, "PUT", "/api/proxy-hosts/0", zzhHost("x.example.com"))
	if code, m := e.call("PUT", zzhPath("/api/proxy-hosts", hid), map[string]any{"bogus": 1}); code != 400 || m["error"] != "bad_request" {
		t.Fatalf("unknown field: %d %v", code, m)
	}
	if code, m := e.call("POST", "/api/proxy-hosts", map[string]any{"domains": "not-a-list"}); code != 400 {
		t.Fatalf("bad JSON type: %d %v", code, m)
	}
	// a failed update changes nothing
	m = e.zzhExpect(422, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("app.example.com"), "forward_port", 0))
	zzhHasField(t, m, "forward_port", "invalid")
	if again, _ := e.s.Store.ProxyHostByID(ctx, hid); again.ForwardPort != 8080 {
		t.Fatalf("failed update stored: %+v", again)
	}

	// validation on create
	m = e.zzhExpect(422, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{}, "forward_host": "app", "forward_port": 0,
		"forward_scheme": "ftp", "body_inspection": "maybe", "security_headers": "max", "ssl_forced": true, "client_ip": "nope",
		"waitroom_path": "no-slash", "max_upload_mb": -1, "upstream_timeout_s": 0, "body_max_kb": 1, "under_attack": "sometimes"})
	for _, f := range []string{"domains", "forward_port", "forward_scheme", "body_inspection", "security_headers", "ssl_forced",
		"client_ip", "waitroom_path", "max_upload_mb", "upstream_timeout_s", "body_max_kb", "under_attack"} {
		zzhHasField(t, m, f, "")
	}
	// duplicate domain (with another proxy host)
	m = e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhHost("APP.example.com"))
	zzhHasField(t, m, "domains", "duplicate")
	if !strings.Contains(fmt.Sprint(m["message"]), fmt.Sprintf("proxy host #%d", hid)) {
		t.Fatalf("duplicate message: %v", m["message"])
	}
	// references that do not exist
	m = e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("refs.example.com"),
		"certificate_id", 991, "waitroom_id", 992, "rate_limit_id", 993, "access_list_id", 994))
	for _, f := range []string{"certificate_id", "waitroom_id", "rate_limit_id", "access_list_id"} {
		zzhHasField(t, m, f, "not_found")
	}
	// trusted proxies need network settings
	m = e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("tp.example.com"), "client_ip", "trusted_proxies"))
	zzhHasField(t, m, "client_ip", "no_trusted_proxies")
	n, _ := e.s.Store.Network(ctx)
	n.TrustedProxies = []string{"10.0.0.0/8"}
	if err := e.s.Store.SetNetwork(ctx, n); err != nil {
		t.Fatal(err)
	}
	tp := e.zzhCreateHost(zzhWith(zzhHost("tp.example.com"), "client_ip", "trusted_proxies"))

	// delete
	e.zzhExpect(200, "DELETE", zzhPath("/api/proxy-hosts", tp), nil)
	m = e.zzhExpect(200, "DELETE", zzhPath("/api/proxy-hosts", hid), nil)
	if zzhObj(t, m, "version")["status"] != "active" {
		t.Fatalf("delete version: %v", m)
	}
	e.zzhExpect(404, "DELETE", zzhPath("/api/proxy-hosts", hid), nil)
	e.zzhExpect(400, "DELETE", "/api/proxy-hosts/x", nil)
	if code, l := e.callList("GET", "/api/proxy-hosts"); code != 200 || len(l) != 0 {
		t.Fatalf("list after delete: %d %v", code, l)
	}
	for _, a := range []string{"proxy_host.create", "proxy_host.update", "proxy_host.delete"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}

func TestZZHProxyHostReferencesAndInUse(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	st := e.s.Store

	wr := id(zzhObj(t, e.zzhExpect(201, "POST", "/api/waitrooms", map[string]any{"name": "shop", "capacity": 100}), "waitroom"))
	rl := &store.RateLimit{Name: "api", RequestsPerSecond: 10, Burst: 5, StatusCode: 429}
	if err := st.SaveRateLimit(ctx, rl); err != nil {
		t.Fatal(err)
	}
	hash, _ := auth.HtpasswdHash("password1")
	al := &store.AccessList{Name: "team", Users: []store.AccessListUser{{Username: "carol", PasswordHash: hash}}}
	if err := st.SaveAccessList(ctx, al); err != nil {
		t.Fatal(err)
	}
	certPEM, keyPEM := selfSigned(t, 1, time.Now().Add(90*24*time.Hour), "ref.example.com")
	cert := id(e.zzhExpect(201, "POST", "/api/certificates", map[string]any{"certificate": certPEM, "private_key": keyPEM}))

	hid := e.zzhCreateHost(zzhWith(zzhHost("ref.example.com"), "certificate_id", cert, "ssl_forced", true, "waitroom_id", wr,
		"waitroom_path", "/checkout", "rate_limit_id", rl.ID, "access_list_id", al.ID, "block_exploits", true,
		"body_inspection", "block", "security_headers", "strict", "client_ip", "cloudflare"))

	// the read-only usernames of the attached list are filled in
	got := e.zzhExpect(200, "GET", zzhPath("/api/proxy-hosts", hid), nil)
	if u, _ := got["basic_auth_users"].([]any); len(u) != 1 || u[0] != "carol" {
		t.Fatalf("basic_auth_users: %v", got["basic_auth_users"])
	}
	_, l := e.callList("GET", "/api/proxy-hosts")
	if u, _ := l[0].(map[string]any)["basic_auth_users"].([]any); len(u) != 1 {
		t.Fatalf("list basic_auth_users: %v", l)
	}

	// what the host uses cannot be deleted
	if code, m := e.call("DELETE", zzhPath("/api/waitrooms", wr), nil); code != 409 || m["error"] != "conflict" {
		t.Fatalf("delete used waitroom: %d %v", code, m)
	}
	if code, m := e.call("DELETE", zzhPath("/api/certificates", cert), nil); code != 409 || !strings.Contains(fmt.Sprint(m["message"]), "proxy host") {
		t.Fatalf("delete used certificate: %d %v", code, m)
	}
	// a shared (not host-owned) list survives the host's deletion
	e.zzhExpect(200, "DELETE", zzhPath("/api/proxy-hosts", hid), nil)
	if _, err := st.AccessListByID(ctx, al.ID); err != nil {
		t.Fatalf("shared list deleted with the host: %v", err)
	}
	e.zzhExpect(200, "DELETE", zzhPath("/api/waitrooms", wr), nil)
	e.zzhExpect(200, "DELETE", zzhPath("/api/certificates", cert), nil)
}

func TestZZHProxyHostRoles(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	viewer, operator := e.as("viewer"), e.as("operator")

	hid := e.zzhCreateHost(zzhWith(zzhHost("adv.example.com"), "advanced_config", "add_header X-Admin 1;"))

	if code, _ := e.zzhAs(viewer, "GET", "/api/proxy-hosts", nil); code != 200 {
		t.Fatalf("viewer list: %d", code)
	}
	if code, _ := e.zzhAs(viewer, "GET", zzhPath("/api/proxy-hosts", hid), nil); code != 200 {
		t.Fatalf("viewer get: %d", code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/proxy-hosts"}, {"PUT", zzhPath("/api/proxy-hosts", hid)}, {"DELETE", zzhPath("/api/proxy-hosts", hid)},
	} {
		if code, m := e.zzhAs(viewer, c.method, c.path, zzhHost("v.example.com")); code != 403 || m["error"] != "forbidden" {
			t.Fatalf("viewer %s %s: %d %v", c.method, c.path, code, m)
		}
	}

	// operators may not add or change advanced configuration…
	code, m := e.zzhAs(operator, "POST", "/api/proxy-hosts", zzhWith(zzhHost("op.example.com"), "advanced_config", "add_header X 1;"))
	if code != 422 {
		t.Fatalf("operator advanced create: %d %v", code, m)
	}
	zzhHasField(t, m, "advanced_config", "forbidden")
	code, m = e.zzhAs(operator, "POST", "/api/proxy-hosts", zzhWith(zzhHost("op.example.com"),
		"locations", []map[string]any{{"path": "/api", "forward_host": "api", "forward_port": 80, "advanced_config": "add_header Y 1;"}}))
	if code != 422 {
		t.Fatalf("operator location advanced create: %d %v", code, m)
	}
	zzhHasField(t, m, "locations[0].advanced_config", "forbidden")
	code, m = e.zzhAs(operator, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("adv.example.com"), "advanced_config", "add_header X-Op 1;"))
	if code != 422 {
		t.Fatalf("operator advanced update: %d %v", code, m)
	}
	zzhHasField(t, m, "advanced_config", "forbidden")
	code, m = e.zzhAs(operator, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("adv.example.com"), "advanced_config", "add_header X-Admin 1;",
		"locations", []map[string]any{{"path": "/x", "forward_host": "x", "forward_port": 80, "advanced_config": "add_header Z 1;"}}))
	if code != 422 {
		t.Fatalf("operator location advanced update: %d %v", code, m)
	}
	zzhHasField(t, m, "locations[0].advanced_config", "forbidden")

	// …but can change everything else, keeping the admin's snippet
	code, m = e.zzhAs(operator, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("adv.example.com"), "advanced_config", "add_header X-Admin 1;", "forward_port", 9090,
		"locations", []map[string]any{{"path": "/api", "forward_host": "api", "forward_port": 81}}))
	if code != 200 {
		t.Fatalf("operator update keeping snippet: %d %v", code, m)
	}
	h, _ := e.s.Store.ProxyHostByID(context.Background(), hid)
	if h.ForwardPort != 9090 || h.AdvancedConfig != "add_header X-Admin 1;" || len(h.Locations) != 1 || h.Locations[0].ForwardPort != 81 {
		t.Fatalf("operator update stored: %+v", h)
	}
	if code, _ := e.zzhAs(operator, "POST", "/api/proxy-hosts", zzhHost("op.example.com")); code != 201 {
		t.Fatalf("operator create: %d", code)
	}
	if code, _ := e.zzhAs(operator, "DELETE", zzhPath("/api/proxy-hosts", hid), nil); code != 200 {
		t.Fatalf("operator delete: %d", code)
	}
}

func TestZZHProxyHostConfigRejected(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()

	m := e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("bad.example.com"), "advanced_config", "REJECT on;"))
	if m["error"] != "config_rejected" || !strings.Contains(fmt.Sprint(m["output"]), "REJECT") {
		t.Fatalf("rejected create: %v", m)
	}
	failedID := int64(m["version_id"].(float64))
	if failedID == 0 {
		t.Fatalf("no failed version id: %v", m)
	}
	if l, _ := e.s.Store.ListProxyHosts(ctx); len(l) != 0 {
		t.Fatalf("rejected host stored: %+v", l)
	}

	hid := e.zzhCreateHost(zzhHost("good.example.com"))
	m = e.zzhExpect(422, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("good.example.com"), "advanced_config", "REJECT on;"))
	if m["error"] != "config_rejected" {
		t.Fatalf("rejected update: %v", m)
	}
	if h, _ := e.s.Store.ProxyHostByID(ctx, hid); h.AdvancedConfig != "" {
		t.Fatalf("rejected update stored: %+v", h)
	}

	// the failed attempt is listed with its test output
	m = e.zzhExpect(200, "GET", "/api/deployments", nil)
	found := false
	for _, v := range m["versions"].([]any) {
		vm := v.(map[string]any)
		if int64(vm["id"].(float64)) == failedID {
			found = vm["status"] == "failed" && strings.Contains(fmt.Sprint(vm["test_output"]), "REJECT")
		}
	}
	if !found {
		t.Fatalf("failed version %d not listed: %v", failedID, m["versions"])
	}
}

// ------------------------------------------------------------------ basic auth (basicauth.go)

func TestZZHBasicAuth(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	st := e.s.Store
	users := func(u ...map[string]any) []map[string]any { return u }
	user := func(name, pw string) map[string]any { return map[string]any{"username": name, "password": pw} }

	// creation with password protection gets the host's own list
	m := e.zzhExpect(201, "POST", "/api/proxy-hosts", zzhWith(zzhHost("ba.example.com"), "access_list_id", 999,
		"basic_auth", map[string]any{"enabled": true, "users": users(user(" alice ", "password1"))}, "basic_auth_users", []string{"ignored"}))
	ph := zzhObj(t, m, "proxy_host")
	hid := id(ph)
	if ph["access_list_id"] == nil {
		t.Fatalf("no list attached: %v", ph)
	}
	listID := int64(ph["access_list_id"].(float64))
	l, err := st.AccessListByID(ctx, listID)
	if err != nil || l.Name != ownListName(hid) || len(l.Users) != 1 || l.Users[0].Username != "alice" {
		t.Fatalf("own list: %+v %v", l, err)
	}
	aliceHash := l.Users[0].PasswordHash
	got := e.zzhExpect(200, "GET", zzhPath("/api/proxy-hosts", hid), nil)
	if u, _ := got["basic_auth_users"].([]any); len(u) != 1 || u[0] != "alice" {
		t.Fatalf("basic_auth_users: %v", got)
	}

	// validation errors (all under "basic_auth"); nothing is changed
	for _, c := range []struct {
		ba   map[string]any
		code string
	}{
		{map[string]any{"enabled": true, "users": users()}, "empty"},
		{map[string]any{"enabled": true, "users": users(user("bob", "short"))}, "weak"},
		{map[string]any{"enabled": true, "users": users(user("bob", strings.Repeat("x", 73)))}, "too_long"},
		{map[string]any{"enabled": true, "users": users(user("bob", ""))}, "password_required"},
		{map[string]any{"enabled": true, "users": users(user("bad name", "password1"))}, "invalid"},
		{map[string]any{"enabled": true, "users": users(user("dup", "password1"), user("dup", "password2"))}, "duplicate"},
	} {
		m := e.zzhExpect(422, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("ba.example.com"), "basic_auth", c.ba))
		zzhHasField(t, m, "basic_auth", c.code)
	}
	m = e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("ba2.example.com"), "basic_auth", map[string]any{"enabled": true}))
	zzhHasField(t, m, "basic_auth", "empty")

	// keep alice's password (empty), add bob
	m = e.zzhExpect(200, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("ba.example.com"),
		"basic_auth", map[string]any{"enabled": true, "users": users(user("alice", ""), user("bob", "password2"))}))
	if int64(zzhObj(t, m, "proxy_host")["access_list_id"].(float64)) != listID {
		t.Fatalf("own list replaced instead of updated: %v", m)
	}
	l, _ = st.AccessListByID(ctx, listID)
	if len(l.Users) != 2 || l.Users[0].PasswordHash != aliceHash || l.Users[1].Username != "bob" {
		t.Fatalf("users after update: %+v", l.Users)
	}
	// an update without basic_auth keeps the list
	e.zzhExpect(200, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("ba.example.com"), "access_list_id", listID))
	// the generated configuration never shows the password hashes
	m = e.zzhExpect(200, "GET", "/api/deployments", nil)
	latest := int64(m["versions"].([]any)[0].(map[string]any)["id"].(float64))
	files := e.zzhExpect(200, "GET", zzhPath("/api/deployments", latest)+"/files", nil)
	masked := false
	for name, body := range files["files"].(map[string]any) {
		if strings.HasSuffix(name, ".htpasswd") {
			masked = true
			if strings.Contains(fmt.Sprint(body), "$") || !strings.Contains(fmt.Sprint(body), "not displayed") {
				t.Fatalf("htpasswd %s shown: %v", name, body)
			}
		}
	}
	if !masked {
		t.Fatalf("no htpasswd file in %v", files["names"])
	}

	// switching protection off removes the host's own list
	m = e.zzhExpect(200, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("ba.example.com"), "basic_auth", map[string]any{"enabled": false}))
	if zzhObj(t, m, "proxy_host")["access_list_id"] != nil {
		t.Fatalf("list still attached: %v", m)
	}
	if _, err := st.AccessListByID(ctx, listID); err == nil {
		t.Fatal("own list not deleted")
	}

	// a list shared by two hosts is copied on first change; the other host keeps it
	hash, _ := auth.HtpasswdHash("sharedpass")
	shared := &store.AccessList{Name: "shared", Users: []store.AccessListUser{{Username: "sam", PasswordHash: hash}}}
	if err := st.SaveAccessList(ctx, shared); err != nil {
		t.Fatal(err)
	}
	h1 := e.zzhCreateHost(zzhWith(zzhHost("s1.example.com"), "access_list_id", shared.ID))
	h2 := e.zzhCreateHost(zzhWith(zzhHost("s2.example.com"), "access_list_id", shared.ID))
	m = e.zzhExpect(200, "PUT", zzhPath("/api/proxy-hosts", h1), zzhWith(zzhHost("s1.example.com"), "access_list_id", shared.ID,
		"basic_auth", map[string]any{"enabled": true, "users": users(user("sam", ""), user("tim", "password3"))}))
	own := int64(zzhObj(t, m, "proxy_host")["access_list_id"].(float64))
	if own == shared.ID {
		t.Fatal("shared list modified in place")
	}
	ol, _ := st.AccessListByID(ctx, own)
	if ol.Name != ownListName(h1) || len(ol.Users) != 2 || ol.Users[0].PasswordHash != hash {
		t.Fatalf("copied list: %+v", ol)
	}
	sl, _ := st.AccessListByID(ctx, shared.ID)
	if len(sl.Users) != 1 {
		t.Fatalf("shared list changed: %+v", sl)
	}
	if h, _ := st.ProxyHostByID(ctx, h2); h.AccessListID == nil || *h.AccessListID != shared.ID {
		t.Fatalf("other host lost the shared list: %+v", h)
	}
	// turning protection off on the other host keeps the shared list (not its own)
	e.zzhExpect(200, "PUT", zzhPath("/api/proxy-hosts", h2), zzhWith(zzhHost("s2.example.com"), "access_list_id", shared.ID, "basic_auth", map[string]any{"enabled": false}))
	if _, err := st.AccessListByID(ctx, shared.ID); err != nil {
		t.Fatalf("shared list deleted: %v", err)
	}
	// deleting a host removes its own list
	e.zzhExpect(200, "DELETE", zzhPath("/api/proxy-hosts", h1), nil)
	if _, err := st.AccessListByID(ctx, own); err == nil {
		t.Fatal("own list kept after host deletion")
	}
}

// ------------------------------------------------------------------ under attack (underattack.go)

type zzhSampler struct{ req int64 }

func (s zzhSampler) Window(string, time.Duration, time.Time) (int64, int64, int64) {
	return s.req, 0, 30
}

func TestZZHUnderAttack(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	viewer := e.as("viewer")

	// without the service
	if code, l := e.callList("GET", "/api/under-attack"); code != 200 || len(l) != 0 {
		t.Fatalf("list without service: %d %v", code, l)
	}
	if code, m := e.call("POST", "/api/proxy-hosts/1/under-attack/deactivate", nil); code != 409 || m["error"] != "not_active" {
		t.Fatalf("deactivate without service: %d %v", code, m)
	}

	svc := &underattack.Service{Store: e.s.Store, Traffic: zzhSampler{req: 1_000_000}}
	e.s.UnderAttack = svc
	auto := e.zzhCreateHost(zzhWith(zzhHost("auto.example.com"), "under_attack", "auto"))
	on := e.zzhCreateHost(zzhWith(zzhHost("on.example.com"), "under_attack", "on", "under_attack_config", map[string]any{"mode": "cookie"}))
	e.zzhCreateHost(zzhHost("off.example.com"))
	m := e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("x.example.com"), "under_attack_config", map[string]any{"mode": "captcha"}))
	zzhHasField(t, m, "under_attack_config.mode", "invalid")

	// "on" is active, "auto" not yet
	_, l := e.callList("GET", "/api/under-attack")
	if len(l) != 2 {
		t.Fatalf("statuses: %v", l)
	}
	byHost := map[int64]map[string]any{}
	for _, x := range l {
		xm := x.(map[string]any)
		byHost[int64(xm["host_id"].(float64))] = xm
	}
	if byHost[on]["active"] != true || byHost[auto]["active"] != false || byHost[auto]["mode"] != "auto" {
		t.Fatalf("statuses before evaluation: %v", byHost)
	}
	if code, m := e.call("POST", zzhPath("/api/proxy-hosts", auto)+"/under-attack/deactivate", nil); code != 409 || m["error"] != "not_active" {
		t.Fatalf("deactivate inactive: %d %v", code, m)
	}

	if tr := svc.Evaluate(context.Background(), time.Now()); len(tr) != 1 || tr[0].HostID != auto || !tr[0].On {
		t.Fatalf("evaluation: %+v", tr)
	}
	_, l = e.callList("GET", "/api/under-attack")
	for _, x := range l {
		xm := x.(map[string]any)
		if int64(xm["host_id"].(float64)) == auto && (xm["active"] != true || xm["reason"] == "" || xm["since"] == nil) {
			t.Fatalf("auto activation not listed: %v", xm)
		}
	}

	if code, _ := e.zzhAs(viewer, "POST", zzhPath("/api/proxy-hosts", auto)+"/under-attack/deactivate", nil); code != 403 {
		t.Fatalf("viewer deactivate: %d", code)
	}
	e.zzhExpect(404, "POST", "/api/proxy-hosts/999/under-attack/deactivate", nil)
	e.zzhExpect(400, "POST", "/api/proxy-hosts/abc/under-attack/deactivate", nil)
	if m := e.zzhExpect(200, "POST", zzhPath("/api/proxy-hosts", auto)+"/under-attack/deactivate", nil); m["status"] != "ok" {
		t.Fatalf("deactivate: %v", m)
	}
	if code, _ := e.call("POST", zzhPath("/api/proxy-hosts", auto)+"/under-attack/deactivate", nil); code != 409 {
		t.Fatalf("second deactivate: %d", code)
	}
	if !e.zzhAudited("under_attack.deactivate") {
		t.Fatal("deactivation not audited")
	}
	if tr := svc.Drain(); len(tr) == 0 || !tr[len(tr)-1].Manual {
		t.Fatalf("manual transition not recorded: %+v", tr)
	}
}

// ------------------------------------------------------------------ overview

func TestZZHOverview(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)

	m := e.zzhExpect(200, "GET", "/api/overview", nil)
	if m["proxy_hosts"] != float64(0) || m["dirty"] != true || zzhObj(t, m, "gateway")["running"] != false || m["version"] != "1.2.3-test" ||
		m["redis"] != false || m["update"] != nil {
		t.Fatalf("empty overview: %v", m)
	}

	e.zzhCreateHost(zzhHost("a.example.com"))
	e.zzhCreateHost(zzhWith(zzhHost("b.example.com"), "enabled", false))
	e.zzhExpect(201, "POST", "/api/waitrooms", map[string]any{"name": "q", "capacity": 10})
	soon, soonKey := selfSigned(t, 2, time.Now().Add(10*24*time.Hour), "soon.example.com")
	later, laterKey := selfSigned(t, 3, time.Now().Add(200*24*time.Hour), "later.example.com")
	e.zzhExpect(201, "POST", "/api/certificates", map[string]any{"certificate": soon, "private_key": soonKey})
	e.zzhExpect(201, "POST", "/api/certificates", map[string]any{"certificate": later, "private_key": laterKey})
	since := time.Now().Add(-2 * time.Minute)
	e.s.Gateway = func() (bool, time.Time) { return true, since }

	m = e.zzhExpect(200, "GET", "/api/overview", nil)
	gw := zzhObj(t, m, "gateway")
	if m["proxy_hosts"] != float64(2) || m["proxy_hosts_enabled"] != float64(1) || m["waitrooms"] != float64(1) ||
		m["certificates"] != float64(2) || m["certificates_expiring"] != float64(1) || m["dirty"] != false ||
		gw["running"] != true || gw["uptime_s"].(float64) < 119 {
		t.Fatalf("overview: %v", m)
	}
	if rooms, _ := m["rooms"].([]any); len(rooms) != 1 || rooms[0].(map[string]any)["name"] != "q" {
		t.Fatalf("rooms: %v", m["rooms"])
	}
	if zzhObj(t, m, "active_version")["status"] != "active" || m["traffic"] == nil {
		t.Fatalf("active version / traffic: %v", m)
	}
	// a stopped gateway reports no uptime
	e.s.Gateway = func() (bool, time.Time) { return false, time.Time{} }
	m = e.zzhExpect(200, "GET", "/api/overview", nil)
	if gw := zzhObj(t, m, "gateway"); gw["running"] != false || gw["uptime_s"] != nil {
		t.Fatalf("stopped gateway: %v", gw)
	}
	// any signed-in user may read it; nobody else
	if code, _ := e.zzhAs(e.as("viewer"), "GET", "/api/overview", nil); code != 200 {
		t.Fatalf("viewer overview: %d", code)
	}
	if w, _ := e.do("GET", "/api/overview", nil); w.Code != 401 {
		t.Fatalf("anonymous overview: %d", w.Code)
	}
}

// ------------------------------------------------------------------ certificates ordered with a host

func TestZZHProxyHostCertificateRequest(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()

	// no ACME service
	m := e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("acme.example.com"), "certificate_request", map[string]any{"email": "ops@example.com"}))
	zzhHasField(t, m, "certificate_request", "unavailable")

	rec := &startRecorder{}
	e.s.ACME = rec
	m = e.zzhExpect(201, "POST", "/api/proxy-hosts", zzhWith(zzhHost("acme.example.com"), "certificate_request", map[string]any{"email": "ops@example.com"}))
	ph, cert := zzhObj(t, m, "proxy_host"), zzhObj(t, m, "certificate")
	cid := id(cert)
	if cert["status"] != "pending" || cert["kind"] != "acme" || cert["challenge"] != "http-01" || cert["name"] != "acme.example.com" ||
		ph["certificate_id"] != float64(cid) {
		t.Fatalf("ordered certificate: %v / %v", cert, ph)
	}
	if len(rec.started) != 1 || rec.started[0] != cid {
		t.Fatalf("issuance not started: %v", rec.started)
	}
	if c, _ := e.s.Store.CertificateByID(ctx, cid); c.ACME.Email != "ops@example.com" || !c.AutoRenew {
		t.Fatalf("stored order: %+v", c)
	}
	hid := id(ph)

	// order validation (prefixed field names); nothing is stored or started
	for _, c := range []struct {
		domains []string
		req     map[string]any
		field   string
		code    string
	}{
		{[]string{"x1.example.com"}, map[string]any{"email": "nope"}, "certificate_request.email", "invalid"},
		{[]string{"*.wild.example.com"}, map[string]any{"email": "ops@example.com"}, "certificate_request.dns_challenge", "wildcard_needs_dns"},
		{[]string{"x2.example.com"}, map[string]any{"email": "ops@example.com", "challenge": "tls-alpn-01"}, "certificate_request.challenge", "invalid"},
		{[]string{"x3.example.com"}, map[string]any{"email": "ops@example.com", "dns_challenge": true, "dns_credential_id": 999}, "certificate_request.dns_credential_id", "not_found"},
		{[]string{"x4.example.com"}, map[string]any{"email": "ops@example.com", "dns_challenge": true, "dns_credentials": "garbage"}, "certificate_request.dns_credentials", "invalid"},
		{[]string{"x5.example.com"}, map[string]any{"email": "ops@example.com", "dns_challenge": true, "dns_provider": "nosuch", "dns_credentials_ini": "a=b"}, "certificate_request.dns_credentials", "invalid"},
		{[]string{"x6.example.com"}, map[string]any{"email": "ops@example.com", "dns_challenge": true, "dns_credentials_ini": "dns_cloudflare_api_token=t", "dns_credential_name": strings.Repeat("n", 101)}, "certificate_request.dns_credential_name", "invalid"},
		{[]string{"intranet"}, map[string]any{"email": "ops@example.com"}, "certificate_request.domains", "invalid_domain"},
	} {
		body := zzhWith(zzhHost("x.example.com"), "domains", c.domains, "certificate_request", c.req)
		m := e.zzhExpect(422, "POST", "/api/proxy-hosts", body)
		zzhHasField(t, m, c.field, c.code)
	}
	if len(rec.started) != 1 {
		t.Fatalf("issuance started for a rejected order: %v", rec.started)
	}

	// DNS-01 with a token object: the credential is saved (and audited) with the order
	m = e.zzhExpect(201, "POST", "/api/proxy-hosts", zzhWith(zzhHost("dns.example.com"), "domains", []string{"dns.example.com", "*.dns.example.com"},
		"certificate_request", map[string]any{"email": "ops@example.com", "dns_challenge": true, "dns_credentials": map[string]string{"api_token": "tok-1"}, "staging": true}))
	cert = zzhObj(t, m, "certificate")
	if cert["challenge"] != "dns-01" || cert["dns_credential_id"] == nil {
		t.Fatalf("dns order: %v", cert)
	}
	creds, _ := e.s.Store.ListDNSCredentials(ctx)
	if len(creds) != 1 || !e.zzhAudited("dns_credential.create") {
		t.Fatalf("credential not saved with the order: %+v", creds)
	}
	// the same token without a name reuses the saved credential
	m = e.zzhExpect(201, "POST", "/api/proxy-hosts", zzhWith(zzhHost("dns2.example.com"),
		"certificate_request", map[string]any{"email": "ops@example.com", "challenge": "dns-01", "dns_credentials": "dns_cloudflare_api_token=tok-1"}))
	if zzhObj(t, m, "certificate")["dns_credential_id"] != float64(creds[0].ID) {
		t.Fatalf("token not reused: %v", m["certificate"])
	}
	// a named credential is saved under that name; the name must be unique
	e.zzhExpect(201, "POST", "/api/proxy-hosts", zzhWith(zzhHost("dns3.example.com"),
		"certificate_request", map[string]any{"email": "ops@example.com", "challenge": "dns-01", "dns_credentials_ini": "dns_cloudflare_api_token=tok-2", "dns_credential_name": "Zone B"}))
	m = e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("dns4.example.com"),
		"certificate_request", map[string]any{"email": "ops@example.com", "challenge": "dns-01", "dns_credentials_ini": "dns_cloudflare_api_token=tok-3", "dns_credential_name": "zone b"}))
	zzhHasField(t, m, "certificate_request.dns_credential_name", "invalid")
	creds, _ = e.s.Store.ListDNSCredentials(ctx)
	names := []string{}
	for _, c := range creds {
		names = append(names, c.Name)
	}
	sort.Strings(names)
	if len(creds) != 2 || !strings.Contains(strings.Join(names, ","), "Zone B") {
		t.Fatalf("credentials: %v", names)
	}
	// a saved credential picked by id
	m = e.zzhExpect(201, "POST", "/api/proxy-hosts", zzhWith(zzhHost("dns5.example.com"),
		"certificate_request", map[string]any{"email": "ops@example.com", "challenge": "dns-01", "dns_credential_id": creds[0].ID}))
	if zzhObj(t, m, "certificate")["dns_credential_id"] != float64(creds[0].ID) {
		t.Fatalf("saved credential not used: %v", m["certificate"])
	}

	// an update can order a new certificate as well
	m = e.zzhExpect(200, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("acme.example.com"), "certificate_request", map[string]any{"email": "ops@example.com"}))
	newID := id(zzhObj(t, m, "certificate"))
	if newID == cid || zzhObj(t, m, "proxy_host")["certificate_id"] != float64(newID) || rec.started[len(rec.started)-1] != newID {
		t.Fatalf("update order: %v", m)
	}
	m = e.zzhExpect(422, "PUT", zzhPath("/api/proxy-hosts", hid), zzhWith(zzhHost("acme.example.com"), "certificate_request", map[string]any{"email": ""}))
	zzhHasField(t, m, "certificate_request.email", "invalid")
}
