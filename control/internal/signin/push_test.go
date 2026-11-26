package signin

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/autoban"
	"corvus/control/internal/store"
)

// fakeGateway records the bodies pushed to PUT /signin-sessions.
type fakeGateway struct {
	mu     sync.Mutex
	bodies []SessionState
	keys   []string
	status int
}

func (g *fakeGateway) handler(t *testing.T) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut || r.URL.Path != "/signin-sessions" || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("unexpected request %s %s %q", r.Method, r.URL.Path, r.Header.Get("Content-Type"))
		}
		b, _ := io.ReadAll(r.Body)
		var st SessionState
		if err := json.Unmarshal(b, &st); err != nil {
			t.Errorf("body: %v", err)
		}
		g.mu.Lock()
		g.bodies = append(g.bodies, st)
		g.keys = append(g.keys, r.Header.Get(autoban.StatusKeyHeader))
		code := g.status
		g.mu.Unlock()
		if code == 0 {
			code = 200
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte("1\n"))
	}
}

func (g *fakeGateway) last() (SessionState, string, int) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if len(g.bodies) == 0 {
		return SessionState{}, "", 0
	}
	return g.bodies[len(g.bodies)-1], g.keys[len(g.keys)-1], len(g.bodies)
}

func TestSessionPush(t *testing.T) {
	e := newEnv(t)
	family, friends := e.group("family"), e.group("friends")
	alice := e.user("alice", "alice-pass-1", false, family)
	bob := e.user("bob", "bob-pass-12", false, friends)
	carol := e.user("carol", "carol-pass-1", false, family, friends)
	pinHost := e.host(store.HostLogin{Enabled: true, Mode: store.LoginPIN, PINHash: "h", PINVersion: 4})
	usersHost := &store.ProxyHost{Domains: []string{"users.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true,
		Login: store.HostLogin{Enabled: true, Mode: store.LoginUsers, GroupIDs: []int64{family}}}
	offHost := &store.ProxyHost{Domains: []string{"off.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true,
		Login: store.HostLogin{Enabled: false, Mode: store.LoginPIN, PINHash: "h"}}
	for _, h := range []*store.ProxyHost{usersHost, offHost} {
		if err := e.st.SaveProxyHost(e.ctx, h); err != nil {
			t.Fatal(err)
		}
	}

	gw := &fakeGateway{}
	srv := httptest.NewServer(gw.handler(t))
	defer srv.Close()
	s := &Sessions{Store: e.st, PushURL: srv.URL + "/signin-sessions", StatusKey: "k1"}
	if err := s.Push(e.ctx); err != nil {
		t.Fatal(err)
	}
	st, key, n := gw.last()
	if n != 1 || key != "k1" {
		t.Fatalf("pushes %d, key %q", n, key)
	}
	id := func(v int64) string { return strconv.FormatInt(v, 10) }
	if len(st.Users) != 3 || st.Users[id(alice.ID)] != 1 || st.Names[id(bob.ID)] != "bob" {
		t.Errorf("users: %+v names %+v", st.Users, st.Names)
	}
	if len(st.Hosts) != 2 || st.Hosts[id(pinHost.ID)] != 4 || st.Hosts[id(usersHost.ID)] != 0 {
		t.Errorf("hosts (login page on only): %+v", st.Hosts)
	}
	if got := st.Access[id(usersHost.ID)]; len(got) != 2 || got[0] != alice.ID || got[1] != carol.ID {
		t.Errorf("access of the users host (family): %v", got)
	}
	if _, ok := st.Access[id(pinHost.ID)]; ok {
		t.Error("PIN host has an access list")
	}

	// signing a user out, disabling one and changing the host's groups change the state
	alice.SessionVersion++
	bob.Enabled = false
	for _, u := range []*store.SigninUser{alice, bob} {
		if err := e.st.SaveSigninUser(e.ctx, u); err != nil {
			t.Fatal(err)
		}
	}
	usersHost.Login.GroupIDs = []int64{friends}
	if err := e.st.SaveProxyHost(e.ctx, usersHost); err != nil {
		t.Fatal(err)
	}
	if err := s.Push(e.ctx); err != nil {
		t.Fatal(err)
	}
	st, _, _ = gw.last()
	if st.Users[id(alice.ID)] != 2 {
		t.Errorf("new session version not pushed: %+v", st.Users)
	}
	if _, ok := st.Users[id(bob.ID)]; ok {
		t.Error("disabled user still pushed")
	}
	if got := st.Access[id(usersHost.ID)]; len(got) != 1 || got[0] != carol.ID {
		t.Errorf("access after the group change (bob disabled): %v", got)
	}

	// gateway errors are reported, with its message
	gw.mu.Lock()
	gw.status = 503
	gw.mu.Unlock()
	if err := s.Push(e.ctx); err == nil {
		t.Error("push to a failing gateway reported success")
	}
	if ss := s.SyncStatus(); ss.ConsecutiveFailures != 1 || ss.LastSuccess == nil {
		t.Errorf("sync status: %+v", ss)
	}
	gw.mu.Lock()
	gw.status = 200
	gw.mu.Unlock()

	// Run pushes at once and again on Trigger
	ctx, cancel := context.WithCancel(e.ctx)
	defer cancel()
	_, _, before := gw.last()
	go s.Run(ctx)
	waitPushes(t, gw, before+1)
	s.Trigger()
	waitPushes(t, gw, before+2)

	// no push URL (no gateway): nothing to do
	if err := (&Sessions{Store: e.st}).Push(e.ctx); err != nil {
		t.Error(err)
	}
	var nilSessions *Sessions
	nilSessions.Trigger() // safe without a push service
}

func waitPushes(t *testing.T, gw *fakeGateway, n int) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, _, got := gw.last(); got >= n {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	_, _, got := gw.last()
	t.Fatalf("pushes: %d, want %d", got, n)
}
