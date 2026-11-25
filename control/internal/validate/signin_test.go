package validate

import (
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestHostLoginValidation(t *testing.T) {
	// disabled: defaults filled in, nothing checked
	l := store.HostLogin{Mode: "nonsense", ExcludePaths: []string{"bad"}}
	var errs Errors
	hostLogin(&errs, &l)
	if len(errs) != 0 || l.SessionHours != DefaultSessionHours || l.GroupIDs == nil {
		t.Fatalf("disabled: %v %+v", errs, l)
	}

	ok := store.HostLogin{Enabled: true, Mode: store.LoginUsers, GroupIDs: []int64{3, 3, 0, 1},
		ExcludePaths: []string{" /api/", "/api/", "", "/hooks"}, PassUser: true}
	errs = nil
	hostLogin(&errs, &ok)
	if len(errs) != 0 || len(ok.GroupIDs) != 2 || strings.Join(ok.ExcludePaths, ",") != "/api/,/hooks" || !ok.PassUser {
		t.Fatalf("users mode: %v %+v", errs, ok)
	}

	pin := store.HostLogin{Enabled: true, Mode: store.LoginPIN, PINHash: "h", PassUser: true}
	errs = nil
	hostLogin(&errs, &pin)
	if len(errs) != 0 || pin.PassUser {
		t.Fatalf("pin mode: %v %+v", errs, pin)
	}

	for name, c := range map[string]store.HostLogin{
		"pin missing":  {Enabled: true, Mode: store.LoginPIN},
		"no groups":    {Enabled: true, Mode: store.LoginUsers},
		"bad mode":     {Enabled: true, Mode: "x"},
		"too long":     {Enabled: true, Mode: store.LoginPIN, PINHash: "h", SessionHours: MaxSessionHours + 1},
		"slash":        {Enabled: true, Mode: store.LoginPIN, PINHash: "h", ExcludePaths: []string{"/"}},
		"reserved":     {Enabled: true, Mode: store.LoginPIN, PINHash: "h", ExcludePaths: []string{"/__corvus/login"}},
		"relative":     {Enabled: true, Mode: store.LoginPIN, PINHash: "h", ExcludePaths: []string{"api"}},
		"traversal":    {Enabled: true, Mode: store.LoginPIN, PINHash: "h", ExcludePaths: []string{"/a/../b"}},
		"many":         {Enabled: true, Mode: store.LoginPIN, PINHash: "h", ExcludePaths: make([]string, MaxExcludePaths+1)},
		"negative ttl": {Enabled: true, Mode: store.LoginPIN, PINHash: "h", SessionHours: -1},
	} {
		errs = nil
		c := c
		hostLogin(&errs, &c)
		if len(errs) == 0 {
			t.Fatalf("%s: accepted %+v", name, c)
		}
	}
}

func TestPINAndSigninUserValidation(t *testing.T) {
	if PIN("4711") != nil || PIN("123") == nil || PIN(strings.Repeat("x", 73)) == nil {
		t.Fatal("PIN rules")
	}
	u := &store.SigninUser{Username: " Anna.B@Example.com "}
	if err := SigninUser(u, "long enough"); err != nil || u.Username != "anna.b@example.com" || u.GroupIDs == nil {
		t.Fatalf("user: %v %+v", err, u)
	}
	for name, c := range map[string]struct {
		name, pw, hash string
	}{
		"no password": {"bob", "", ""},
		"weak":        {"bob", "short", ""},
		"bad name":    {"-bob", "long enough", ""},
		"space":       {"bo b", "long enough", ""},
		"long":        {"bob", strings.Repeat("x", 73), ""},
	} {
		if SigninUser(&store.SigninUser{Username: c.name, PasswordHash: c.hash}, c.pw) == nil {
			t.Fatalf("%s accepted", name)
		}
	}
	// an existing user keeps its password
	if err := SigninUser(&store.SigninUser{Username: "bob", PasswordHash: "h"}, ""); err != nil {
		t.Fatal(err)
	}
	if SigninGroup(&store.SigninGroup{Name: " Family 2 "}) != nil || SigninGroup(&store.SigninGroup{Name: ""}) == nil {
		t.Fatal("group names")
	}
}
