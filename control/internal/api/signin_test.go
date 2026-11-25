package api

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"corvus/control/internal/auth"
)

// Sign-in users, groups and a host's login page through the API (#294).
func TestSigninUsersAndGroupsAPI(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()

	g := e.must(201, "POST", "/api/signin-groups", map[string]any{"name": "Family"})
	gid := id(g)
	u := e.must(201, "POST", "/api/signin-users", map[string]any{
		"username": "  Anna@Example.com ", "password": "correct horse", "totp_required": true, "group_ids": []int64{gid},
	})
	uid := id(u)
	if u["username"] != "anna@example.com" || u["totp_required"] != true || u["totp_enrolled"] != false || u["enabled"] != true {
		t.Fatalf("created user: %v", u)
	}
	for _, k := range []string{"password", "password_hash", "totp_secret", "session_version"} {
		if _, ok := u[k]; ok {
			t.Fatalf("user exposes %s: %v", k, u)
		}
	}
	stored, err := e.s.Store.SigninUserByName(ctx, "ANNA@example.com")
	if err != nil || !auth.CheckPassword(stored.PasswordHash, "correct horse") || stored.SessionVersion != 1 {
		t.Fatalf("stored user: %+v %v", stored, err)
	}

	// validation and duplicates
	w := e.must(422, "POST", "/api/signin-users", map[string]any{"username": "anna@example.com", "password": "another one"})
	if !strings.Contains(fmt.Sprint(w["fields"]), "duplicate") {
		t.Fatalf("duplicate name: %v", w)
	}
	w = e.must(422, "POST", "/api/signin-users", map[string]any{"username": "bob", "password": "short"})
	if !strings.Contains(fmt.Sprint(w["fields"]), "weak") {
		t.Fatalf("weak password: %v", w)
	}
	e.must(422, "POST", "/api/signin-users", map[string]any{"username": "bob"})                                                       // no password
	e.must(422, "POST", "/api/signin-users", map[string]any{"username": "bob", "password": "long enough", "group_ids": []int64{999}}) // unknown group

	// group lists its members
	_, groups := e.callList("GET", "/api/signin-groups")
	if len(groups) != 1 || fmt.Sprint(groups[0].(map[string]any)["user_ids"]) != fmt.Sprintf("[%d]", uid) {
		t.Fatalf("groups: %v", groups)
	}

	// an update without a password keeps it; a new password ends the sessions
	path := fmt.Sprintf("/api/signin-users/%d", uid)
	e.must(200, "PUT", path, map[string]any{"username": "anna@example.com", "totp_required": true, "group_ids": []int64{gid}, "note": "sister"})
	stored, _ = e.s.Store.SigninUserByID(ctx, uid)
	if !auth.CheckPassword(stored.PasswordHash, "correct horse") || stored.SessionVersion != 1 || stored.Note != "sister" {
		t.Fatalf("kept password: %+v", stored)
	}
	e.must(200, "PUT", path, map[string]any{"username": "anna@example.com", "password": "new password!", "totp_required": true, "group_ids": []int64{gid}})
	stored, _ = e.s.Store.SigninUserByID(ctx, uid)
	if !auth.CheckPassword(stored.PasswordHash, "new password!") || stored.SessionVersion != 2 {
		t.Fatalf("new password: %+v", stored)
	}

	// disabling, ending sessions and resetting two-step all bump the session version
	stored.TOTPSecret = "sealed-secret"
	if err := e.s.Store.SaveSigninUser(ctx, stored); err != nil {
		t.Fatal(err)
	}
	e.must(200, "PUT", path, map[string]any{"username": "anna@example.com", "totp_required": true, "enabled": false, "group_ids": []int64{gid}})
	e.must(200, "POST", path+"/end-sessions", nil)
	m := e.must(200, "POST", path+"/reset-mfa", nil)
	stored, _ = e.s.Store.SigninUserByID(ctx, uid)
	if stored.SessionVersion != 5 || stored.TOTPSecret != "" || m["totp_enrolled"] != false || stored.Enabled {
		t.Fatalf("bumps: %+v", stored)
	}

	// audit: never the password or its hash
	list, _ := e.s.Store.ListAudit(ctx, 20)
	for _, a := range list {
		if strings.Contains(string(a.Detail), "new password!") || strings.Contains(string(a.Detail), "$2a$") {
			t.Fatalf("audit leaks a secret: %s %s", a.Action, a.Detail)
		}
	}

	// a viewer can read but not change
	viewer := e.as("viewer")
	if w, _ := e.do("GET", "/api/signin-users", nil, withCookie(viewer)); w.Code != 200 {
		t.Fatalf("viewer list: %d", w.Code)
	}
	if w, _ := e.do("POST", "/api/signin-groups", map[string]any{"name": "x"}, withCookie(viewer), withCSRF()); w.Code != 403 {
		t.Fatalf("viewer create: %d", w.Code)
	}

	// deleting a user removes it from its groups
	e.must(200, "DELETE", path, nil)
	_, groups = e.callList("GET", "/api/signin-groups")
	if fmt.Sprint(groups[0].(map[string]any)["user_ids"]) != "[]" {
		t.Fatalf("membership after delete: %v", groups)
	}
}

