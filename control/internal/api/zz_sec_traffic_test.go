package api

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/traffic"
)

// ------------------------------------------------------------------ rate limits

func TestSecRateLimitsCRUD(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")

	// validation
	for _, c := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"name": "", "requests_per_second": 10}, "name"},
		{map[string]any{"name": "api", "requests_per_second": 0}, "requests_per_second"},
		{map[string]any{"name": "api", "requests_per_second": 10, "status_code": 500}, "status_code"},
		{map[string]any{"name": "api", "requests_per_second": 10, "exempt_cidrs": []string{"not-a-cidr"}}, "exempt_cidrs"},
		{map[string]any{"name": "api", "requests_per_second": 10, "exempt_cidrs": []string{"0.0.0.0/0"}}, "exempt_cidrs"},
	} {
		m := e.must(422, "POST", "/api/rate-limits", c.body)
		if zsField(m) != c.field {
			t.Errorf("%v: field %q, want %q (%v)", c.body, zsField(m), c.field, m)
		}
	}
	if w, _ := e.zsAs(viewer, "POST", "/api/rate-limits", map[string]any{"name": "v", "requests_per_second": 1}); w.Code != 403 {
		t.Fatalf("viewer create: %d", w.Code)
	}
	// unknown JSON fields are refused
	if w, _ := e.zsAs(op, "POST", "/api/rate-limits", map[string]any{"name": "v", "bogus": 1}); w.Code != 400 {
		t.Fatalf("unknown field: %d", w.Code)
	}

	// create (operator) with defaults
	w, m := e.zsAs(op, "POST", "/api/rate-limits", map[string]any{"name": " api ", "requests_per_second": 10, "burst": 20,
		"exempt_cidrs": []string{"10.0.0.1"}})
	if w.Code != 201 {
		t.Fatalf("create: %d %s", w.Code, w.Body)
	}
	rl := zsObj(t, m, "rate_limit")
	rid := id(rl)
	if rl["name"] != "api" || rl["status_code"].(float64) != 429 || rl["enabled"] != true || m["version"] == nil {
		t.Fatalf("created: %v", m)
	}
	if ex := rl["exempt_cidrs"].([]any); len(ex) != 1 || ex[0] != "10.0.0.1/32" {
		t.Fatalf("exempt normalised: %v", ex)
	}
	// duplicate name
	m = e.must(422, "POST", "/api/rate-limits", map[string]any{"name": "api", "requests_per_second": 5})
	if zsField(m) != "name" || zsCode(m) != "duplicate" {
		t.Fatalf("duplicate: %v", m)
	}

	// list (viewer may read)
	w, _ = e.zsAs(viewer, "GET", "/api/rate-limits", nil)
	var list []map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &list)
	if w.Code != 200 || len(list) != 1 || list[0]["name"] != "api" {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}

	// update
	m = e.must(200, "PUT", fmt.Sprintf("/api/rate-limits/%d", rid), map[string]any{"name": "api2", "requests_per_second": 50,
		"status_code": 503, "dry_run": true, "enabled": false})
	rl = zsObj(t, m, "rate_limit")
	if rl["name"] != "api2" || rl["requests_per_second"].(float64) != 50 || rl["status_code"].(float64) != 503 || rl["dry_run"] != true || rl["enabled"] != false {
		t.Fatalf("updated: %v", rl)
	}
	e.must(422, "PUT", fmt.Sprintf("/api/rate-limits/%d", rid), map[string]any{"name": "api2", "requests_per_second": -1})
	e.must(404, "PUT", "/api/rate-limits/9999", map[string]any{"name": "x", "requests_per_second": 1})
	e.must(400, "PUT", "/api/rate-limits/abc", map[string]any{"name": "x", "requests_per_second": 1})
	e.must(400, "PUT", fmt.Sprintf("/api/rate-limits/%d", rid), map[string]any{"nope": true})

	// in use by a proxy host: cannot be deleted
	ph := e.must(201, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{"rl.example.com"}, "forward_scheme": "http",
		"forward_host": "app", "forward_port": 8080, "rate_limit_id": rid})
	m = e.must(409, "DELETE", fmt.Sprintf("/api/rate-limits/%d", rid), nil)
	if !strings.Contains(m["message"].(string), "used by 1 proxy host") {
		t.Fatalf("in use: %v", m)
	}
	if l := e.zsList("/api/rate-limits"); l[0]["hosts"].(float64) != 1 {
		t.Fatalf("hosts count: %v", l)
	}
	e.must(200, "DELETE", fmt.Sprintf("/api/proxy-hosts/%d", id(zsObj(t, ph, "proxy_host"))), nil)

	if w, _ := e.zsAs(viewer, "DELETE", fmt.Sprintf("/api/rate-limits/%d", rid), nil); w.Code != 403 {
		t.Fatalf("viewer delete: %d", w.Code)
	}
	e.must(200, "DELETE", fmt.Sprintf("/api/rate-limits/%d", rid), nil)
	e.must(404, "DELETE", fmt.Sprintf("/api/rate-limits/%d", rid), nil)
	e.must(400, "DELETE", "/api/rate-limits/0", nil)
	if l := e.zsList("/api/rate-limits"); len(l) != 0 {
		t.Fatalf("after delete: %v", l)
	}
	a := zsAudit(t, e)
	for _, act := range []string{"rate_limit.create", "rate_limit.update", "rate_limit.delete"} {
		if zsCount(a, act) != 1 {
			t.Errorf("audit %s: %v", act, a)
		}
	}
}

