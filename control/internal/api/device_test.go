package api

import (
	"context"
	"net/http"
	"testing"
	"time"

	"corvus/control/internal/auth"
)

// A browser that signed in to an account before is not kept out by an account lock
// caused by failures elsewhere; it has its own count (#142).
func TestKnownDeviceSkipsAccountLock(t *testing.T) {
	e := newTestEnv(t)
	e.s.DataKey = make([]byte, 32)
	e.s.Limiter = auth.NewLimiter(100, time.Minute)
	e.s.AcctLimiter = auth.NewLimiter(3, time.Minute)
	e.h = e.s.Handler()
	const pw = "Correct-horse-battery-9"
	hash, err := auth.HashPassword(pw)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Store.CreateUser(context.Background(), "op@example.com", "Op", hash, "operator"); err != nil {
		t.Fatal(err)
	}
	login := func(email, pass string, dev *http.Cookie) (int, *http.Cookie) {
		opts := []reqOpt{withCSRF()}
		if dev != nil {
			opts = append(opts, func(r *http.Request) { r.AddCookie(dev) })
		}
		w, _ := e.do("POST", "/api/auth/login", map[string]string{"email": email, "password": pass}, opts...)
		for _, c := range w.Result().Cookies() {
			if c.Name == deviceCookie {
				return w.Code, c
			}
		}
		return w.Code, nil
	}

	code, dev := login("op@example.com", pw, nil)
	if code != 200 || dev == nil || dev.Path != devicePath || !dev.HttpOnly {
		t.Fatalf("first sign-in: %d, cookie %+v", code, dev)
	}

	// failures from another browser lock the account for everyone else
	for i := 0; i < 3; i++ {
		if c, _ := login("OP@example.com", "wrong", nil); c != 401 {
			t.Fatalf("failure %d: %d", i, c)
		}
	}
	if c, _ := login("op@example.com", pw, nil); c != 429 {
		t.Fatalf("unknown browser while locked: %d, want 429", c)
	}
	// ... but not for the browser that signed in before
	if c, _ := login("op@example.com", pw, dev); c != 200 {
		t.Fatalf("known browser while locked: %d, want 200", c)
	}

	// the cookie is bound to its account and cannot be altered
	if c, _ := login("other@example.com", "wrong", dev); c != 401 {
		t.Fatalf("other account: %d", c)
	}
	forged := *dev
	forged.Value = forged.Value[:len(forged.Value)-2] + "xx"
	if _, ok := e.s.knownDevice(reqWith(&forged), "op@example.com"); ok {
		t.Fatal("forged cookie accepted")
	}
	if _, ok := e.s.knownDevice(reqWith(dev), "other@example.com"); ok {
		t.Fatal("cookie accepted for another account")
	}

	// a known browser still has a limit of its own
	for i := 0; i < 3; i++ {
		login("op@example.com", "wrong", dev)
	}
	if c, _ := login("op@example.com", pw, dev); c != 429 {
		t.Fatalf("known browser after its own failures: %d, want 429", c)
	}
}

func reqWith(c *http.Cookie) *http.Request {
	r, _ := http.NewRequest("POST", "/api/auth/login", nil)
	r.AddCookie(c)
	return r
}
