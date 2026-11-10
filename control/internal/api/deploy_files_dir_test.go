package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"corvus/control/internal/deploy"
	"corvus/control/internal/store"
)

// The file viewer only reads generated version directories, whatever the database says.
func TestDeploymentFilesRefusesForeignDir(t *testing.T) {
	e := newTestEnv(t)
	dir := t.TempDir()
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Dir: filepath.Join(dir, "nginx")}
	e.h = e.s.Handler()
	_ = os.WriteFile(filepath.Join(dir, "secret.txt"), []byte("outside"), 0o600)
	ctx := context.Background()
	v := &store.ConfigVersion{Message: "restored", Status: "superseded", Dir: "../.."}
	if err := e.s.Store.CreateVersion(ctx, v); err != nil {
		t.Fatal(err)
	}
	w, m := e.do("GET", fmt.Sprintf("/api/deployments/%d/files", v.ID), nil, withCookie(e.cookie))
	if w.Code != 409 || m["files"] != nil {
		t.Fatalf("got %d %v", w.Code, m)
	}
}