// A proxy host's login page: the PIN is write-only, omitted login is unchanged, groups
// must exist, Basic auth and the login page exclude each other, a group in use cannot be
// deleted.
func TestProxyHostLoginAPI(t *testing.T) {
	e := newFullEnv(t)
	ctx := context.Background()
	gid := id(e.must(201, "POST", "/api/signin-groups", map[string]any{"name": "Friends"}))

	host := func(login map[string]any) map[string]any {
		b := map[string]any{"domains": []string{"photos.example.com"}, "forward_host": "app", "forward_port": 8080}
		if login != nil {
			b["login"] = login
		}
		return b
	}
	m := zsObj(t, e.must(201, "POST", "/api/proxy-hosts", host(map[string]any{"enabled": true, "mode": "pin", "pin": "4711"})), "proxy_host")
	hid := id(m)
	l := m["login"].(map[string]any)
	if l["pin_set"] != true || l["session_hours"] != float64(168) || l["mode"] != "pin" {
		t.Fatalf("pin login: %v", l)
	}
	if _, ok := l["pin"]; ok {
		t.Fatalf("pin returned: %v", l)
	}
	if _, ok := l["pin_hash"]; ok {
		t.Fatalf("pin hash returned: %v", l)
	}
	h, _ := e.s.Store.ProxyHostByID(ctx, hid)
	if !auth.CheckPassword(h.Login.PINHash, "4711") || h.Login.PINVersion != 1 {
		t.Fatalf("stored pin: %+v", h.Login)
	}
	path := fmt.Sprintf("/api/proxy-hosts/%d", hid)

	// an older client without "login" keeps the login page; pin omitted keeps the PIN
	e.must(200, "PUT", path, host(nil))
	e.must(200, "PUT", path, host(map[string]any{"enabled": true, "mode": "pin", "session_hours": 12, "pin_set": true}))
	h, _ = e.s.Store.ProxyHostByID(ctx, hid)
	if !h.Login.Enabled || !auth.CheckPassword(h.Login.PINHash, "4711") || h.Login.PINVersion != 1 || h.Login.SessionHours != 12 {
		t.Fatalf("kept login: %+v", h.Login)
	}
	// a new PIN ends the old sessions
	e.must(200, "PUT", path, host(map[string]any{"enabled": true, "mode": "pin", "pin": "081542"}))
	h, _ = e.s.Store.ProxyHostByID(ctx, hid)
	if h.Login.PINVersion != 2 || !auth.CheckPassword(h.Login.PINHash, "081542") {
		t.Fatalf("new pin: %+v", h.Login)
	}

	// validation
	for name, c := range map[string]map[string]any{
		"short pin":   {"enabled": true, "mode": "pin", "pin": "12"},
		"no groups":   {"enabled": true, "mode": "users"},
		"bad group":   {"enabled": true, "mode": "users", "group_ids": []int64{999}},
		"bad mode":    {"enabled": true, "mode": "magic"},
		"exclude /":   {"enabled": true, "mode": "users", "group_ids": []int64{gid}, "exclude_paths": []string{"/"}},
		"reserved":    {"enabled": true, "mode": "users", "group_ids": []int64{gid}, "exclude_paths": []string{"/__corvus/x"}},
		"long":        {"enabled": true, "mode": "users", "group_ids": []int64{gid}, "session_hours": 721},
		"bad exclude": {"enabled": true, "mode": "users", "group_ids": []int64{gid}, "exclude_paths": []string{"api"}},
	} {
		if code, w := e.call("PUT", path, host(c)); code != 422 {
			t.Fatalf("%s: %d %v", name, code, w)
		}
	}
	// a new host needs a PIN in pin mode
	e.must(422, "POST", "/api/proxy-hosts", map[string]any{"domains": []string{"x.example.com"}, "forward_host": "app", "forward_port": 80,
		"login": map[string]any{"enabled": true, "mode": "pin"}})

	// users mode with exclusions (normalized: trimmed, duplicates dropped)
	m = zsObj(t, e.must(200, "PUT", path, host(map[string]any{"enabled": true, "mode": "users", "group_ids": []int64{gid, gid},
		"exclude_paths": []string{" /api/", "/api/", "/webhook"}, "pass_user": true})), "proxy_host")
	l = m["login"].(map[string]any)
	if fmt.Sprint(l["group_ids"]) != fmt.Sprintf("[%d]", gid) || fmt.Sprint(l["exclude_paths"]) != "[/api/ /webhook]" || l["pass_user"] != true {
		t.Fatalf("users login: %v", l)
	}

	// Basic auth and the login page exclude each other
	b := host(map[string]any{"enabled": true, "mode": "users", "group_ids": []int64{gid}})
	b["basic_auth"] = map[string]any{"enabled": true, "users": []map[string]any{{"username": "a", "password": "password1"}}}
	if code, w := e.call("PUT", path, b); code != 422 || !strings.Contains(fmt.Sprint(w["fields"]), "basic_auth") {
		t.Fatalf("basic auth + login: %d %v", code, w)
	}

	// the group is in use
	if code, _ := e.call("DELETE", fmt.Sprintf("/api/signin-groups/%d", gid), nil); code != 409 {
		t.Fatalf("delete group in use: %d", code)
	}
	_, groups := e.callList("GET", "/api/signin-groups")
	if groups[0].(map[string]any)["hosts"] != float64(1) {
		t.Fatalf("group use: %v", groups)
	}
	// turning the login page off keeps its settings; then the group can go
	e.must(200, "PUT", path, host(map[string]any{"enabled": false, "mode": "users", "group_ids": []int64{}}))
	e.must(200, "DELETE", fmt.Sprintf("/api/signin-groups/%d", gid), nil)

	// the audit log never holds the PIN
	list, _ := e.s.Store.ListAudit(ctx, 30)
	for _, a := range list {
		if strings.Contains(string(a.Detail), "081542") || strings.Contains(string(a.Detail), "pin_hash") {
			t.Fatalf("audit leaks the pin: %s", a.Detail)
		}
	}
}