// ------------------------------------------------------------------ access lists

func TestSecAccessListsCRUD(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")

	for _, c := range []struct {
		body        map[string]any
		field, code string
	}{
		{map[string]any{"name": "team", "users": []map[string]any{{"username": "bob", "password": "short"}}}, "users", "weak"},
		{map[string]any{"name": "team", "users": []map[string]any{{"username": "bob", "password": strings.Repeat("x", 73)}}}, "users", "too_long"},
		{map[string]any{"name": "team", "users": []map[string]any{{"username": "bob"}}}, "users", "password_required"},
		{map[string]any{"name": "team", "users": []map[string]any{}}, "users", "empty"},
		{map[string]any{"name": "", "users": []map[string]any{{"username": "bob", "password": "password1"}}}, "name", "invalid"},
		{map[string]any{"name": "team", "users": []map[string]any{{"username": "bad name", "password": "password1"}}}, "users", "invalid"},
	} {
		m := e.must(422, "POST", "/api/access-lists", c.body)
		if zsField(m) != c.field || zsCode(m) != c.code {
			t.Errorf("%v: got %s/%s, want %s/%s", c.body, zsField(m), zsCode(m), c.field, c.code)
		}
	}
	if w, _ := e.zsAs(viewer, "POST", "/api/access-lists", map[string]any{"name": "x"}); w.Code != 403 {
		t.Fatalf("viewer create: %d", w.Code)
	}

	m := e.must(201, "POST", "/api/access-lists", map[string]any{"name": "team", "users": []map[string]any{{"username": " bob ", "password": "password1"}}})
	al := zsObj(t, m, "access_list")
	aid := id(al)
	users := al["users"].([]any)
	if len(users) != 1 || users[0].(map[string]any)["username"] != "bob" {
		t.Fatalf("created: %v", al)
	}
	e.must(422, "POST", "/api/access-lists", map[string]any{"name": "team", "users": []map[string]any{{"username": "x", "password": "password1"}}})

	st, err := e.s.Store.AccessListByID(t.Context(), aid)
	if err != nil {
		t.Fatal(err)
	}
	oldHash := st.Users[0].PasswordHash

	// bob keeps his password, alice is new; a new user without password is refused
	e.must(422, "PUT", fmt.Sprintf("/api/access-lists/%d", aid), map[string]any{"name": "team",
		"users": []map[string]any{{"username": "bob"}, {"username": "carol"}}})
	m = e.must(200, "PUT", fmt.Sprintf("/api/access-lists/%d", aid), map[string]any{"name": "team2",
		"users": []map[string]any{{"username": "bob"}, {"username": "alice", "password": "password2"}}})
	if zsObj(t, m, "access_list")["name"] != "team2" {
		t.Fatalf("updated: %v", m)
	}
	st, _ = e.s.Store.AccessListByID(t.Context(), aid)
	hashes := map[string]string{}
	for _, u := range st.Users {
		hashes[u.Username] = u.PasswordHash
	}
	if len(st.Users) != 2 || hashes["bob"] != oldHash || hashes["alice"] == "" || hashes["alice"] == oldHash {
		t.Fatalf("stored users: %+v", st.Users)
	}
	e.must(404, "PUT", "/api/access-lists/999", map[string]any{"name": "x", "users": []map[string]any{{"username": "a", "password": "password1"}}})
	e.must(400, "PUT", "/api/access-lists/x", map[string]any{})

	w, _ := e.zsAs(viewer, "GET", "/api/access-lists", nil)
	if w.Code != 200 || !strings.Contains(w.Body.String(), `"team2"`) || strings.Contains(w.Body.String(), "password") {
		t.Fatalf("list: %d %s", w.Code, w.Body)
	}

	// in use by a proxy host
	ph := e.must(201, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{"al.example.com"}, "forward_scheme": "http",
		"forward_host": "app", "forward_port": 8080, "access_list_id": aid})
	e.must(409, "DELETE", fmt.Sprintf("/api/access-lists/%d", aid), nil)
	e.must(200, "DELETE", fmt.Sprintf("/api/proxy-hosts/%d", id(zsObj(t, ph, "proxy_host"))), nil)

	e.must(200, "DELETE", fmt.Sprintf("/api/access-lists/%d", aid), nil)
	e.must(404, "DELETE", fmt.Sprintf("/api/access-lists/%d", aid), nil)
	e.must(400, "DELETE", "/api/access-lists/-1", nil)
	a := zsAudit(t, e)
	if zsCount(a, "access_list.create") != 1 || zsCount(a, "access_list.update") != 1 || zsCount(a, "access_list.delete") != 1 {
		t.Fatalf("audit: %v", a)
	}
}

