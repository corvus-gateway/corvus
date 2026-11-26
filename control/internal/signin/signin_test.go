package signin

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

type env struct {
	t   *testing.T
	st  *store.Store
	svc *Service
	now time.Time
	ctx context.Context
}

func newEnv(t *testing.T) *env {
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	key := make([]byte, 32)
	_, _ = rand.Read(key)
	e := &env{t: t, st: st, ctx: context.Background(), now: time.Date(2026, 11, 25, 12, 0, 0, 0, time.UTC)}
	e.svc = &Service{Store: st, DataKey: key, IP: auth.NewLimiter(5, 10*time.Minute), Account: auth.NewLimiter(3, 15*time.Minute),
		Now: func() time.Time { return e.now }}
	return e
}

func (e *env) host(login store.HostLogin) *store.ProxyHost {
	e.t.Helper()
	h := &store.ProxyHost{Domains: []string{"photos.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true, Login: login}
	if err := e.st.SaveProxyHost(e.ctx, h); err != nil {
		e.t.Fatal(err)
	}
	return h
}

func (e *env) user(name, pw string, totp bool, groups ...int64) *store.SigninUser {
	e.t.Helper()
	hash, err := auth.HashSecret(pw)
	if err != nil {
		e.t.Fatal(err)
	}
	u := &store.SigninUser{Username: name, PasswordHash: hash, Enabled: true, TOTPRequired: totp, GroupIDs: groups}
	if err := e.st.SaveSigninUser(e.ctx, u); err != nil {
		e.t.Fatal(err)
	}
	return u
}

func (e *env) group(name string) int64 {
	e.t.Helper()
	g := &store.SigninGroup{Name: name}
	if err := e.st.SaveSigninGroup(e.ctx, g); err != nil {
		e.t.Fatal(err)
	}
	return g.ID
}

func (e *env) sign(in Request) Response {
	e.t.Helper()
	if in.Client == "" {
		in.Client = "203.0.113.5"
	}
	r, err := e.svc.SignIn(e.ctx, in)
	if err != nil {
		e.t.Fatal(err)
	}
	return r
}

func code(secret string, at time.Time) string {
	for c := 0; c < 1000000; c++ {
		s := strings.Repeat("0", 6-len(itoa(c))) + itoa(c)
		if auth.TOTPStep(secret, s, at) == at.Unix()/30 {
			return s
		}
	}
	return ""
}

func itoa(n int) string {
	b, _ := json.Marshal(n)
	return string(b)
}

func TestPINSignIn(t *testing.T) {
	e := newEnv(t)
	pin, _ := auth.HashSecret("4711")
	h := e.host(store.HostLogin{Enabled: true, Mode: store.LoginPIN, PINHash: pin, PINVersion: 3, SessionHours: 24})

	if r := e.sign(Request{HostID: h.ID, Secret: "4711"}); r.Result != ResultOK || r.Version != 3 || r.Hours != 24 || r.UserID != 0 || r.Event != "login_succeeded" {
		t.Fatalf("pin ok: %+v", r)
	}
	if r := e.sign(Request{HostID: h.ID, Secret: "1234"}); r.Result != ResultFail || r.Event != "login_failed" {
		t.Fatalf("wrong pin: %+v", r)
	}
	// the client is limited after 5 failures, even with the right PIN
	for i := 0; i < 4; i++ {
		e.sign(Request{HostID: h.ID, Secret: "x"})
	}
	if r := e.sign(Request{HostID: h.ID, Secret: "4711"}); r.Result != ResultLimited || r.RetryAfter <= 0 {
		t.Fatalf("limited: %+v", r)
	}
	// another client is not
	if r := e.sign(Request{HostID: h.ID, Secret: "4711", Client: "198.51.100.9"}); r.Result != ResultOK {
		t.Fatalf("other client: %+v", r)
	}
	// login page turned off, or host gone
	h.Login.Enabled = false
	_ = e.st.SaveProxyHost(e.ctx, h)
	if r := e.sign(Request{HostID: h.ID, Secret: "4711", Client: "198.51.100.9"}); r.Result != ResultUnavailable {
		t.Fatalf("disabled: %+v", r)
	}
	if r := e.sign(Request{HostID: 999, Secret: "4711"}); r.Result != ResultUnavailable {
		t.Fatalf("no host: %+v", r)
	}
}

func TestUserSignIn(t *testing.T) {
	e := newEnv(t)
	fam, other := e.group("family"), e.group("other")
	h := e.host(store.HostLogin{Enabled: true, Mode: store.LoginUsers, GroupIDs: []int64{fam}, SessionHours: 168, PassUser: true})
	anna := e.user("anna", "correct horse", false, fam)
	e.user("olaf", "correct horse", false, other)

	r := e.sign(Request{HostID: h.ID, Username: " ANNA ", Secret: "correct horse"})
	if r.Result != ResultOK || r.UserID != anna.ID || r.User != "anna" || r.Version != 1 || !r.PassUser || r.Hours != 168 {
		t.Fatalf("ok: %+v", r)
	}
	if u, _ := e.st.SigninUserByID(e.ctx, anna.ID); u.LastLoginAt == nil {
		t.Fatal("last login not recorded")
	}
	// wrong password, unknown user, user of another group, disabled user: all just "fail"
	for name, in := range map[string]Request{
		"password":    {Username: "anna", Secret: "wrong"},
		"unknown":     {Username: "nobody", Secret: "correct horse"},
		"other group": {Username: "olaf", Secret: "correct horse"},
	} {
		in.HostID, in.Client = h.ID, "192.0.2."+itoa(len(name))
		if r := e.sign(in); r.Result != ResultFail {
			t.Fatalf("%s: %+v", name, r)
		}
	}
	anna.Enabled = false
	_ = e.st.SaveSigninUser(e.ctx, anna)
	if r := e.sign(Request{HostID: h.ID, Username: "anna", Secret: "correct horse", Client: "192.0.2.50"}); r.Result != ResultFail {
		t.Fatalf("disabled user: %+v", r)
	}

	// the account is locked after 3 failures from different clients
	bob := e.user("bob", "bobs password", false, fam)
	for i := 0; i < 3; i++ {
		e.sign(Request{HostID: h.ID, Username: "bob", Secret: "guess", Client: "198.51.100." + itoa(i)})
	}
	if r := e.sign(Request{HostID: h.ID, Username: "bob", Secret: "bobs password", Client: "198.51.100.77"}); r.Result != ResultLimited {
		t.Fatalf("account lock: %+v", r)
	}
	_ = bob
}

func TestTOTPEnrolAndSignIn(t *testing.T) {
	e := newEnv(t)
	fam := e.group("family")
	h := e.host(store.HostLogin{Enabled: true, Mode: store.LoginUsers, GroupIDs: []int64{fam}, SessionHours: 168})
	other := e.host(store.HostLogin{Enabled: true, Mode: store.LoginUsers, GroupIDs: []int64{fam}, SessionHours: 168})
	u := e.user("anna", "correct horse", true, fam)

	// first sign-in: enrol
	r := e.sign(Request{HostID: h.ID, Host: "photos.example.com", Username: "anna", Secret: "correct horse"})
	if r.Result != ResultEnroll || r.Step == "" || r.KeyText == "" || !strings.HasPrefix(r.QRPNG, "data:image/png;base64,") {
		t.Fatalf("enroll: %+v", r)
	}
	secret, step := r.KeyText, r.Step
	// a step is bound to its host
	if r := e.sign(Request{HostID: other.ID, Step: step, Code: code(secret, e.now)}); r.Result != ResultFail {
		t.Fatalf("step on another host: %+v", r)
	}
	// a wrong code keeps the enrolment
	if r := e.sign(Request{HostID: h.ID, Step: step, Code: "000000"}); r.Result != ResultEnroll || r.KeyText != secret || r.Step != step {
		t.Fatalf("wrong enrol code: %+v", r)
	}
	if r := e.sign(Request{HostID: h.ID, Step: step, Code: code(secret, e.now)}); r.Result != ResultOK || r.UserID != u.ID {
		t.Fatalf("enrolled: %+v", r)
	}
	stored, _ := e.st.SigninUserByID(e.ctx, u.ID)
	if got, err := seal.Open(e.svc.DataKey, stored.TOTPSecret); err != nil || got != secret {
		t.Fatalf("stored secret: %q %v", got, err)
	}
	// the step is used up
	if r := e.sign(Request{HostID: h.ID, Step: step, Code: code(secret, e.now)}); r.Result != ResultFail {
		t.Fatalf("step reused: %+v", r)
	}

	// next sign-in: password, then the code; the same code cannot be used again
	e.now = e.now.Add(time.Minute)
	r = e.sign(Request{HostID: h.ID, Username: "anna", Secret: "correct horse"})
	if r.Result != ResultTOTP || r.Step == "" || r.KeyText != "" || r.QRPNG != "" {
		t.Fatalf("totp: %+v", r)
	}
	c := code(secret, e.now)
	if r := e.sign(Request{HostID: h.ID, Step: r.Step, Code: c}); r.Result != ResultOK {
		t.Fatalf("totp ok: %+v", r)
	}
	r = e.sign(Request{HostID: h.ID, Username: "anna", Secret: "correct horse"})
	if r := e.sign(Request{HostID: h.ID, Step: r.Step, Code: c}); r.Result == ResultOK {
		t.Fatalf("replayed code accepted: %+v", r)
	}

	// five wrong codes use the step up
	e.now = e.now.Add(2 * time.Minute)
	r = e.sign(Request{HostID: h.ID, Username: "anna", Secret: "correct horse", Client: "198.51.100.1"})
	st := r.Step
	for i := 0; i < 4; i++ {
		if r := e.sign(Request{HostID: h.ID, Step: st, Code: "111111", Client: "198.51.100.1"}); r.Result != ResultTOTP && r.Result != ResultLimited {
			t.Fatalf("wrong code %d: %+v", i, r)
		}
	}
	e.svc.IP.Reset("ip:198.51.100.1")
	e.svc.Account.Reset("user:anna")
	if r := e.sign(Request{HostID: h.ID, Step: st, Code: "111111", Client: "198.51.100.1"}); r.Result != ResultFail {
		t.Fatalf("step not used up: %+v", r)
	}
	if r := e.sign(Request{HostID: h.ID, Step: st, Code: code(secret, e.now), Client: "198.51.100.1"}); r.Result != ResultFail {
		t.Fatalf("used-up step accepted: %+v", r)
	}

	// steps expire
	r = e.sign(Request{HostID: h.ID, Username: "anna", Secret: "correct horse", Client: "198.51.100.2"})
	e.now = e.now.Add(stepTTL + time.Second)
	if r := e.sign(Request{HostID: h.ID, Step: r.Step, Code: code(secret, e.now), Client: "198.51.100.2"}); r.Result != ResultFail {
		t.Fatalf("expired step: %+v", r)
	}
}

func TestHandler(t *testing.T) {
	e := newEnv(t)
	pin, _ := auth.HashSecret("4711")
	h := e.host(store.HostLogin{Enabled: true, Mode: store.LoginPIN, PINHash: pin, PINVersion: 1, SessionHours: 1})
	srv := httptest.NewServer(e.svc.Handler())
	defer srv.Close()

	b, _ := json.Marshal(Request{HostID: h.ID, Client: "203.0.113.5", Secret: "4711"})
	resp, err := http.Post(srv.URL+"/v1/signin", "application/json", bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	var out Response
	_ = json.NewDecoder(resp.Body).Decode(&out)
	resp.Body.Close()
	if resp.StatusCode != 200 || out.Result != ResultOK {
		t.Fatalf("handler: %d %+v", resp.StatusCode, out)
	}
	if resp, _ := http.Get(srv.URL + "/v1/signin"); resp.StatusCode != http.StatusMethodNotAllowed {
		t.Fatalf("GET: %d", resp.StatusCode)
	}
	if resp, _ := http.Post(srv.URL+"/v1/signin", "application/json", strings.NewReader(strings.Repeat("x", 9000))); resp.StatusCode != 400 {
		t.Fatalf("oversized: %d", resp.StatusCode)
	}
}
