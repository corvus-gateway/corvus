package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"corvus/control/internal/compiler"
	"corvus/control/internal/store"
)

// fakeRunner rejects any configuration containing "REJECT".
type fakeRunner struct{ reloads int }

func (f *fakeRunner) Test(_ context.Context, conf string) (string, error) {
	dir := filepath.Dir(filepath.FromSlash(conf))
	var bad bool
	_ = filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			b, _ := os.ReadFile(p)
			bad = bad || strings.Contains(string(b), "REJECT")
		}
		return nil
	})
	if bad {
		return "nginx: [emerg] unknown directive \"REJECT\"", errors.New("test failed")
	}
	return "ok", nil
}
func (f *fakeRunner) Reload() error { f.reloads++; return nil }

func setup(t *testing.T) (*Deployer, *fakeRunner, string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("atomic symlink swap requires POSIX rename semantics; run on Linux")
	}
	dir := t.TempDir()
	if err := os.Symlink(dir, filepath.Join(dir, "probe")); err != nil {
		t.Skip("symlinks not available:", err)
	}
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	r := &fakeRunner{}
	p := compiler.Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, DefaultCert: "/data/certs/default"}
	return &Deployer{Store: st, Compiler: &compiler.Compiler{P: p}, Runner: r, Dir: filepath.Join(dir, "nginx")}, r, dir
}

func readCurrent(t *testing.T, d *Deployer, name string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(d.Dir, "current", name))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func TestApplyRenderTestAndRollback(t *testing.T) {
	d, r, _ := setup(t)
	ctx := context.Background()
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}

	room := &store.Waitroom{Name: "event", Enabled: true, Capacity: 500, ActiveTTL: 120, GrantTTL: 30, QueueTTL: 120, Refresh: 15, FailMode: "open"}
	host := &store.ProxyHost{Domains: []string{"shop.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, WaitroomPath: "/event", Enabled: true}
	v1, err := d.Apply(ctx, nil, "add host", func(tx *store.Store) error {
		if err := tx.SaveWaitroom(ctx, room); err != nil {
			return err
		}
		host.WaitroomID = &room.ID
		return tx.SaveProxyHost(ctx, host)
	})
	if err != nil {
		t.Fatal(err)
	}
	conf := readCurrent(t, d, "hosts/1.conf")
	for _, want := range []string{"server_name shop.example.com;", `set $corvus_upstream "app";`, `location ~* "^/event(?:[/;]|$)" {`, `set $corvus_room "event";`, "server app:8080 max_fails=0 resolve;", "proxy_pass http://corvus_up_1;"} {
		if !strings.Contains(conf, want) {
			t.Errorf("host config missing %q:\n%s", want, conf)
		}
	}
	if !strings.Contains(readCurrent(t, d, "policy.json"), `"capacity": 500`) {
		t.Error("policy.json missing room")
	}
	if dirty, _ := d.Dirty(ctx); dirty {
		t.Error("dirty right after deploy")
	}

	// A change rejected by the gateway test must leave DB and live config untouched.
	_, err = d.Apply(ctx, nil, "bad", func(tx *store.Store) error {
		h, _ := tx.ProxyHostByID(ctx, host.ID)
		h.AdvancedConfig = "REJECT;"
		return tx.SaveProxyHost(ctx, h)
	})
	var te *TestError
	if !errors.As(err, &te) || te.VersionID == 0 {
		t.Fatalf("expected TestError with recorded version, got %v", err)
	}
	if h, _ := d.Store.ProxyHostByID(ctx, host.ID); h.AdvancedConfig != "" {
		t.Error("rejected change was committed to the database")
	}
	if strings.Contains(readCurrent(t, d, "hosts/1.conf"), "REJECT") {
		t.Error("rejected change went live")
	}
	if fv, _ := d.Store.VersionByID(ctx, te.VersionID); fv.Status != "failed" {
		t.Errorf("failed attempt status = %q", fv.Status)
	}

	// Change capacity, then roll back to v1.
	if _, err := d.Apply(ctx, nil, "capacity", func(tx *store.Store) error {
		w, _ := tx.WaitroomByID(ctx, room.ID)
		w.Capacity = 1000
		return tx.SaveWaitroom(ctx, w)
	}); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readCurrent(t, d, "policy.json"), `"capacity": 1000`) {
		t.Fatal("capacity change not live")
	}
	rb, err := d.Rollback(ctx, nil, v1.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readCurrent(t, d, "policy.json"), `"capacity": 500`) || rb.Dir != v1.Dir {
		t.Error("rollback did not restore v1")
	}
	if dirty, _ := d.Dirty(ctx); !dirty {
		t.Error("after rollback the DB differs from live config; Dirty should be true")
	}
	if active, _ := d.Store.ActiveVersion(ctx); active.ID != rb.ID {
		t.Error("rollback version is not active")
	}
	if r.reloads < 4 {
		t.Errorf("reloads = %d", r.reloads)
	}
}
