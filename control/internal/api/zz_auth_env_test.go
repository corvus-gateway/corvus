package api

import (
	"context"
	"crypto/hmac"
	"crypto/sha1"
	"database/sql"
	"encoding/base32"
	"encoding/binary"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
)

// Helpers shared by the zz_auth_* tests.

// zaPassword is the password of users created with zaUser.
const zaPassword = "Correct-horse-battery-9"

var (
	zaHashOnce sync.Once
	zaHashVal  string
)

// zaHash returns a bcrypt hash of zaPassword (computed once: bcrypt is slow on purpose).
func zaHash(t *testing.T) string {
	t.Helper()
	zaHashOnce.Do(func() {
		h, err := auth.HashPassword(zaPassword)
		if err != nil {
			panic(err)
		}
		zaHashVal = h
	})
	return zaHashVal
}

// newAuthEnv is newFullEnv with a database file whose path is known, so tests can
// break the database from a second connection to reach error paths.
func newAuthEnv(t *testing.T) (*fullEnv, string) {
	t.Helper()
	e := newFullEnv(t)
	path := filepath.Join(t.TempDir(), "auth.db")
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	admin, err := st.CreateUser(ctx, "admin@example.com", "Admin", zaHash(t), "admin")
	if err != nil {
		t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := st.CreateSession(ctx, auth.TokenHash(raw), admin.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	e.s.Store = st
	e.s.Deployer.Store = st
	e.admin, e.cookie = admin, raw
	e.h = e.s.Handler()
	return e, path
}

// zaExec runs SQL on the database file through a separate connection.
func zaExec(t *testing.T, path, q string) {
	t.Helper()
	db, err := sql.Open("sqlite", "file:"+path+"?_pragma=busy_timeout(5000)")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if _, err := db.Exec(q); err != nil {
		t.Fatalf("%s: %v", q, err)
	}
}

// zaUser creates a user whose password is zaPassword and returns it with a session.
func zaUser(t *testing.T, s *Server, email, role string) (*store.User, string) {
	t.Helper()
	ctx := context.Background()
	u, err := s.Store.CreateUser(ctx, email, strings.SplitN(email, "@", 2)[0], zaHash(t), role)
	if err != nil {
		t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := s.Store.CreateSession(ctx, auth.TokenHash(raw), u.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	return u, raw
}

// zaTOTP is the RFC 6238 code (SHA-1, 6 digits, 30 s) of a base32 secret at time at.
func zaTOTP(t *testing.T, secret string, at time.Time) string {
	t.Helper()
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		t.Fatal(err)
	}
	var msg [8]byte
	binary.BigEndian.PutUint64(msg[:], uint64(at.Unix()/30))
	m := hmac.New(sha1.New, key)
	m.Write(msg[:])
	sum := m.Sum(nil)
	off := sum[len(sum)-1] & 0x0f
	code := binary.BigEndian.Uint32(sum[off:off+4]) & 0x7fffffff
	return fmt.Sprintf("%06d", code%1_000_000)
}

// zaWrongTOTP returns a 6-digit code that matches none of the accepted time steps.
func zaWrongTOTP(t *testing.T, secret string) string {
	t.Helper()
	now := time.Now()
	valid := map[string]bool{}
	for _, d := range []time.Duration{-60 * time.Second, -30 * time.Second, 0, 30 * time.Second, 60 * time.Second} {
		valid[zaTOTP(t, secret, now.Add(d))] = true
	}
	for i := 0; ; i++ {
		c := fmt.Sprintf("%06d", i)
		if !valid[c] {
			return c
		}
	}
}

// zaCookie returns the cookie named name set by a response, or nil.
func zaCookie(w *httptest.ResponseRecorder, name string) *http.Cookie {
	for _, c := range w.Result().Cookies() {
		if c.Name == name {
			return c
		}
	}
	return nil
}

// zaAudited counts audit entries with action.
func zaAudited(t *testing.T, s *Server, action string) int {
	t.Helper()
	list, err := s.Store.ListAudit(context.Background(), 1000)
	if err != nil {
		t.Fatal(err)
	}
	n := 0
	for _, a := range list {
		if a.Action == action {
			n++
		}
	}
	return n
}

// zaRemote sets the client address of a request.
func zaRemote(addr string) reqOpt { return func(r *http.Request) { r.RemoteAddr = addr } }

// zaHeader sets a request header.
func zaHeader(k, v string) reqOpt { return func(r *http.Request) { r.Header.Set(k, v) } }

// zaRawBody replaces the request body (for malformed JSON).
func zaRawBody(b string) reqOpt {
	return func(r *http.Request) {
		r.Body = http.NoBody
		if b != "" {
			r.Body = zaNopCloser{strings.NewReader(b)}
		}
	}
}

type zaNopCloser struct{ *strings.Reader }

func (zaNopCloser) Close() error { return nil }
