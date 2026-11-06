package api

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

type testEnv struct {
	t      *testing.T
	s      *Server
	h      http.Handler
	admin  *store.User
	cookie string
}

func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	admin, err := st.CreateUser(ctx, "admin@example.com", "Admin", "x", "admin")
	if err != nil {
		t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := st.CreateSession(ctx, auth.TokenHash(raw), admin.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	s := &Server{Store: st, Traffic: traffic.New(), Version: "1.2.3-test"}
	return &testEnv{t: t, s: s, h: s.Handler(), admin: admin, cookie: raw}
}

type reqOpt func(*http.Request)

func withCookie(v string) reqOpt {
	return func(r *http.Request) { r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: v}) }
}
func withBearer(v string) reqOpt {
	return func(r *http.Request) { r.Header.Set("Authorization", "Bearer "+v) }
}
func withCSRF() reqOpt { return func(r *http.Request) { r.Header.Set("X-Corvus-Request", "1") } }

func (e *testEnv) do(method, path string, body any, opts ...reqOpt) (*httptest.ResponseRecorder, map[string]any) {
	e.t.Helper()
	var rd *bytes.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	r := httptest.NewRequest(method, path, rd)
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func (e *testEnv) createToken(name, role string, days int) (string, map[string]any) {
	e.t.Helper()
	w, m := e.do("POST", "/api/tokens", map[string]any{"name": name, "role": role, "expires_days": days}, withCookie(e.cookie), withCSRF())
	if w.Code != 201 {
		e.t.Fatalf("create token: %d %s", w.Code, w.Body)
	}
	return m["token"].(string), m
}

var tokenRe = regexp.MustCompile(`^cvs_[0-9A-Za-z]{43}$`)

func TestTokenCreateAndVerify(t *testing.T) {
	e := newTestEnv(t)
	tok, m := e.createToken("ci", "viewer", 30)
	if !tokenRe.MatchString(tok) {
		t.Fatalf("bad token format %q", tok)
	}
	if m["prefix"] != tok[:8] || m["role"] != "viewer" || m["expires_at"] == nil {
		t.Fatalf("unexpected create response %v", m)
	}
	// Only the hash is stored; the plaintext is not returned again by the list.
	w, _ := e.do("GET", "/api/tokens", nil, withCookie(e.cookie))
	if w.Code != 200 || strings.Contains(w.Body.String(), tok) || !strings.Contains(w.Body.String(), tok[:8]) {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}
	if _, err := e.s.Store.APITokenByHash(context.Background(), auth.TokenHash(tok)); err != nil {
		t.Fatalf("hash lookup: %v", err)
	}

	w, me := e.do("GET", "/api/auth/me", nil, withBearer(tok))
	if w.Code != 200 || me["role"] != "viewer" || me["email"] != "token:ci" {
		t.Fatalf("me via token: %d %v", w.Code, me)
	}
	// viewer token cannot change configuration (and the denial is audited with the token name)
	w, _ = e.do("POST", "/api/ip-lists", map[string]any{}, withBearer(tok))
	if w.Code != 403 {
		t.Fatalf("viewer token on op endpoint: %d", w.Code)
	}
	list, _ := e.s.Store.ListAudit(context.Background(), 10)
	found := false
	for _, a := range list {
		if a.Action == "auth.forbidden" && strings.Contains(string(a.Detail), `"via":"token:ci"`) && a.UserID != nil && *a.UserID == e.admin.ID {
			found = true
		}
	}
	if !found {
		t.Fatalf("forbidden audit entry with token attribution missing: %+v", list)
	}

	// last_used_at is recorded
	got, _ := e.s.Store.APITokenByHash(context.Background(), auth.TokenHash(tok))
	if got.LastUsedAt == nil {
		t.Fatal("last_used_at not set")
	}

	// wrong / malformed tokens
	for _, bad := range []string{"cvs_nope", tok + "x", "abc", strings.Replace(tok, "cvs_", "xyz_", 1)} {
		if w, _ := e.do("GET", "/api/auth/me", nil, withBearer(bad)); w.Code != 401 {
			t.Fatalf("bad token %q accepted: %d", bad, w.Code)
		}
	}

	// delete → revoked
	id := int64(m["id"].(float64))
	if w, _ := e.do("DELETE", "/api/tokens/"+itoa(id), nil, withCookie(e.cookie), withCSRF()); w.Code != 200 {
		t.Fatalf("delete: %d", w.Code)
	}
	if w, _ := e.do("GET", "/api/auth/me", nil, withBearer(tok)); w.Code != 401 {
		t.Fatalf("deleted token still works: %d", w.Code)
	}
	actions := map[string]bool{}
	list, _ = e.s.Store.ListAudit(context.Background(), 20)
	for _, a := range list {
		actions[a.Action] = true
	}
	if !actions["token.create"] || !actions["token.delete"] {
		t.Fatalf("token audit entries missing: %v", actions)
	}
}

func itoa(n int64) string { b, _ := json.Marshal(n); return string(b) }

func TestTokenExpiry(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	past := time.Now().Add(-time.Minute)
	plain := newAPIToken()
	if _, err := e.s.Store.CreateAPIToken(ctx, "old", plain[:8], auth.TokenHash(plain), "viewer", e.admin.ID, &past); err != nil {
		t.Fatal(err)
	}
	w, m := e.do("GET", "/api/auth/me", nil, withBearer(plain))
	if w.Code != 401 || m["message"] != "token expired" {
		t.Fatalf("expired token: %d %v", w.Code, m)
	}
	if w, _ := e.do("GET", "/metrics", nil, withBearer(plain)); w.Code != 401 {
		t.Fatalf("expired token on /metrics: %d", w.Code)
	}
	// "never" expires
	tok, m := e.createToken("forever", "viewer", 0)
	if m["expires_at"] != nil {
		t.Fatalf("expires_at should be null: %v", m)
	}
	if w, _ := e.do("GET", "/api/auth/me", nil, withBearer(tok)); w.Code != 200 {
		t.Fatalf("non-expiring token: %d", w.Code)
	}
}

func TestTokenRoleLimits(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()

	// A creator cannot mint a token above their own role.
	op, _ := e.s.Store.CreateUser(ctx, "op@example.com", "Op", "x", "operator")
	body, _ := json.Marshal(map[string]any{"name": "x", "role": "admin"})
	r := httptest.NewRequest("POST", "/api/tokens", bytes.NewReader(body))
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, op))
	w := httptest.NewRecorder()
	e.s.createToken(w, r)
	if w.Code != 422 || !strings.Contains(w.Body.String(), "more privileges") {
		t.Fatalf("role escalation not rejected: %d %s", w.Code, w.Body)
	}

	// Token management and user management need a real session, even for an admin token.
	adminTok, _ := e.createToken("auto", "admin", 0)
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/users"}, {"GET", "/api/tokens"}, {"POST", "/api/tokens"}, {"DELETE", "/api/tokens/1"},
		{"POST", "/api/auth/password"}, {"POST", "/api/auth/mfa/setup"}, {"POST", "/api/auth/logout"},
	} {
		w, m := e.do(c.method, c.path, map[string]any{"name": "y", "role": "viewer"}, withBearer(adminTok))
		if w.Code != 403 || m["error"] != "session_required" {
			t.Fatalf("%s %s with token: %d %v", c.method, c.path, w.Code, m)
		}
	}
	// the operator session cannot reach token management at all
	opRaw := auth.RandomToken(32)
	_ = e.s.Store.CreateSession(ctx, auth.TokenHash(opRaw), op.ID, time.Hour, "", "")
	if w, _ := e.do("GET", "/api/tokens", nil, withCookie(opRaw)); w.Code != 403 {
		t.Fatalf("operator listing tokens: %d", w.Code)
	}

	// Effective role follows the creator: demoted → capped, disabled → rejected.
	if err := e.s.Store.UpdateUser(ctx, e.admin.ID, e.admin.Name, "viewer", false); err != nil {
		t.Fatal(err)
	}
	if _, me := e.do("GET", "/api/auth/me", nil, withBearer(adminTok)); me["role"] != "viewer" {
		t.Fatalf("role not capped to creator's: %v", me)
	}
	if err := e.s.Store.UpdateUser(ctx, e.admin.ID, e.admin.Name, "admin", true); err != nil {
		t.Fatal(err)
	}
	if w, _ := e.do("GET", "/api/auth/me", nil, withBearer(adminTok)); w.Code != 401 {
		t.Fatalf("token of a disabled user accepted: %d", w.Code)
	}
}