// ------------------------------------------------------------------ network settings

func TestSecNetworkSettings(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	op := e.as("operator")

	w, m := e.zsAs(viewer, "GET", "/api/settings/network", nil)
	if w.Code != 200 || m["real_ip_header"] != "X-Forwarded-For" {
		t.Fatalf("get: %d %s", w.Code, w.Body)
	}
	cf := zsObj(t, m, "cloudflare")
	if cf["source"] != "builtin" || cf["ipv4"].(float64) == 0 || cf["ipv6"].(float64) == 0 || cf["updated_at"] != nil {
		t.Fatalf("cloudflare: %v", cf)
	}
	if h := m["real_ip_headers"].([]any); len(h) < 2 {
		t.Fatalf("headers: %v", h)
	}

	// admin only
	if w, _ := e.zsAs(op, "PUT", "/api/settings/network", map[string]any{"mask_ips": true}); w.Code != 403 {
		t.Fatalf("operator put: %d", w.Code)
	}

	m = e.must(422, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{"nope"}})
	if zsField(m) != "trusted_proxies" {
		t.Fatalf("trusted: %v", m)
	}
	m = e.must(422, "PUT", "/api/settings/network", map[string]any{"real_ip_header": "X-Evil"})
	if zsField(m) != "real_ip_header" {
		t.Fatalf("header: %v", m)
	}
	m = e.must(422, "PUT", "/api/settings/network", map[string]any{"admin_allow": []string{"bad"}})
	if zsField(m) != "admin_allow" || zsCode(m) != "invalid" {
		t.Fatalf("admin_allow invalid: %v", m)
	}
	// httptest requests come from 192.0.2.1: a list without it would lock the admin out
	m = e.must(422, "PUT", "/api/settings/network", map[string]any{"admin_allow": []string{"10.0.0.0/8"}})
	if zsCode(m) != "lockout" || !strings.Contains(m["message"].(string), "192.0.2.1") {
		t.Fatalf("lockout: %v", m)
	}

	m = e.must(200, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{"10.1.0.0/16"},
		"real_ip_header": "CF-Connecting-IP", "mask_ips": true, "admin_allow": []string{"192.0.2.0/24"}})
	n := zsObj(t, m, "network")
	if n["real_ip_header"] != "CF-Connecting-IP" || n["mask_ips"] != true || m["version"] == nil {
		t.Fatalf("put: %v", m)
	}
	if tp := n["trusted_proxies"].([]any); len(tp) != 1 || tp[0] != "10.1.0.0/16" {
		t.Fatalf("trusted: %v", tp)
	}
	// the allowlist takes effect at once (cache invalidated): another address is refused
	r := httptest.NewRequest("GET", "/api/status", nil)
	r.RemoteAddr = "198.51.100.7:4000"
	rw := httptest.NewRecorder()
	e.h.ServeHTTP(rw, r)
	if rw.Code != 403 {
		t.Fatalf("outside the allowlist: %d", rw.Code)
	}
	e.must(200, "GET", "/api/status", nil)

	// omitted fields stay; an empty allowlist allows everybody again
	m = e.must(200, "PUT", "/api/settings/network", map[string]any{"trusted_proxies": []string{}, "admin_allow": []string{}})
	n = zsObj(t, m, "network")
	if n["real_ip_header"] != "CF-Connecting-IP" || n["mask_ips"] != true || len(n["admin_allow"].([]any)) != 0 {
		t.Fatalf("partial put: %v", n)
	}
	rw = httptest.NewRecorder()
	e.h.ServeHTTP(rw, r)
	if rw.Code != 200 {
		t.Fatalf("allowlist cleared: %d", rw.Code)
	}
	if zsCount(zsAudit(t, e), "network.update") != 2 {
		t.Fatalf("audit: %v", zsAudit(t, e))
	}
}

