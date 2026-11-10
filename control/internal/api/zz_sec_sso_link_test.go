package api

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/sso"
	"corvus/control/internal/store"
)

// zlAdmin calls the console API as the local administrator of an ssoEnv.
func zlAdmin(t *testing.T, e *ssoEnv, method, path string) (int, []byte) {
	t.Helper()
	ctx := context.Background()
	root, err := e.s.Store.UserByEmail(ctx, "root@example.com")
	if err != nil {
		t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := e.s.Store.CreateSession(ctx, auth.TokenHash(raw), root.ID, time.Hour, "", ""); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequest(method, e.console.URL+path, nil)
	req.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: raw})
	req.Header.Set("X-Corvus-Request", "1")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	var buf [1 << 16]byte
	n, _ := resp.Body.Read(buf[:])
	return resp.StatusCode, buf[:n]
}

func zlSubject(t *testing.T, e *ssoEnv, uid int64) string {
	t.Helper()
	sub, err := e.s.Store.IdentitySubject(context.Background(), uid, e.idp.srv.URL)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		t.Fatal(err)
	}
	return sub
}

// An existing user is linked on the first SSO sign-in with a verified email; later
// sign-ins follow the IdP account, not the email.
func TestZZSSOLinkFollowsSubject(t *testing.T) {
	e := newSSOEnv(t, func(s *sso.Settings) { s.AutoProvision = false })
	ctx := context.Background()
	alice, err := e.s.Store.CreateUser(ctx, "alice@example.com", "Alice", "x", "viewer")
	if err != nil {
		t.Fatal(err)
	}
	c := e.browser()
	if loc := e.login(c); loc != "/" {
		t.Fatalf("first sign-in: %q", loc)
	}
	if sub := zlSubject(t, e, alice.ID); sub != "sub-alice@example.com" {
		t.Fatalf("not linked: %q", sub)
	}
	list, _ := e.s.Store.ListAudit(ctx, 5)
	var d map[string]any
	_ = json.Unmarshal(list[0].Detail, &d)
	if list[0].Action != "auth.sso_login" || d["linked"] != true {
		t.Fatalf("link not audited: %s %s", list[0].Action, list[0].Detail)
	}
	// the IdP now reports another address (no local user has it): still alice
	e.idp.mutate = func(m map[string]any) { m["email"] = "alice.renamed@example.com" }
	c2 := e.browser()
	if loc := e.login(c2); loc != "/" {
		t.Fatalf("renamed sign-in: %q", loc)
	}
	if u := e.me(c2); u == nil || u.ID != alice.ID {
		t.Fatalf("me = %+v", u)
	}
	// a different IdP account claiming alice's address is refused
	e.idp.mutate = func(m map[string]any) { m["sub"] = "intruder" }
	wantErr(t, e.login(e.browser()), "link_conflict")
}

// Without email_verified true an existing user is not linked.
func TestZZSSOLinkNeedsVerifiedEmail(t *testing.T) {
	e := newSSOEnv(t, nil)
	ctx := context.Background()
	alice, _ := e.s.Store.CreateUser(ctx, "alice@example.com", "Alice", "x", "viewer")
	e.idp.mutate = func(m map[string]any) { delete(m, "email_verified") }
	wantErr(t, e.login(e.browser()), "link_unverified")
	if zlSubject(t, e, alice.ID) != "" {
		t.Fatal("linked without a verified email")
	}
	e.idp.mutate = func(m map[string]any) { m["email_verified"] = "true" } // string form
	if loc := e.login(e.browser()); loc != "/" {
		t.Fatalf("verified (string): %q", loc)
	}
	if zlSubject(t, e, alice.ID) != "sub-alice@example.com" {
		t.Fatal("not linked")
	}
}

// With trust_email (IdPs such as Entra ID that never send email_verified) a missing
// claim links an existing user; an explicit false is still refused.
func TestZZSSOLinkTrustEmail(t *testing.T) {
	e := newSSOEnv(t, func(s *sso.Settings) { s.AutoProvision = false; s.TrustEmail = true })
	ctx := context.Background()
	alice, _ := e.s.Store.CreateUser(ctx, "alice@example.com", "Alice", "x", "viewer")
	e.idp.mutate = func(m map[string]any) { m["email_verified"] = false }
	wantErr(t, e.login(e.browser()), "email_unverified")
	if zlSubject(t, e, alice.ID) != "" {
		t.Fatal("linked with email_verified false")
	}
	e.idp.mutate = func(m map[string]any) { delete(m, "email_verified") }
	if loc := e.login(e.browser()); loc != "/" {
		t.Fatalf("trusted sign-in: %q", loc)
	}
	if zlSubject(t, e, alice.ID) != "sub-alice@example.com" {
		t.Fatal("not linked")
	}
}