func TestCSRFBearerExemption(t *testing.T) {
	e := newTestEnv(t)
	ok := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(204) })
	h := e.s.csrf(ok)
	run := func(opts ...reqOpt) int {
		r := httptest.NewRequest("POST", "/api/proxy-hosts", strings.NewReader("{}"))
		for _, o := range opts {
			o(r)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w.Code
	}
	if c := run(withCookie(e.cookie)); c != 403 {
		t.Fatalf("cookie request without CSRF header: %d", c)
	}
	if c := run(withCookie(e.cookie), withCSRF()); c != 204 {
		t.Fatalf("cookie request with CSRF header: %d", c)
	}
	if c := run(withBearer("cvs_whatever")); c != 204 {
		t.Fatalf("bearer request should be exempt: %d", c)
	}
	// end to end: an operator token can POST without the header (reaches the handler's validation)
	tok, _ := e.createToken("op", "operator", 0)
	w, m := e.do("DELETE", "/api/tokens/999", nil, withBearer(tok))
	if w.Code != 403 || m["error"] != "session_required" { // not "csrf"
		t.Fatalf("bearer DELETE: %d %v", w.Code, m)
	}
	// and the cookie path still rejects a missing header
	w, m = e.do("DELETE", "/api/tokens/999", nil, withCookie(e.cookie))
	if w.Code != 403 || m["error"] != "csrf" {
		t.Fatalf("cookie DELETE without header: %d %v", w.Code, m)
	}
}