// fakeCF answers the Cloudflare IP list downloads (http.DefaultTransport is swapped:
// the fetch uses a client without its own transport). No network is used.
type fakeCF struct {
	fail   bool
	status int
	calls  int
}

func (f *fakeCF) RoundTrip(r *http.Request) (*http.Response, error) {
	f.calls++
	if f.fail {
		return nil, errors.New("dial refused")
	}
	body := "203.0.113.0/24\n198.51.100.0/24\n"
	if strings.HasSuffix(r.URL.Path, "v6") {
		body = "2001:db8::/32\n"
	}
	st := f.status
	if st == 0 {
		st = 200
	}
	return &http.Response{StatusCode: st, Body: io.NopCloser(strings.NewReader(body)), Header: http.Header{}, Request: r}, nil
}

func TestSecCloudflareRefresh(t *testing.T) {
	e := newFullEnv(t)
	op := e.as("operator")
	f := &fakeCF{}
	old := http.DefaultTransport
	http.DefaultTransport = f
	t.Cleanup(func() { http.DefaultTransport = old })

	if w, _ := e.zsAs(op, "POST", "/api/settings/network/cloudflare/refresh", nil); w.Code != 403 {
		t.Fatalf("operator: %d", w.Code)
	}
	if f.calls != 0 {
		t.Fatal("fetched for a forbidden request")
	}

	f.fail = true
	m := e.must(502, "POST", "/api/settings/network/cloudflare/refresh", nil)
	if m["error"] != "fetch_failed" || !strings.Contains(m["message"].(string), "dial refused") {
		t.Fatalf("failed fetch: %v", m)
	}
	f.fail, f.status = false, 500
	e.must(502, "POST", "/api/settings/network/cloudflare/refresh", nil)

	f.status = 0
	m = e.must(200, "POST", "/api/settings/network/cloudflare/refresh", nil)
	cf := zsObj(t, zsObj(t, m, "network"), "cloudflare")
	if cf["source"] != "remote" || cf["ipv4"].(float64) != 2 || cf["ipv6"].(float64) != 1 || cf["updated_at"] == nil || m["version"] == nil {
		t.Fatalf("refreshed: %v", m)
	}
	if zsCount(zsAudit(t, e), "network.cloudflare_refresh") != 1 {
		t.Fatal("not audited")
	}
}

