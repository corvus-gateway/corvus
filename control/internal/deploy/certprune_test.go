package deploy

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// Files of deleted certificates (with their private keys) are removed once no retained
// version directory refers to them; certificates in the database, recent directories
// and other directories (the default certificate) stay.
func TestPruneDeletedCertificateFiles(t *testing.T) {
	d, _, dir := setup(t)
	d.Keep = 2
	ctx := context.Background()
	certsDir := filepath.Join(dir, "certs")
	old := time.Now().Add(-time.Hour)
	mkCert := func(name string, aged bool) string {
		t.Helper()
		p := filepath.Join(certsDir, name)
		if err := os.MkdirAll(p, 0o700); err != nil {
			t.Fatal(err)
		}
		for _, f := range []string{"fullchain.pem", "privkey.pem"} {
			if err := os.WriteFile(filepath.Join(p, f), []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if aged {
			if err := os.Chtimes(p, old, old); err != nil {
				t.Fatal(err)
			}
		}
		return p
	}
	exists := func(p string) bool { _, err := os.Stat(p); return err == nil }

	used := &store.Certificate{Name: "used", Kind: "custom", Domains: []string{"shop.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	spare := &store.Certificate{Name: "spare", Kind: "custom", Domains: []string{"x.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	for _, c := range []*store.Certificate{used, spare} {
		if err := d.Store.CreateCertificate(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	usedDir := mkCert(fmt.Sprint(used.ID), true)
	spareDir := mkCert(fmt.Sprint(spare.ID), true)
	orphan := mkCert("99", true)     // deleted long ago, referenced by no version
	fresh := mkCert("98", false)     // written just now (a transaction not committed yet)
	deflt := mkCert("default", true) // not a certificate id

	host := &store.ProxyHost{Domains: []string{"shop.example.com"}, ForwardScheme: "http", ForwardHost: "app", ForwardPort: 8080, CertificateID: &used.ID, Enabled: true}
	if _, err := d.Apply(ctx, nil, "host with certificate", func(tx *store.Store) error { return tx.SaveProxyHost(ctx, host) }); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(readCurrent(t, d, fmt.Sprintf("hosts/%d.conf", host.ID)), filepath.ToSlash(usedDir)+"/") {
		t.Fatal("the host does not use the certificate files")
	}
	if exists(orphan) || !exists(fresh) || !exists(deflt) || !exists(usedDir) || !exists(spareDir) {
		t.Fatalf("after the first deployment: orphan %v fresh %v default %v used %v spare %v",
			exists(orphan), exists(fresh), exists(deflt), exists(usedDir), exists(spareDir))
	}

	// the certificate is deleted; the previous version (a rollback target) still uses it
	if _, err := d.Apply(ctx, nil, "drop certificate", func(tx *store.Store) error {
		host.CertificateID = nil
		if err := tx.SaveProxyHost(ctx, host); err != nil {
			return err
		}
		return tx.DeleteCertificate(ctx, used.ID)
	}); err != nil {
		t.Fatal(err)
	}
	if !exists(usedDir) {
		t.Fatal("files removed while a retained version refers to them")
	}
	// once no retained version refers to it, the files go
	for i := 0; i < 3; i++ {
		if _, err := d.Apply(ctx, nil, "more", nil); err != nil {
			t.Fatal(err)
		}
	}
	if exists(usedDir) || !exists(spareDir) || !exists(fresh) || !exists(deflt) {
		t.Fatalf("after pruning: used %v spare %v fresh %v default %v", exists(usedDir), exists(spareDir), exists(fresh), exists(deflt))
	}
}
