package api

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/backup"
	"corvus/control/internal/store"
)

// ------------------------------------------------------------------ known devices

func TestZZKnownDeviceCookie(t *testing.T) {
	s := &Server{DataKey: bytes.Repeat([]byte{7}, 32)}
	const email = "op@example.com"
	id := strings.Repeat("a", 32)
	future := time.Now().Add(time.Hour).Unix()
	past := time.Now().Add(-time.Hour).Unix()
	cookie := func(v string) *http.Request {
		return reqWith(&http.Cookie{Name: deviceCookie, Value: v})
	}
	valid := id + "." + strconv.FormatInt(future, 10) + "." + s.deviceMAC(email, id, future)
	if got, ok := s.knownDevice(cookie(valid), " OP@example.com "); !ok || got != id {
		t.Fatalf("valid cookie: %q %v", got, ok)
	}
	for name, v := range map[string]string{
		"two parts":    id + "." + strconv.FormatInt(future, 10),
		"short id":     "abc." + strconv.FormatInt(future, 10) + "." + s.deviceMAC(email, "abc", future),
		"bad expiry":   id + ".soon." + s.deviceMAC(email, id, future),
		"expired":      id + "." + strconv.FormatInt(past, 10) + "." + s.deviceMAC(email, id, past),
		"wrong expiry": id + "." + strconv.FormatInt(future+1, 10) + "." + s.deviceMAC(email, id, future),
	} {
		if _, ok := s.knownDevice(cookie(v), email); ok {
			t.Errorf("%s accepted", name)
		}
	}
	if _, ok := s.knownDevice(reqWith(&http.Cookie{Name: "other", Value: valid}), email); ok {
		t.Error("cookie with another name accepted")
	}
	if k := s.accountKey(cookie(valid), email); k != "acct:"+email+"|dev:"+id {
		t.Errorf("accountKey = %q", k)
	}

	// signing in again keeps the browser's id and renews the cookie
	w := httptest.NewRecorder()
	s.rememberDevice(w, cookie(valid), email)
	c := zaCookie(w, deviceCookie)
	if c == nil || !strings.HasPrefix(c.Value, id+".") || c.Path != devicePath || c.MaxAge != int(deviceTTL.Seconds()) {
		t.Fatalf("renewed cookie: %+v", c)
	}

	// without a data key there are no known devices
	none := &Server{}
	w = httptest.NewRecorder()
	none.rememberDevice(w, cookie(valid), email)
	if len(w.Result().Cookies()) != 0 {
		t.Fatal("device cookie set without a data key")
	}
	if _, ok := none.knownDevice(cookie(valid), email); ok {
		t.Fatal("device known without a data key")
	}
	if k := none.accountKey(cookie(valid), email); k != "acct:"+email {
		t.Errorf("accountKey without key = %q", k)
	}
}

// ------------------------------------------------------------------ API tokens

func TestZZTokenValidation(t *testing.T) {
	e := newFullEnv(t)
	t.Cleanup(func() { tokenTouched.Range(func(k, _ any) bool { tokenTouched.Delete(k); return true }) })
	if code, _ := e.call("POST", "/api/tokens", nil); code != 400 {
		t.Fatalf("empty body: %d", code)
	}
	for _, c := range []struct {
		body  map[string]any
		field string
	}{
		{map[string]any{"name": "  ", "role": "viewer"}, "name"},
		{map[string]any{"name": strings.Repeat("n", 65), "role": "viewer"}, "name"},
		{map[string]any{"name": `say "hi"`, "role": "viewer"}, "name"},
		{map[string]any{"name": "tab\there", "role": "viewer"}, "name"},
		{map[string]any{"name": "ci", "role": "root"}, "role"},
		{map[string]any{"name": "ci", "role": "viewer", "expires_days": -1}, "expires_days"},
		{map[string]any{"name": "ci", "role": "viewer", "expires_days": 3651}, "expires_days"},
	} {
		code, m := e.call("POST", "/api/tokens", c.body)
		if code != 422 || zaFields(m)[c.field] != "invalid" {
			t.Errorf("%v: %d %v", c.body, code, m)
		}
	}
	// a token without expiry
	_, m := e.createToken(" never ", "admin", 0)
	if m["name"] != "never" || m["expires_at"] != nil {
		t.Fatalf("token without expiry: %v", m)
	}
	for _, p := range []string{"/api/tokens/x", "/api/tokens/0"} {
		if code, _ := e.call("DELETE", p, nil); code != 400 {
			t.Fatalf("delete %s: %d", p, code)
		}
	}
	if code, m := e.call("DELETE", "/api/tokens/987", nil); code != 404 || m["error"] != "not_found" {
		t.Fatalf("delete unknown: %d %v", code, m)
	}
	if n := zaAudited(t, e.s, "token.create"); n != 1 {
		t.Fatalf("token.create audited %d times", n)
	}
}

