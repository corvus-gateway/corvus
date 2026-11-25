package store

import (
	"context"
	"strings"
	"testing"
)

// Sign-in users (0035): users, groups and memberships; a host's login settings keep the
// PIN hash in the database but never serialize it.
func TestSigninUsersStore(t *testing.T) {
	s := openTest(t)
	ctx := context.Background()

	var n int
	if err := s.q.QueryRowContext(ctx, `SELECT COUNT(*) FROM schema_migrations WHERE version = '0035_signin_users'`).Scan(&n); err != nil || n != 1 {
		t.Fatalf("migration 0035: %d %v", n, err)
	}

	g := &SigninGroup{Name: "Family"}
	if err := s.SaveSigninGroup(ctx, g); err != nil {
		t.Fatal(err)
	}
	u := &SigninUser{Username: "Anna", PasswordHash: "$2a$12$x", Enabled: true, GroupIDs: []int64{g.ID}}
	if err := s.SaveSigninUser(ctx, u); err != nil {
		t.Fatal(err)
	}
	if u.Username != "anna" || u.SessionVersion != 1 {
		t.Fatalf("saved user: %+v", u)
	}
	got, err := s.SigninUserByName(ctx, "ANNA")
	if err != nil || got.ID != u.ID || len(got.GroupIDs) != 1 || got.GroupIDs[0] != g.ID || got.LastLoginAt != nil {
		t.Fatalf("by name: %+v %v", got, err)
	}
	if err := s.SaveSigninUser(ctx, &SigninUser{Username: "anna", PasswordHash: "x", Enabled: true}); err == nil || !IsUnique(err) {
		t.Fatalf("duplicate name: %v", err)
	}
	if err := s.TouchSigninUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	got, _ = s.SigninUserByID(ctx, u.ID)
	if got.LastLoginAt == nil {
		t.Fatal("last login not recorded")
	}

	groups, err := s.ListSigninGroups(ctx)
	if err != nil || len(groups) != 1 || len(groups[0].UserIDs) != 1 || groups[0].Hosts != 0 {
		t.Fatalf("groups: %+v %v", groups, err)
	}

	// a host allowing the group: counted, and the group cannot be deleted
	h := &ProxyHost{Domains: []string{"a.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true,
		Login: HostLogin{Enabled: true, Mode: LoginUsers, GroupIDs: []int64{g.ID}, SessionHours: 168, ExcludePaths: []string{"/api/"}}}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	groups, _ = s.ListSigninGroups(ctx)
	if groups[0].Hosts != 1 {
		t.Fatalf("group use: %+v", groups)
	}
	if err := s.DeleteSigninGroup(ctx, g.ID); err == nil {
		t.Fatal("deleted a group in use")
	}

	// PIN: stored, read back, never serialized
	h.Login = HostLogin{Enabled: true, Mode: LoginPIN, SessionHours: 24, PINHash: "$2a$12$pin", PINVersion: 3}
	if err := s.SaveProxyHost(ctx, h); err != nil {
		t.Fatal(err)
	}
	back, _ := s.ProxyHostByID(ctx, h.ID)
	if back.Login.PINHash != "$2a$12$pin" || back.Login.PINVersion != 3 || !back.Login.PINSet || back.Login.Mode != LoginPIN {
		t.Fatalf("pin round trip: %+v", back.Login)
	}
	if js := toJSON(back); strings.Contains(js, "pin_hash") || strings.Contains(js, "$2a$12$pin") || !strings.Contains(js, `"pin_set":true`) {
		t.Fatalf("host JSON: %s", js)
	}

	// hosts without a login page store "{}" and read back empty lists
	plain := &ProxyHost{Domains: []string{"b.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 80, Enabled: true}
	if err := s.SaveProxyHost(ctx, plain); err != nil {
		t.Fatal(err)
	}
	var raw string
	_ = s.q.QueryRowContext(ctx, `SELECT login FROM proxy_hosts WHERE id = ?`, plain.ID).Scan(&raw)
	back, _ = s.ProxyHostByID(ctx, plain.ID)
	if raw != "{}" || back.Login.Enabled || back.Login.GroupIDs == nil || back.Login.ExcludePaths == nil {
		t.Fatalf("no login: %q %+v", raw, back.Login)
	}

	// deleting a user removes its memberships; deleting an unused group works
	if err := s.DeleteSigninUser(ctx, u.ID); err != nil {
		t.Fatal(err)
	}
	groups, _ = s.ListSigninGroups(ctx)
	if len(groups[0].UserIDs) != 0 {
		t.Fatalf("membership left: %+v", groups)
	}
	h.Login = HostLogin{}
	_ = s.SaveProxyHost(ctx, h)
	if err := s.DeleteSigninGroup(ctx, g.ID); err != nil {
		t.Fatal(err)
	}
	if err := s.DeleteSigninGroup(ctx, g.ID); err != ErrNotFound {
		t.Fatalf("delete twice: %v", err)
	}
}