// New users are created linked, also without email_verified.
func TestZZSSOProvisionedUsersAreLinked(t *testing.T) {
	e := newSSOEnv(t, nil)
	ctx := context.Background()
	e.identity("new@example.com", "corvus-ops")
	e.idp.mutate = func(m map[string]any) { delete(m, "email_verified") }
	c := e.browser()
	if loc := e.login(c); loc != "/" {
		t.Fatalf("provision: %q", loc)
	}
	u := e.me(c)
	if u == nil || zlSubject(t, e, u.ID) != "sub-new@example.com" {
		t.Fatalf("provisioned user not linked: %+v", u)
	}
	// concurrent first sign-in of the same account: the existing user is returned
	cl := sso.ParseClaims(map[string]any{"sub": "sub-new@example.com", "email": "new@example.com"})
	again, err := e.s.provisionSSOUser(ctx, e.idp.srv.URL, "sub-new@example.com", cl, "viewer")
	if err != nil || again.ID != u.ID {
		t.Fatalf("race: %v %+v", err, again)
	}
	// the same address from another account cannot take the user over
	if _, err := e.s.provisionSSOUser(ctx, e.idp.srv.URL, "someone-else", cl, "viewer"); !errors.Is(err, errSSOLinkConflict) {
		t.Fatalf("other subject: %v", err)
	}
}

func TestZZSSOMissingSubject(t *testing.T) {
	e := newSSOEnv(t, nil)
	e.idp.mutate = func(m map[string]any) { delete(m, "sub") }
	wantErr(t, e.login(e.browser()), "token_invalid")
}

// Administrators see and remove SSO links; afterwards the user links again on the
// next verified sign-in.
func TestZZSSOAdminUnlink(t *testing.T) {
	e := newSSOEnv(t, func(s *sso.Settings) { s.AutoProvision = false })
	ctx := context.Background()
	alice, _ := e.s.Store.CreateUser(ctx, "alice@example.com", "Alice", "x", "viewer")
	if err := e.s.Store.LinkIdentity(ctx, alice.ID, e.idp.srv.URL, "old-account"); err != nil {
		t.Fatal(err)
	}
	wantErr(t, e.login(e.browser()), "link_conflict")

	code, body := zlAdmin(t, e, "GET", "/api/users")
	var users []map[string]any
	if code != 200 || json.Unmarshal(body, &users) != nil {
		t.Fatalf("users: %d %s", code, body)
	}
	for _, u := range users {
		if (u["email"] == "alice@example.com") != (u["sso_linked"] == true) {
			t.Fatalf("sso_linked: %v", u)
		}
	}
	if code, body := zlAdmin(t, e, "POST", "/api/users/"+itoa(alice.ID)+"/reset-sso"); code != 200 {
		t.Fatalf("unlink: %d %s", code, body)
	}
	if code, _ := zlAdmin(t, e, "POST", "/api/users/9999/reset-sso"); code != 404 {
		t.Fatalf("unknown user: %d", code)
	}
	if code, _ := zlAdmin(t, e, "POST", "/api/users/x/reset-sso"); code != 400 {
		t.Fatalf("bad id: %d", code)
	}
	if e.audited("user.sso_unlinked") != 1 {
		t.Fatal("unlink not audited")
	}
	if loc := e.login(e.browser()); loc != "/" {
		t.Fatalf("after unlink: %q", loc)
	}
	if zlSubject(t, e, alice.ID) != "sub-alice@example.com" {
		t.Fatal("not linked again")
	}
}

// One client address cannot fill the table of pending sign-ins.
func TestZZSSOFlowsPerAddress(t *testing.T) {
	var rt ssoRuntime
	base := time.Now().Add(time.Minute)
	for i := 0; i < oidcMaxFlowsPerIP+5; i++ {
		if !rt.putFlow(itoa(int64(i)), &oidcFlow{ip: "198.51.100.7", exp: base.Add(time.Duration(i) * time.Second)}) {
			t.Fatalf("flow %d refused", i)
		}
	}
	if len(rt.flows) != oidcMaxFlowsPerIP {
		t.Fatalf("%d flows kept", len(rt.flows))
	}
	if rt.flows["0"] != nil || rt.flows[itoa(oidcMaxFlowsPerIP+4)] == nil {
		t.Fatal("the oldest flows must make room")
	}
	if !rt.putFlow("other", &oidcFlow{ip: "198.51.100.8", exp: base}) || len(rt.flows) != oidcMaxFlowsPerIP+1 {
		t.Fatal("another address is not affected")
	}
	if got := flowClientKey("2001:db8:1:2:3:4:5:6"); got != "2001:db8:1:2::/64" {
		t.Fatalf("IPv6 key %q", got)
	}
	if got := flowClientKey("192.0.2.1"); got != "192.0.2.1" {
		t.Fatalf("IPv4 key %q", got)
	}

	// through the handler: the flow remembers the client address
	e := newSSOEnv(t, nil)
	zaStart(t, e, e.browser())
	e.s.ssoRT.mu.Lock()
	defer e.s.ssoRT.mu.Unlock()
	for _, f := range e.s.ssoRT.flows {
		if f.ip == "" {
			t.Fatal("flow without client address")
		}
	}
}