func TestZZTokenDatabaseErrors(t *testing.T) {
	e, path := newAuthEnv(t)
	zaExec(t, path, "DROP TABLE api_tokens")
	if code, m := e.call("POST", "/api/tokens", map[string]any{"name": "ci", "role": "viewer"}); code != 500 || m["error"] != "internal" {
		t.Fatalf("create: %d %v", code, m)
	}
	if code, m := e.call("DELETE", "/api/tokens/1", nil); code != 500 || m["error"] != "internal" {
		t.Fatalf("delete: %d %v", code, m)
	}
}

// admSession refuses token principals even on paths that sessionOnly does not list.
func TestZZAdmSessionRefusesTokens(t *testing.T) {
	e := newTestEnv(t)
	t.Cleanup(func() { tokenTouched.Range(func(k, _ any) bool { tokenTouched.Delete(k); return true }) })
	tok, _ := e.createToken("ci", "admin", 0)
	called := false
	h := e.s.admSession(func(w http.ResponseWriter, r *http.Request) { called = true })
	r := httptest.NewRequest("GET", "/api/overview", nil)
	r.Header.Set("Authorization", "Bearer "+tok)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 403 || !strings.Contains(w.Body.String(), "session_required") || called {
		t.Fatalf("token on admSession: %d %s called=%v", w.Code, w.Body, called)
	}
	r = httptest.NewRequest("GET", "/api/overview", nil)
	r.AddCookie(&http.Cookie{Name: auth.SessionCookie, Value: e.cookie})
	w = httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 200 || !called {
		t.Fatalf("session on admSession: %d called=%v", w.Code, called)
	}
}

func TestZZAuditDetailForTokens(t *testing.T) {
	r := httptest.NewRequest("GET", "/", nil)
	if got := auditDetail(r, "plain"); got != "plain" {
		t.Fatalf("session request detail changed: %v", got)
	}
	r = r.WithContext(context.WithValue(r.Context(), tokenCtxKey{}, &store.APIToken{Name: "ci"}))
	for _, c := range []struct {
		in   any
		want string
	}{
		{nil, `{"via":"token:ci"}`},
		{"plain", `{"detail":"plain","via":"token:ci"}`},
		{[]int{1, 2}, `{"detail":[1,2],"via":"token:ci"}`},
		{map[string]int{"n": 1}, `{"n":1,"via":"token:ci"}`},
	} {
		if got := w2s(auditDetail(r, c.in)); got != c.want {
			t.Errorf("auditDetail(%v) = %s, want %s", c.in, got, c.want)
		}
	}
}

// ------------------------------------------------------------------ backup & restore

func zaUploadBody(t *testing.T, field string, data []byte) (io.Reader, string) {
	t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	if field != "" {
		fw, _ := mw.CreateFormFile(field, "backup.tar.gz")
		fw.Write(data)
	}
	mw.WriteField("note", "x")
	mw.Close()
	return &body, mw.FormDataContentType()
}

func zaPost(e *testEnv, path string, body io.Reader, ctype string) (*httptest.ResponseRecorder, map[string]any) {
	return e.do("POST", path, nil, withCookie(e.cookie), withCSRF(), func(r *http.Request) {
		r.Body = io.NopCloser(body)
		r.ContentLength = -1
		if ctype != "" {
			r.Header.Set("Content-Type", ctype)
		}
	})
}