// ------------------------------------------------------------------ traffic

func zsSeedTraffic(c *traffic.Collector) float64 {
	now := float64(time.Now().Unix())
	recs := []traffic.Record{
		{T: now, H: "a.example", M: "GET", U: "/", S: 200, RT: 0.01, IP: "203.0.113.1", B: 100},
		{T: now, H: "a.example", M: "GET", U: "/", S: 200, RT: 0.02, IP: "203.0.113.2", B: 100},
		{T: now, H: "a.example", M: "POST", U: "/login", S: 403, X: "sqli", IP: "203.0.113.9", UA: "=HYPERLINK(evil)"},
		{T: now, H: "a.example", M: "GET", U: "/api", S: 502, RT: 0.5, IP: "203.0.113.3"},
		{T: now, H: "b.example", M: "GET", U: "/b", S: 429, RL: "REJECTED", IP: "198.51.100.4"},
		{T: now, H: "b.example", M: "GET", U: "/x", S: 403, R: "1:block", IP: "198.51.100.5"},
	}
	for i := range recs {
		c.Add(&recs[i])
	}
	return now
}

func TestSecTrafficEndpoints(t *testing.T) {
	e := newFullEnv(t)
	viewer := e.as("viewer")
	zsSeedTraffic(e.s.Traffic)

	get := func(path string, v any) {
		t.Helper()
		w, _ := e.zsAs(viewer, "GET", path, nil)
		if w.Code != 200 {
			t.Fatalf("GET %s: %d %s", path, w.Code, w.Body)
		}
		if err := json.Unmarshal(w.Body.Bytes(), v); err != nil {
			t.Fatalf("GET %s: %v %s", path, err, w.Body)
		}
	}

	var sum map[string]any
	get("/api/traffic/summary?range=1h", &sum)
	if sum["requests"].(float64) != 6 || sum["range"] != "1h" || sum["s2xx"].(float64) != 2 {
		t.Fatalf("summary: %v", sum)
	}
	get("/api/traffic/summary?range=1h&host=b.example", &sum)
	if sum["requests"].(float64) != 2 {
		t.Fatalf("host summary: %v", sum)
	}

	var ts struct {
		Step   float64          `json:"step_s"`
		Points []map[string]any `json:"points"`
	}
	get("/api/traffic/timeseries?range=5m", &ts)
	var total float64
	for _, p := range ts.Points {
		total += p["requests"].(float64)
	}
	if ts.Step <= 0 || len(ts.Points) == 0 || total != 6 {
		t.Fatalf("timeseries: step %v, %d points, %v requests", ts.Step, len(ts.Points), total)
	}

	var hosts []map[string]any
	get("/api/traffic/hosts?range=1h", &hosts)
	byHost := map[string]float64{}
	for _, h := range hosts {
		byHost[h["host"].(string)] = h["requests"].(float64)
	}
	if byHost["a.example"] != 4 || byHost["b.example"] != 2 {
		t.Fatalf("hosts: %v", hosts)
	}

	var paths []map[string]any
	get("/api/traffic/top-paths?range=1h&host=a.example&limit=1", &paths)
	if len(paths) != 1 || paths[0]["path"] != "/" || paths[0]["requests"].(float64) != 2 {
		t.Fatalf("top paths: %v", paths)
	}

	var evs []map[string]any
	get("/api/traffic/events?kind=blocked_exploit", &evs)
	if len(evs) != 1 || evs[0]["path"] != "/login" {
		t.Fatalf("events: %v", evs)
	}
	get("/api/traffic/events?limit=50", &evs)
	if len(evs) < 3 {
		t.Fatalf("all events: %v", evs)
	}

	var logs struct {
		Items []map[string]any `json:"items"`
	}
	get("/api/traffic/logs?host=a.example&status=5xx", &logs)
	if len(logs.Items) != 1 || logs.Items[0]["path"] != "/api" {
		t.Fatalf("logs: %v", logs)
	}
	get("/api/traffic/logs?q=198.51.100&limit=10", &logs)
	if len(logs.Items) != 2 {
		t.Fatalf("logs q: %v", logs)
	}

	// unauthenticated
	if w, _ := e.do("GET", "/api/traffic/summary", nil); w.Code != 401 {
		t.Fatalf("anonymous: %d", w.Code)
	}
}