// ------------------------------------------------------------------ /metrics

var (
	sampleRe = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*)(\{(.*)\})? (\S+)$`)
	labelRe  = regexp.MustCompile(`^([a-zA-Z_][a-zA-Z0-9_]*)="((?:\\.|[^"\\])*)"(,|$)`)
)

type promSample struct {
	name   string
	labels map[string]string
	value  string
}

func parseProm(t *testing.T, body string) (map[string]string, []promSample) {
	t.Helper()
	types := map[string]string{}
	var out []promSample
	sc := bufio.NewScanner(strings.NewReader(body))
	for sc.Scan() {
		line := sc.Text()
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "# TYPE ") {
			f := strings.Fields(line)
			if len(f) != 4 {
				t.Fatalf("bad TYPE line %q", line)
			}
			types[f[2]] = f[3]
			continue
		}
		if strings.HasPrefix(line, "#") {
			continue
		}
		m := sampleRe.FindStringSubmatch(line)
		if m == nil {
			t.Fatalf("unparsable sample line %q", line)
		}
		s := promSample{name: m[1], labels: map[string]string{}, value: m[4]}
		rest := m[3]
		for rest != "" {
			lm := labelRe.FindStringSubmatch(rest)
			if lm == nil {
				t.Fatalf("bad labels in %q (at %q)", line, rest)
			}
			v := strings.NewReplacer(`\\`, `\`, `\"`, `"`, `\n`, "\n").Replace(lm[2])
			s.labels[lm[1]] = v
			rest = rest[len(lm[0]):]
		}
		if _, ok := types[s.name]; !ok {
			t.Fatalf("sample %q before its TYPE line", s.name)
		}
		out = append(out, s)
	}
	return types, out
}

func TestMetricsEndpoint(t *testing.T) {
	e := newTestEnv(t)
	if w, _ := e.do("GET", "/metrics", nil); w.Code != 401 {
		t.Fatalf("unauthenticated /metrics: %d", w.Code)
	}
	if w, _ := e.do("GET", "/metrics", nil, withCookie(e.cookie)); w.Code != 401 {
		t.Fatalf("/metrics with only a session cookie: %d", w.Code)
	}
	tok, _ := e.createToken("prom", "viewer", 0)

	now := float64(time.Now().Unix())
	weird := "we\"ird\\host\nx"
	recs := []traffic.Record{
		{T: now, H: "a.example", S: 200, RT: 0.004},
		{T: now, H: "a.example", S: 502, RT: 0.2},
		{T: now, H: "a.example", S: 403, X: "sqli"},
		{T: now, H: "a.example", S: 429, RL: "REJECTED"},
		{T: now, H: "a.example", S: 403, R: "7:block"},
		{T: now, H: weird, S: 301, RT: 0.01},
	}
	for i := range recs {
		e.s.Traffic.Add(&recs[i])
	}

	w, _ := e.do("GET", "/metrics", nil, withBearer(tok))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/plain; version=0.0.4") {
		t.Fatalf("/metrics: %d %q", w.Code, w.Header().Get("Content-Type"))
	}
	body := w.Body.String()
	types, samples := parseProm(t, body)
	for name, typ := range map[string]string{
		"corvus_requests_total": "counter", "corvus_blocked_total": "counter", "corvus_request_duration_seconds": "gauge",
		"corvus_security_rule_hits_24h": "gauge", "corvus_access_queue_waiting": "gauge", "corvus_access_queue_active": "gauge",
		"corvus_gateway_up": "gauge", "corvus_redis_up": "gauge", "corvus_certificate_expiry_seconds": "gauge",
		"corvus_config_version": "gauge", "corvus_build_info": "gauge",
	} {
		if types[name] != typ {
			t.Errorf("family %s: type %q, want %q", name, types[name], typ)
		}
	}
	get := func(name string, labels map[string]string) (string, bool) {
		for _, s := range samples {
			if s.name != name {
				continue
			}
			match := true
			for k, v := range labels {
				if s.labels[k] != v {
					match = false
				}
			}
			if match {
				return s.value, true
			}
		}
		return "", false
	}
	checks := []struct {
		name   string
		labels map[string]string
		want   string
	}{
		{"corvus_requests_total", map[string]string{"host": "a.example", "class": "2xx"}, "1"},
		{"corvus_requests_total", map[string]string{"host": "a.example", "class": "4xx"}, "3"},
		{"corvus_requests_total", map[string]string{"host": "a.example", "class": "5xx"}, "1"},
		{"corvus_requests_total", map[string]string{"host": weird, "class": "3xx"}, "1"},
		{"corvus_blocked_total", map[string]string{"host": "a.example", "kind": "exploit"}, "1"},
		{"corvus_blocked_total", map[string]string{"host": "a.example", "kind": "rate_limit"}, "1"},
		{"corvus_blocked_total", map[string]string{"host": "a.example", "kind": "rule"}, "1"},
		{"corvus_blocked_total", map[string]string{"host": "a.example", "kind": "body"}, "0"},
		{"corvus_security_rule_hits_24h", map[string]string{"rule_id": "7"}, "1"},
		{"corvus_build_info", map[string]string{"version": "1.2.3-test"}, "1"},
		{"corvus_gateway_up", nil, "0"},
		{"corvus_redis_up", nil, "0"},
		{"corvus_config_version", nil, "0"},
	}
	for _, c := range checks {
		if v, ok := get(c.name, c.labels); !ok || v != c.want {
			t.Errorf("%s%v = %q (found %v), want %q", c.name, c.labels, v, ok, c.want)
		}
	}
	if _, ok := get("corvus_request_duration_seconds", map[string]string{"host": "a.example", "quantile": "0.99"}); !ok {
		t.Error("missing p99 latency for a.example")
	}
	if !strings.Contains(body, `host="we\"ird\\host\nx"`) {
		t.Errorf("label not escaped:\n%s", body)
	}

	// counters are monotonic: more traffic only increases them
	e.s.Traffic.Add(&traffic.Record{T: now + 1, H: "a.example", S: 200})
	_, samples = parseProm(t, e.recordBody(tok))
	if v, _ := get("corvus_requests_total", map[string]string{"host": "a.example", "class": "2xx"}); v != "2" {
		t.Errorf("counter after more traffic = %q, want 2", v)
	}
}

func (e *testEnv) recordBody(tok string) string {
	w, _ := e.do("GET", "/metrics", nil, withBearer(tok))
	return w.Body.String()
}

func TestNewAPITokenUnique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 200; i++ {
		tok := newAPIToken()
		if !tokenRe.MatchString(tok) || seen[tok] {
			t.Fatalf("bad or duplicate token %q", tok)
		}
		seen[tok] = true
	}
}

// API tokens cannot change how people sign in or reach the console, or import from NPM.
func TestSessionOnlySettings(t *testing.T) {
	for _, c := range []struct {
		method, path string
		want         bool
	}{
		{"PUT", "/api/settings/sso", true}, {"GET", "/api/settings/sso", false},
		{"PUT", "/api/settings/network", true}, {"GET", "/api/settings/network", false},
		{"POST", "/api/import/npm/apply", true}, {"POST", "/api/import/npm/preview", true},
		{"POST", "/api/settings/network/cloudflare/refresh", false}, {"GET", "/api/proxy-hosts", false},
	} {
		if got := sessionOnly(httptest.NewRequest(c.method, c.path, nil)); got != c.want {
			t.Errorf("%s %s: session only = %v, want %v", c.method, c.path, got, c.want)
		}
	}
}