func TestZZBackupRequestErrors(t *testing.T) {
	// no data directory configured
	plain := newTestEnv(t)
	if w, m := plain.do("GET", "/api/backup", nil, withCookie(plain.cookie)); w.Code != 503 || m["error"] != "unavailable" {
		t.Fatalf("download without data dir: %d %v", w.Code, m)
	}
	if w, m := plain.upload([]byte("x"), withCookie(plain.cookie), withCSRF()); w.Code != 503 || m["error"] != "unavailable" {
		t.Fatalf("upload without data dir: %d %v", w.Code, m)
	}

	e, d := newBackupEnv(t)
	// not multipart / no file field
	if w, m := zaPost(e, "/api/backup/restore", strings.NewReader(`{"a":1}`), "application/json"); w.Code != 400 || !strings.Contains(m["message"].(string), "multipart") {
		t.Fatalf("JSON upload: %d %v", w.Code, m)
	}
	body, ct := zaUploadBody(t, "", nil)
	if w, m := zaPost(e, "/api/backup/restore", body, ct); w.Code != 400 || !strings.Contains(m["message"].(string), `missing "file"`) {
		t.Fatalf("upload without file: %d %v", w.Code, m)
	}
	// nothing to confirm
	if w, m := e.do("POST", "/api/backup/restore/confirm", nil, withCookie(e.cookie), withCSRF()); w.Code != 404 || m["error"] != "not_found" {
		t.Fatalf("confirm without upload: %d %v", w.Code, m)
	}

	// a backup of a PostgreSQL installation cannot be restored into SQLite
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "secrets"), 0o700)
	os.WriteFile(filepath.Join(src, "secrets", "data_key"), []byte("KEY"), 0o600)
	var pg bytes.Buffer
	if _, err := backup.Create(context.Background(), &pg, backup.CreateOptions{DataDir: src, Version: "1.0.0", Engine: store.Postgres, Now: time.Now()}); err != nil {
		t.Fatal(err)
	}
	w, m := e.upload(pg.Bytes(), withCookie(e.cookie), withCSRF())
	if w.Code != 422 || m["error"] != "invalid_backup" || !strings.Contains(m["message"].(string), "postgres") {
		t.Fatalf("engine mismatch: %d %v", w.Code, m)
	}
	if _, err := os.Stat(filepath.Join(d, backup.UploadDir)); err == nil {
		t.Fatal("mismatching upload kept")
	}

	// a staged upload that was changed on disk is not confirmed
	w, _ = e.do("GET", "/api/backup", nil, withCookie(e.cookie))
	if w.Code != 200 {
		t.Fatalf("download: %d", w.Code)
	}
	archive := w.Body.Bytes()
	if w, m := e.upload(archive, withCookie(e.cookie), withCSRF()); w.Code != 200 || m["state"] != "uploaded" {
		t.Fatalf("upload: %d %v", w.Code, m)
	}
	if err := os.WriteFile(filepath.Join(d, backup.UploadDir, "secrets", "data_key"), []byte("TAMPERED"), 0o600); err != nil {
		t.Fatal(err)
	}
	if w, m := e.do("POST", "/api/backup/restore/confirm", nil, withCookie(e.cookie), withCSRF()); w.Code != 422 || m["error"] != "invalid_backup" {
		t.Fatalf("confirm tampered: %d %v", w.Code, m)
	}
	if _, err := os.Stat(filepath.Join(d, backup.PendingDir)); err == nil {
		t.Fatal("tampered upload was confirmed")
	}

	// the data directory cannot be written: an internal error, not "invalid backup"
	file := filepath.Join(t.TempDir(), "not-a-dir")
	os.WriteFile(file, []byte("x"), 0o600)
	e.s.Paths.DataDir = filepath.ToSlash(file)
	if w, m := e.upload(archive, withCookie(e.cookie), withCSRF()); w.Code != 500 || m["error"] != "internal" {
		t.Fatalf("unwritable data dir: %d %v", w.Code, m)
	}
	// ... and a download cannot produce an archive (the client gets an empty, broken file)
	w, _ = e.do("GET", "/api/backup", nil, withCookie(e.cookie))
	if !strings.HasSuffix(w.Header().Get("Content-Disposition"), `.tar.gz"`) {
		t.Fatalf("download headers: %v", w.Header())
	}
	if _, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes())); err == nil {
		t.Fatal("download from a broken data dir produced a readable archive")
	}
}

func TestZZCapReader(t *testing.T) {
	c := &capReader{r: strings.NewReader("0123456789"), n: 4}
	b, err := io.ReadAll(c)
	if err == nil || !c.over || len(b) > 5 {
		t.Fatalf("over the limit: %q %v over=%v", b, err, c.over)
	}
	c = &capReader{r: strings.NewReader("0123"), n: 10}
	b, err = io.ReadAll(c)
	if err != nil || c.over || string(b) != "0123" {
		t.Fatalf("under the limit: %q %v over=%v", b, err, c.over)
	}
	c = &capReader{r: strings.NewReader("x"), n: 0}
	if _, err := c.Read(make([]byte, 4)); err == nil || !c.over {
		t.Fatal("exhausted reader must fail")
	}
}