func TestSecTrafficLogExports(t *testing.T) {
	e := newFullEnv(t)
	zsSeedTraffic(e.s.Traffic)
	logViews.Delete(e.admin.ID) // the view throttle is process-wide

	w, _ := e.do("GET", "/api/traffic/logs?format=csv&host=a.example", nil, withCookie(e.cookie))
	if w.Code != 200 || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/csv") ||
		!strings.Contains(w.Header().Get("Content-Disposition"), `filename="corvus-logs-`) {
		t.Fatalf("csv: %d %v", w.Code, w.Header())
	}
	rows, err := csv.NewReader(bytes.NewReader(w.Body.Bytes())).ReadAll()
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 5 || rows[0][0] != "time" || rows[0][10] != "user_agent" {
		t.Fatalf("csv rows: %v", rows)
	}
	found := false
	for _, r := range rows[1:] {
		if r[3] == "/login" {
			found = true
			if r[10] != "'=HYPERLINK(evil)" || r[4] != "403" {
				t.Fatalf("formula not neutralised: %v", r)
			}
		}
	}
	if !found {
		t.Fatalf("no /login row: %v", rows)
	}

	w, _ = e.do("GET", "/api/traffic/logs?format=json", nil, withCookie(e.cookie))
	var items []map[string]any
	if w.Code != 200 || w.Header().Get("Content-Type") != "application/json" || json.Unmarshal(w.Body.Bytes(), &items) != nil || len(items) != 6 {
		t.Fatalf("json: %d %s", w.Code, w.Body)
	}

	// exports are always audited; plain views at most once per 10 minutes
	e.must(200, "GET", "/api/traffic/logs", nil)
	e.must(200, "GET", "/api/traffic/logs", nil)
	if n := zsCount(zsAudit(t, e), "traffic.logs_viewed"); n != 3 {
		t.Fatalf("audited views: %d, want 3 (2 exports + 1 view)", n)
	}
}

// rules normalises NPM-style allow/deny entries ("all" last), used by the NPM import.
func TestSecAccessRuleEntries(t *testing.T) {
	got, err := rules("allow", []string{"ALL", "10.0.0.1", " all ", "192.168.0.0/16"})
	if err != nil || strings.Join(got, ",") != "10.0.0.1/32,192.168.0.0/16,all,all" {
		t.Fatalf("rules: %v %v", got, err)
	}
	if got, err := rules("deny", nil); err != nil || len(got) != 0 {
		t.Fatalf("empty: %v %v", got, err)
	}
	_, err = rules("deny", []string{"bogus"})
	if err == nil || !strings.HasPrefix(err.Error(), "invalid input: deny:") {
		t.Fatalf("invalid: %v", err)
	}
}

func TestSecCSVSafe(t *testing.T) {
	for in, want := range map[string]string{"": "", "/a": "/a", "=1": "'=1", "+1": "'+1", "-1": "'-1", "@x": "'@x", "\tx": "'\tx", "\rx": "'\rx"} {
		if got := csvSafe(in); got != want {
			t.Errorf("csvSafe(%q) = %q, want %q", in, got, want)
		}
	}
}
