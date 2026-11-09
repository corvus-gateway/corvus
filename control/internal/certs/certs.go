// Package certs manages certificate files on disk (default self-signed and uploaded).
package certs

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// Info is what Corvus records about a certificate.
type Info struct {
	Domains  []string
	NotAfter time.Time
}

// EnsureSelfSigned creates dir/fullchain.pem + dir/privkey.pem if they do not exist.
func EnsureSelfSigned(dir, commonName string, hosts []string) error {
	if _, err := os.Stat(filepath.Join(dir, "fullchain.pem")); err == nil {
		return nil
	}
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return err
	}
	serial, _ := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 120))
	tpl := &x509.Certificate{
		SerialNumber:          serial,
		Subject:               pkix.Name{CommonName: commonName, Organization: []string{"Corvus"}},
		NotBefore:             time.Now().Add(-time.Hour),
		NotAfter:              time.Now().AddDate(10, 0, 0),
		KeyUsage:              x509.KeyUsageDigitalSignature,
		ExtKeyUsage:           []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		BasicConstraintsValid: true,
	}
	for _, h := range hosts {
		if ip := net.ParseIP(h); ip != nil {
			tpl.IPAddresses = append(tpl.IPAddresses, ip)
		} else {
			tpl.DNSNames = append(tpl.DNSNames, h)
		}
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		return err
	}
	kb, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return err
	}
	return Write(dir,
		pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: kb}))
}

// Parse validates an uploaded PEM certificate chain and private key.
func Parse(certPEM, keyPEM []byte) (*Info, error) {
	if len(certPEM) > 256<<10 || len(keyPEM) > 64<<10 {
		return nil, errors.New("certificate or key is too large")
	}
	pair, err := tls.X509KeyPair(certPEM, keyPEM)
	if err != nil {
		return nil, fmt.Errorf("certificate and key do not form a valid pair: %w", err)
	}
	leaf, err := x509.ParseCertificate(pair.Certificate[0])
	if err != nil {
		return nil, err
	}
	if time.Now().After(leaf.NotAfter) {
		return nil, fmt.Errorf("certificate expired on %s", leaf.NotAfter.Format(time.DateOnly))
	}
	domains := append([]string{}, leaf.DNSNames...)
	for _, ip := range leaf.IPAddresses {
		domains = append(domains, ip.String())
	}
	if len(domains) == 0 && leaf.Subject.CommonName != "" {
		domains = []string{leaf.Subject.CommonName}
	}
	for i := range domains {
		domains[i] = strings.ToLower(domains[i])
	}
	return &Info{Domains: domains, NotAfter: leaf.NotAfter.UTC()}, nil
}

// Write stores the pair; the private key is only readable by the owner. Both files are
// written under temporary names first and then renamed, so a reload or config test never
// sees a new certificate next to the old key (or a half-written file).
func Write(dir string, certPEM, keyPEM []byte) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	files := []struct {
		name string
		data []byte
		mode os.FileMode
	}{{"fullchain.pem", certPEM, 0o644}, {"privkey.pem", keyPEM, 0o600}}
	for _, f := range files {
		if err := os.WriteFile(filepath.Join(dir, f.name+".tmp"), f.data, f.mode); err != nil {
			return err
		}
	}
	// both renames happen right after each other; a crash while writing leaves the old pair
	for _, name := range []string{"privkey.pem", "fullchain.pem"} {
		if err := os.Rename(filepath.Join(dir, name+".tmp"), filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}
