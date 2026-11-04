package api

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"math/big"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/certs"
	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
	"corvus/control/internal/store"
)

// selfSigned returns a PEM certificate and key for names.
func selfSigned(t *testing.T, serial int64, notAfter time.Time, names ...string) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	tpl := &x509.Certificate{SerialNumber: big.NewInt(serial), Subject: pkix.Name{CommonName: names[0]}, DNSNames: names,
		NotBefore: time.Now().Add(-time.Hour), NotAfter: notAfter}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(key)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}))
}

type countRunner struct{ reloads int }

func (f *countRunner) Test(context.Context, string) (string, error) { return "ok", nil }
func (f *countRunner) Reload() error                                { f.reloads++; return nil }

// Rejections happen before anything is deployed: unknown ID, ACME certificate, a key
// that does not match.
func TestReplaceCertificateRejects(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	acme := &store.Certificate{Name: "le", Kind: "acme", Domains: []string{"a.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	custom := &store.Certificate{Name: "up", Kind: "custom", Domains: []string{"b.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	for _, c := range []*store.Certificate{acme, custom} {
		if err := e.s.Store.CreateCertificate(ctx, c); err != nil {
			t.Fatal(err)
		}
	}
	cert, key := selfSigned(t, 1, time.Now().Add(24*time.Hour), "b.example.com")
	_, otherKey := selfSigned(t, 2, time.Now().Add(24*time.Hour), "b.example.com")
	put := func(id string, body map[string]any) (int, string) {
		w, _ := e.do("PUT", "/api/certificates/"+id, body, withCookie(e.cookie), withCSRF())
		return w.Code, w.Body.String()
	}
	if code, _ := put("99", map[string]any{"certificate": cert, "private_key": key}); code != 404 {
		t.Errorf("unknown certificate: %d", code)
	}
	if code, body := put("1", map[string]any{"certificate": cert, "private_key": key}); code != 422 || !strings.Contains(body, "not_custom") {
		t.Errorf("ACME certificate: %d %s", code, body)
	}
	if code, body := put("2", map[string]any{"certificate": cert, "private_key": otherKey}); code != 422 || !strings.Contains(body, "certificate") {
		t.Errorf("mismatched key: %d %s", code, body)
	}
	if code, _ := put("2", map[string]any{"certificate": "garbage", "private_key": key}); code != 422 {
		t.Errorf("invalid PEM: %d", code)
	}
}

// A replacement keeps the ID, writes the new files, updates domains and expiry and
// redeploys.
func TestReplaceCertificate(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	dir := t.TempDir()
	p := compiler.Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181}
	runner := &countRunner{}
	e.s.Paths = p
	e.s.Deployer = &deploy.Deployer{Store: e.s.Store, Compiler: &compiler.Compiler{P: p}, Runner: runner, Dir: filepath.Join(dir, "nginx")}

	oldCert, oldKey := selfSigned(t, 1, time.Now().Add(24*time.Hour), "shop.example.com")
	c := &store.Certificate{Name: "shop", Kind: "custom", Domains: []string{"shop.example.com"}, NotAfter: time.Now().Add(24 * time.Hour)}
	if err := e.s.Store.CreateCertificate(ctx, c); err != nil {
		t.Fatal(err)
	}
	if err := certs.Write(filepath.FromSlash(p.CertDir(c.ID)), []byte(oldCert), []byte(oldKey)); err != nil {
		t.Fatal(err)
	}
	if err := e.s.Store.SaveProxyHost(ctx, &store.ProxyHost{Domains: []string{"shop.example.com"}, ForwardHost: "app", ForwardPort: 80, CertificateID: &c.ID, Enabled: true}); err != nil {
		t.Fatal(err)
	}
	reloads := runner.reloads

	notAfter := time.Now().Add(90 * 24 * time.Hour).Truncate(time.Second)
	newCert, newKey := selfSigned(t, 2, notAfter, "shop.example.com", "www.shop.example.com")
	w, _ := e.do("PUT", "/api/certificates/1", map[string]any{"certificate": newCert, "private_key": newKey}, withCookie(e.cookie), withCSRF())
	if w.Code != 200 {
		t.Fatalf("replace: %d %s", w.Code, w.Body)
	}
	got, err := e.s.Store.CertificateByID(ctx, c.ID)
	if err != nil || got.Name != "shop" || len(got.Domains) != 2 || !got.NotAfter.Equal(notAfter.UTC()) {
		t.Fatalf("stored: %+v %v", got, err)
	}
	b, _ := os.ReadFile(filepath.Join(filepath.FromSlash(p.CertDir(c.ID)), "fullchain.pem"))
	if string(b) != newCert {
		t.Error("certificate file not replaced")
	}
	if runner.reloads <= reloads {
		t.Error("hosts were not redeployed")
	}
	h, _ := e.s.Store.ProxyHostByID(ctx, 1)
	if h.CertificateID == nil || *h.CertificateID != c.ID {
		t.Error("host lost its certificate")
	}

	// A configuration the gateway rejects leaves the previous pair in place.
	e.s.Deployer.Runner = rejectRunner{}
	third, thirdKey := selfSigned(t, 3, notAfter, "shop.example.com")
	if w, _ := e.do("PUT", "/api/certificates/1", map[string]any{"certificate": third, "private_key": thirdKey}, withCookie(e.cookie), withCSRF()); w.Code == 200 {
		t.Fatal("rejected deployment reported success")
	}
	b, _ = os.ReadFile(filepath.Join(filepath.FromSlash(p.CertDir(c.ID)), "fullchain.pem"))
	if string(b) != newCert {
		t.Error("previous certificate not restored after a failed deployment")
	}
	if got, _ := e.s.Store.CertificateByID(ctx, c.ID); len(got.Domains) != 2 {
		t.Errorf("store changed by a failed deployment: %+v", got)
	}
}

type rejectRunner struct{}

func (rejectRunner) Test(context.Context, string) (string, error) {
	return "nginx: [emerg] test", errors.New("rejected")
}
func (rejectRunner) Reload() error { return nil }
