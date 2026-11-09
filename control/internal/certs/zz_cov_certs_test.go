package certs

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func makePair(t *testing.T, tpl *x509.Certificate) (certPEM, keyPEM []byte) {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if tpl.SerialNumber == nil {
		tpl.SerialNumber = big.NewInt(1)
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalPKCS8PrivateKey(key)
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb})
}

func TestCovEnsureSelfSigned(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "default")
	if err := EnsureSelfSigned(dir, "corvus.local", []string{"corvus.local", "192.0.2.10", "::1"}); err != nil {
		t.Fatal(err)
	}
	cp, _ := os.ReadFile(filepath.Join(dir, "fullchain.pem"))
	kp, _ := os.ReadFile(filepath.Join(dir, "privkey.pem"))
	info, err := Parse(cp, kp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(info.Domains, ",") != "corvus.local,192.0.2.10,::1" {
		t.Fatalf("domains = %v", info.Domains)
	}
	if time.Until(info.NotAfter) < 9*365*24*time.Hour {
		t.Fatalf("self-signed validity too short: %v", info.NotAfter)
	}
	blk, _ := pem.Decode(cp)
	c, _ := x509.ParseCertificate(blk.Bytes)
	if c.Subject.CommonName != "corvus.local" || c.Subject.Organization[0] != "Corvus" || len(c.IPAddresses) != 2 {
		t.Fatalf("subject/IPs: %v %v", c.Subject, c.IPAddresses)
	}
	// an existing pair is kept
	if err := EnsureSelfSigned(dir, "other", []string{"other"}); err != nil {
		t.Fatal(err)
	}
	if cp2, _ := os.ReadFile(filepath.Join(dir, "fullchain.pem")); !bytes.Equal(cp, cp2) {
		t.Fatal("existing certificate must not be replaced")
	}
	// a directory that cannot be created
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	if err := EnsureSelfSigned(filepath.Join(f, "sub"), "x", nil); err == nil {
		t.Fatal("unwritable directory must fail")
	}
}

func TestCovParse(t *testing.T) {
	now := time.Now()
	cp, kp := makePair(t, &x509.Certificate{Subject: pkix.Name{CommonName: "cn.example"}, DNSNames: []string{"WWW.Example.COM"},
		IPAddresses: []net.IP{net.ParseIP("2001:DB8::1")}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)})
	info, err := Parse(cp, kp)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(info.Domains, ",") != "www.example.com,2001:db8::1" || info.NotAfter.Location() != time.UTC {
		t.Fatalf("info = %+v", info)
	}

	// no SANs: the common name is used
	cp2, kp2 := makePair(t, &x509.Certificate{Subject: pkix.Name{CommonName: "Legacy.Example"}, NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)})
	if info, err := Parse(cp2, kp2); err != nil || len(info.Domains) != 1 || info.Domains[0] != "legacy.example" {
		t.Fatalf("CN fallback: %+v %v", info, err)
	}
	// neither SANs nor CN: no domains
	cp3, kp3 := makePair(t, &x509.Certificate{NotBefore: now.Add(-time.Hour), NotAfter: now.Add(time.Hour)})
	if info, err := Parse(cp3, kp3); err != nil || len(info.Domains) != 0 {
		t.Fatalf("no names: %+v %v", info, err)
	}

	if _, err := Parse(cp, kp2); err == nil || !strings.Contains(err.Error(), "do not form a valid pair") {
		t.Fatalf("mismatched pair = %v", err)
	}
	cpOld, kpOld := makePair(t, &x509.Certificate{DNSNames: []string{"old.example"}, NotBefore: now.Add(-48 * time.Hour), NotAfter: now.Add(-24 * time.Hour)})
	if _, err := Parse(cpOld, kpOld); err == nil || !strings.Contains(err.Error(), "expired on") {
		t.Fatalf("expired = %v", err)
	}
	if _, err := Parse(bytes.Repeat([]byte("a"), 256<<10+1), kp); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("large cert = %v", err)
	}
	if _, err := Parse(cp, bytes.Repeat([]byte("a"), 64<<10+1)); err == nil || !strings.Contains(err.Error(), "too large") {
		t.Fatalf("large key = %v", err)
	}
	if _, err := Parse([]byte("junk"), []byte("junk")); err == nil {
		t.Fatal("junk accepted")
	}
}

func TestCovWriteErrors(t *testing.T) {
	f := filepath.Join(t.TempDir(), "file")
	_ = os.WriteFile(f, nil, 0o600)
	if err := Write(filepath.Join(f, "sub"), []byte("c"), []byte("k")); err == nil {
		t.Fatal("directory below a file must fail")
	}
	// a directory where the temporary file should go makes the write fail
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "fullchain.pem.tmp"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir, []byte("c"), []byte("k")); err == nil {
		t.Fatal("blocked temp file must fail")
	}
	// a directory in place of the final key file makes the rename fail
	dir2 := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir2, "privkey.pem", "x"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := Write(dir2, []byte("c"), []byte("k")); err == nil {
		t.Fatal("blocked rename must fail")
	}
	if st, err := os.Stat(filepath.Join(dir2, "fullchain.pem")); err == nil {
		t.Fatalf("certificate must not be installed without its key: %v", st)
	}
}
