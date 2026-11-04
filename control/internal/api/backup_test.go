package api

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
)

// newBackupEnv is a test server whose database lives in a real data directory.
func newBackupEnv(t *testing.T) (*testEnv, string) {
	t.Helper()
	d := t.TempDir()
	st, err := store.Open(filepath.Join(d, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	ctx := context.Background()
	admin, err := st.CreateUser(ctx, "admin@example.com", "Admin", "x", "admin")
	if err != nil {
		t.Fatal(err)
	}
	raw := auth.RandomToken(32)
	if err := st.CreateSession(ctx, auth.TokenHash(raw), admin.ID, time.Hour, "127.0.0.1", "test"); err != nil {
		t.Fatal(err)
	}
	for p, body := range map[string]string{"secrets/data_key": "KEY", "certs/default/fullchain.pem": "CERT", "nginx/x.conf": "x", "history/traffic.db": "h"} {
		full := filepath.Join(d, filepath.FromSlash(p))
		os.MkdirAll(filepath.Dir(full), 0o700)
		os.WriteFile(full, []byte(body), 0o600)
	}
	s := &Server{Store: st, Traffic: traffic.New(), Version: "1.2.3-test"}
	s.Paths.DataDir = filepath.ToSlash(d)
	return &testEnv{t: t, s: s, h: s.Handler(), admin: admin, cookie: raw}, d
}

func (e *testEnv) upload(archive []byte, opts ...reqOpt) (*httptest.ResponseRecorder, map[string]any) {
	e.t.Helper()
	var body bytes.Buffer
	mw := multipart.NewWriter(&body)
	fw, _ := mw.CreateFormFile("file", "backup.tar.gz")
	fw.Write(archive)
	mw.Close()
	r := httptest.NewRequest("POST", "/api/backup/restore", &body)
	r.Header.Set("Content-Type", mw.FormDataContentType())
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	e.h.ServeHTTP(w, r)
	var m map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &m)
	return w, m
}

func TestBackupDownloadAndRestore(t *testing.T) {
	e, d := newBackupEnv(t)
	w, _ := e.do("GET", "/api/backup", nil, withCookie(e.cookie))
	if w.Code != 200 {
		t.Fatalf("download: %d %s", w.Code, w.Body)
	}
	if cd := w.Header().Get("Content-Disposition"); !strings.Contains(cd, "corvus-backup-1.2.3-test-") || !strings.HasSuffix(cd, `.tar.gz"`) {
		t.Fatalf("Content-Disposition %q", cd)
	}
	if w.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("backup must not be cached")
	}
	archive := w.Body.Bytes()
	names := map[string]bool{}
	gz, err := gzip.NewReader(bytes.NewReader(archive))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatal(err)
		}
		names[h.Name] = true
		if h.Name == "corvus.db" { // the snapshot is a complete, openable database
			dbPath := filepath.Join(t.TempDir(), "restored.db")
			b, _ := io.ReadAll(tr)
			os.WriteFile(dbPath, b, 0o600)
			rs, err := store.Open(dbPath)
			if err != nil {
				t.Fatalf("open snapshot: %v", err)
			}
			n, err := rs.CountUsers(context.Background())
			rs.Close()
			if err != nil || n != 1 {
				t.Fatalf("snapshot users: %d %v", n, err)
			}
		}
	}
	if !names["corvus.db"] || !names["secrets/data_key"] || !names["certs/default/fullchain.pem"] || !names["manifest.json"] || names["nginx/x.conf"] || names["history/traffic.db"] {
		t.Fatalf("archive entries %v", names)
	}
	list, _ := e.s.Store.ListAudit(context.Background(), 5)
	if len(list) == 0 || list[0].Action != "backup.download" {
		t.Fatalf("download not audited: %+v", list)
	}

	// Upload → summary → confirm → pending.
	w, m := e.upload(archive, withCookie(e.cookie), withCSRF())
	if w.Code != 200 || m["state"] != "uploaded" {
		t.Fatalf("upload: %d %s", w.Code, w.Body)
	}
	sum := m["summary"].(map[string]any)
	if sum["version"] != "1.2.3-test" || sum["has_db"] != true || sum["certs"].(float64) != 1 {
		t.Fatalf("summary %v", sum)
	}
	w, m = e.do("POST", "/api/backup/restore/confirm", nil, withCookie(e.cookie), withCSRF())
	if w.Code != 200 || m["state"] != "pending" || m["restart_command"] != "docker compose restart" {
		t.Fatalf("confirm: %d %s", w.Code, w.Body)
	}
	if _, err := os.Stat(filepath.Join(d, "restore-pending", "corvus.db")); err != nil {
		t.Fatal("restore not staged")
	}
	w, m = e.do("GET", "/api/backup/restore", nil, withCookie(e.cookie))
	if w.Code != 200 || m["state"] != "pending" {
		t.Fatalf("status: %d %s", w.Code, w.Body)
	}
	w, _ = e.do("DELETE", "/api/backup/restore", nil, withCookie(e.cookie), withCSRF())
	if w.Code != 204 {
		t.Fatalf("discard: %d", w.Code)
	}
	if _, err := os.Stat(filepath.Join(d, "restore-pending")); err == nil {
		t.Fatal("discard left the staged restore")
	}

	// Garbage is rejected with a clear error.
	w, m = e.upload([]byte("not a backup"), withCookie(e.cookie), withCSRF())
	if w.Code != 422 || m["error"] != "invalid_backup" {
		t.Fatalf("garbage upload: %d %s", w.Code, w.Body)
	}
	// Missing CSRF header.
	w, _ = e.upload(archive, withCookie(e.cookie))
	if w.Code != 403 {
		t.Fatalf("upload without CSRF header: %d", w.Code)
	}
}

func TestBackupPermissions(t *testing.T) {
	e, _ := newBackupEnv(t)
	ctx := context.Background()
	op, _ := e.s.Store.CreateUser(ctx, "op@example.com", "Op", "x", "operator")
	opRaw := auth.RandomToken(32)
	_ = e.s.Store.CreateSession(ctx, auth.TokenHash(opRaw), op.ID, time.Hour, "", "")
	adminTok, _ := e.createToken("ci", "admin", 0)
	// Token ids repeat across test databases: don't leave this token's last-used throttle behind.
	t.Cleanup(func() { tokenTouched.Range(func(k, _ any) bool { tokenTouched.Delete(k); return true }) })

	for _, c := range []struct {
		name string
		opt  reqOpt
		code int
	}{
		{"anonymous", func(*http.Request) {}, 401},
		{"operator session", withCookie(opRaw), 403},
		{"admin API token", withBearer(adminTok), 403},
	} {
		for _, rq := range []struct{ method, path string }{
			{"GET", "/api/backup"}, {"GET", "/api/backup/restore"}, {"POST", "/api/backup/restore/confirm"}, {"DELETE", "/api/backup/restore"},
		} {
			w, _ := e.do(rq.method, rq.path, nil, c.opt, withCSRF())
			if w.Code != c.code {
				t.Errorf("%s %s %s: %d, want %d", c.name, rq.method, rq.path, w.Code, c.code)
			}
		}
		w, _ := e.upload([]byte("x"), c.opt, withCSRF())
		if w.Code != c.code {
			t.Errorf("%s upload: %d, want %d", c.name, w.Code, c.code)
		}
	}
}
