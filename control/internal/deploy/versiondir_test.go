package deploy

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestVersionPath(t *testing.T) {
	d := &Deployer{Dir: filepath.Join("data", "nginx")}
	if p, err := d.VersionPath("v000042"); err != nil || p != filepath.Join("data", "nginx", "versions", "v000042") {
		t.Fatalf("valid: %q %v", p, err)
	}
	if _, err := d.VersionPath("v1234567"); err != nil {
		t.Fatalf("ids beyond six digits: %v", err)
	}
	for _, bad := range []string{"", "v", "v12", "../..", "v000001/..", "../v000001", "v000001/../../etc", `v000001\..`, "/etc", "x000001", "v00000a", "v000001 "} {
		if _, err := d.VersionPath(bad); !errors.Is(err, ErrBadVersionDir) {
			t.Errorf("%q accepted: %v", bad, err)
		}
	}
	if err := d.swapLink("../../etc"); !errors.Is(err, ErrBadVersionDir) {
		t.Fatalf("swapLink: %v", err)
	}
}

// A version directory taken from the database (e.g. a restored backup) must name a
// generated version directory before it is read or activated.
func TestRollbackRefusesForeignVersionDir(t *testing.T) {
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	ctx := context.Background()
	v := &store.ConfigVersion{Message: "restored", Dir: "../.."}
	if err := st.CreateVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	_ = st.FinishVersion(ctx, v.ID, true, "ok")
	r := &fakeRunner{}
	d := &Deployer{Store: st, Runner: r, Dir: filepath.Join(dir, "nginx")}
	if _, err := d.Rollback(ctx, nil, v.ID); !errors.Is(err, ErrBadVersionDir) {
		t.Fatalf("rollback: %v", err)
	}
	if r.reloads != 0 {
		t.Fatal("reloaded")
	}
}

func TestEnsureCurrentIgnoresForeignVersionDir(t *testing.T) {
	d, _, _ := setup(t)
	ctx := context.Background()
	v1, err := d.Apply(ctx, nil, "first", nil)
	if err != nil {
		t.Fatal(err)
	}
	// the active version now names a directory outside versions/
	outside := filepath.Join(d.Dir, "outside")
	_ = os.MkdirAll(outside, 0o755)
	_ = os.WriteFile(filepath.Join(outside, "nginx.conf"), []byte("# not generated\n"), 0o644)
	if err := d.Store.SetVersionDir(ctx, v1.ID, "../outside"); err != nil {
		t.Fatal(err)
	}
	if err := d.EnsureCurrent(ctx); err != nil {
		t.Fatal(err)
	}
	link, _ := os.Readlink(d.currentLink())
	if strings.Contains(link, "outside") || !strings.HasPrefix(filepath.ToSlash(link), "versions/v") {
		t.Fatalf("current = %s", link)
	}
}
