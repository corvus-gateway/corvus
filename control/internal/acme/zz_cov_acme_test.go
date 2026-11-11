package acme

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// fakeCA is a minimal RFC 8555 server: enough of the protocol for lego to register an
// account, order a certificate, answer an http-01 challenge and download the chain.
// The challenge is validated by asking the Service's own HTTP-01 handler for the key
// authorization and comparing it with the account key's thumbprint.
type fakeCA struct {
	t      *testing.T
	srv    *httptest.Server
	svc    *Service
	caKey  *ecdsa.PrivateKey
	caCert *x509.Certificate
	caPEM  []byte

	mu         sync.Mutex
	nonce      int
	thumbs     map[string]string // account URL → JWK thumbprint
	accounts   int
	orders     int
	orderIDs   map[int][]string
	certs      map[int][]byte
	rejectAll  bool          // newOrder answers with an ACME problem
	gate       chan struct{} // when set, newOrder waits for it to be closed
	validity   time.Duration
	challenges int
	atGate     int // orders waiting at the gate
}

func newFakeCA(t *testing.T, svc *Service) *fakeCA {
	t.Helper()
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "Fake CA"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(24 * time.Hour),
		IsCA: true, BasicConstraintsValid: true, KeyUsage: x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	ca, _ := x509.ParseCertificate(der)
	f := &fakeCA{t: t, svc: svc, caKey: key, caCert: ca, caPEM: pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}),
		thumbs: map[string]string{}, orderIDs: map[int][]string{}, certs: map[int][]byte{}, validity: 90 * 24 * time.Hour}
	// lego insists on HTTPS; trust the test server's certificate via LEGO_CA_CERTIFICATES.
	f.srv = httptest.NewTLSServer(http.HandlerFunc(f.serve))
	t.Cleanup(f.srv.Close)
	caFile := filepath.Join(t.TempDir(), "ca.pem")
	if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: f.srv.Certificate().Raw}), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("LEGO_CA_CERTIFICATES", caFile)
	return f
}

func (f *fakeCA) dir() string { return f.srv.URL + "/dir" }

type jws struct {
	Protected string `json:"protected"`
	Payload   string `json:"payload"`
}

type protectedHeader struct {
	KID string          `json:"kid"`
	JWK json.RawMessage `json:"jwk"`
}

func b64(s string) []byte {
	b, _ := base64.RawURLEncoding.DecodeString(s)
	return b
}

// thumbprint is the RFC 7638 thumbprint of an EC P-256 JWK.
func thumbprint(raw json.RawMessage) string {
	var k struct{ Crv, Kty, X, Y string }
	_ = json.Unmarshal(raw, &k)
	canon := fmt.Sprintf(`{"crv":"%s","kty":"%s","x":"%s","y":"%s"}`, k.Crv, k.Kty, k.X, k.Y)
	sum := sha256.Sum256([]byte(canon))
	return base64.RawURLEncoding.EncodeToString(sum[:])
}

func (f *fakeCA) problem(w http.ResponseWriter, status int, typ, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"type": "urn:ietf:params:acme:error:" + typ, "detail": detail, "status": status})
}

func (f *fakeCA) order(id int, status string) map[string]any {
	var idents []map[string]string
	var authz []string
	for i, d := range f.orderIDs[id] {
		idents = append(idents, map[string]string{"type": "dns", "value": d})
		authz = append(authz, fmt.Sprintf("%s/authz/%d/%d", f.srv.URL, id, i))
	}
	o := map[string]any{"status": status, "identifiers": idents, "authorizations": authz, "finalize": fmt.Sprintf("%s/finalize/%d", f.srv.URL, id)}
	if status == "valid" {
		o["certificate"] = fmt.Sprintf("%s/cert/%d", f.srv.URL, id)
	}
	return o
}

func (f *fakeCA) serve(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.nonce++
	w.Header().Set("Replay-Nonce", "nonce-"+strconv.Itoa(f.nonce))
	f.mu.Unlock()
	w.Header().Set("Cache-Control", "no-store")
	base := f.srv.URL
	if r.URL.Path == "/dir" {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"newNonce": base + "/nonce", "newAccount": base + "/acct", "newOrder": base + "/order",
			"revokeCert": base + "/revoke", "keyChange": base + "/keychange",
		})
		return
	}
	if r.URL.Path == "/nonce" {
		w.WriteHeader(http.StatusOK)
		return
	}
	var msg jws
	if err := json.NewDecoder(r.Body).Decode(&msg); err != nil {
		f.problem(w, 400, "malformed", "not a JWS")
		return
	}
	var hdr protectedHeader
	_ = json.Unmarshal(b64(msg.Protected), &hdr)
	payload := b64(msg.Payload)
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	w.Header().Set("Content-Type", "application/json")
	f.mu.Lock()
	defer f.mu.Unlock()
	switch parts[0] {
	case "acct":
		f.accounts++
		loc := fmt.Sprintf("%s/acct/%d", base, f.accounts)
		f.thumbs[loc] = thumbprint(hdr.JWK)
		w.Header().Set("Location", loc)
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(map[string]any{"status": "valid"})
	case "order":
		if len(parts) == 2 { // POST-as-GET of an existing order
			id, _ := strconv.Atoi(parts[1])
			status := "pending"
			if f.certs[id] != nil {
				status = "valid"
			}
			_ = json.NewEncoder(w).Encode(f.order(id, status))
			return
		}
		if _, ok := f.thumbs[hdr.KID]; !ok {
			f.problem(w, 400, "accountDoesNotExist", "unknown account "+hdr.KID)
			return
		}
		if f.rejectAll {
			f.problem(w, 403, "rejectedIdentifier", "policy forbids issuing for this name")
			return
		}
		if g := f.gate; g != nil {
			f.atGate++
			f.mu.Unlock()
			<-g
			f.mu.Lock()
		}
		var req struct {
			Identifiers []struct{ Value string } `json:"identifiers"`
		}
		_ = json.Unmarshal(payload, &req)
		f.orders++
		id := f.orders
		for _, i := range req.Identifiers {
			f.orderIDs[id] = append(f.orderIDs[id], i.Value)
		}
		w.Header().Set("Location", fmt.Sprintf("%s/order/%d", base, id))
		w.WriteHeader(http.StatusCreated)
		_ = json.NewEncoder(w).Encode(f.order(id, "pending"))
	case "authz":
		id, _ := strconv.Atoi(parts[1])
		i, _ := strconv.Atoi(parts[2])
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "pending", "identifier": map[string]string{"type": "dns", "value": f.orderIDs[id][i]},
			"challenges": []map[string]string{{"type": "http-01", "status": "pending", "url": fmt.Sprintf("%s/chall/%d/%d", base, id, i), "token": fmt.Sprintf("tok%d_%d", id, i)}},
		})
	case "chall":
		token := fmt.Sprintf("tok%s_%s", parts[1], parts[2])
		rec := httptest.NewRecorder()
		f.svc.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/"+token, nil))
		want := token + "." + f.thumbs[hdr.KID]
		status := "valid"
		if rec.Code != 200 || rec.Body.String() != want {
			f.t.Errorf("http-01 key authorization = %d %q, want %q", rec.Code, rec.Body.String(), want)
			status = "invalid"
		}
		f.challenges++
		_ = json.NewEncoder(w).Encode(map[string]any{"type": "http-01", "status": status, "url": base + r.URL.Path, "token": token})
	case "finalize":
		id, _ := strconv.Atoi(parts[1])
		var req struct{ CSR string }
		_ = json.Unmarshal(payload, &req)
		csr, err := x509.ParseCertificateRequest(b64(req.CSR))
		if err != nil {
			f.problem(w, 400, "badCSR", err.Error())
			return
		}
		serial, _ := rand.Int(rand.Reader, big.NewInt(1<<62))
		tpl := &x509.Certificate{SerialNumber: serial, Subject: pkix.Name{CommonName: csr.Subject.CommonName}, DNSNames: csr.DNSNames,
			NotBefore: time.Now().Add(-time.Minute), NotAfter: time.Now().Add(f.validity), KeyUsage: x509.KeyUsageDigitalSignature,
			ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth}}
		der, err := x509.CreateCertificate(rand.Reader, tpl, f.caCert, csr.PublicKey, f.caKey)
		if err != nil {
			f.problem(w, 500, "serverInternal", err.Error())
			return
		}
		f.certs[id] = append(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), f.caPEM...)
		_ = json.NewEncoder(w).Encode(f.order(id, "valid"))
	case "cert":
		id, _ := strconv.Atoi(parts[1])
		w.Header().Set("Content-Type", "application/pem-certificate-chain")
		_, _ = w.Write(f.certs[id])
	default:
		f.problem(w, 404, "malformed", "unknown path "+r.URL.Path)
	}
}

// acmeEnv is a Service wired to a fake CA and a temporary certificate directory.
func acmeEnv(t *testing.T) (*Service, *fakeCA, *atomic.Int32) {
	t.Helper()
	s := testService(t)
	f := newFakeCA(t, s)
	s.Directory = f.dir()
	dir := t.TempDir()
	s.CertDir = func(id int64) string { return filepath.Join(dir, strconv.FormatInt(id, 10)) }
	issued := new(atomic.Int32)
	s.OnIssued = func(context.Context) { issued.Add(1) }
	s.DeployDelay = 10 * time.Millisecond
	return s, f, issued
}

func newACMECert(t *testing.T, s *Service, status string, notAfter time.Time, info store.ACMEInfo, domains ...string) int64 {
	t.Helper()
	c := &store.Certificate{Name: "le", Kind: "acme", Domains: domains, Status: status, NotAfter: notAfter, AutoRenew: true, ACME: info}
	if err := s.Store.CreateCertificate(context.Background(), c); err != nil {
		t.Fatal(err)
	}
	return c.ID
}

func TestCovIssueHTTP01EndToEnd(t *testing.T) {
	s, f, issued := acmeEnv(t)
	ctx := context.Background()
	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com", Challenge: "http-01"}, "www.example.com", "example.com")
	if err := s.issue(ctx, id); err != nil {
		t.Fatal(err)
	}
	c, _ := s.Store.CertificateByID(ctx, id)
	if c.Status != "valid" || c.LastError != "" || time.Until(c.NotAfter) < 80*24*time.Hour {
		t.Fatalf("certificate after issuance: %+v", c)
	}
	if strings.Join(c.Domains, ",") != "www.example.com,example.com" && strings.Join(c.Domains, ",") != "example.com,www.example.com" {
		t.Fatalf("domains = %v", c.Domains)
	}
	if waitCount(issued, 1) != 1 || f.accounts != 1 || f.challenges != 2 {
		t.Fatalf("issued=%d accounts=%d challenges=%d", issued.Load(), f.accounts, f.challenges)
	}
	chain, err := os.ReadFile(filepath.Join(s.CertDir(id), "fullchain.pem"))
	if err != nil || strings.Count(string(chain), "BEGIN CERTIFICATE") != 2 {
		t.Fatalf("fullchain: %v", err)
	}
	if st, err := os.Stat(filepath.Join(s.CertDir(id), "privkey.pem")); err != nil || st.Mode().Perm() != 0o600 {
		t.Fatalf("privkey: %v %v", st, err)
	}
	// the challenge tokens are cleaned up after validation
	rec := httptest.NewRecorder()
	s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/.well-known/acme-challenge/tok1_0", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("token still served after cleanup: %d", rec.Code)
	}
	// the account was stored sealed with its registration and is reused next time
	acc, err := s.Store.ACMEAccount(ctx, "ops@example.com", false)
	if err != nil || !strings.Contains(acc.Registration, f.srv.URL+"/acct/1") || strings.Contains(acc.KeyPEM, "PRIVATE KEY") {
		t.Fatalf("stored account: %+v %v", acc, err)
	}
	if pemKey, err := seal.Open(s.Key, acc.KeyPEM); err != nil || !strings.Contains(pemKey, "EC PRIVATE KEY") {
		t.Fatalf("sealed account key: %v", err)
	}
	id2 := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, "api.example.com")
	if err := s.issue(ctx, id2); err != nil {
		t.Fatal(err)
	}
	if f.accounts != 1 {
		t.Fatalf("stored account must be reused, registrations = %d", f.accounts)
	}
	if waitCount(issued, 2) != 2 {
		t.Fatalf("OnIssued calls = %d", issued.Load())
	}
}

func TestCovAccountFromOtherCAIsReplaced(t *testing.T) {
	s, f, _ := acmeEnv(t)
	ctx := context.Background()
	// an account registered with another CA (different host)
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	der, _ := x509.MarshalECPrivateKey(key)
	sealed, _ := seal.Seal(s.Key, string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: der})))
	if err := s.Store.SaveACMEAccount(ctx, &store.ACMEAccount{Email: "ops@example.com", KeyPEM: sealed,
		Registration: `{"uri":"https://acme-v02.api.letsencrypt.org/acme/acct/42","body":{"status":"valid"}}`}); err != nil {
		t.Fatal(err)
	}
	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, "www.example.com")
	if err := s.issue(ctx, id); err != nil {
		t.Fatal(err)
	}
	if f.accounts != 1 {
		t.Fatalf("a fresh account must be registered with this CA, registrations = %d", f.accounts)
	}
	acc, _ := s.Store.ACMEAccount(ctx, "ops@example.com", false)
	if !strings.Contains(acc.Registration, f.srv.URL) || acc.KeyPEM == sealed {
		t.Fatalf("stored account was not replaced: %+v", acc)
	}
}

func TestCovCorruptStoredAccount(t *testing.T) {
	s, f, _ := acmeEnv(t)
	ctx := context.Background()
	notPEM, _ := seal.Seal(s.Key, "not a pem block")
	badDER, _ := seal.Seal(s.Key, string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: []byte("junk")})))
	for name, keyPEM := range map[string]string{"unsealable": "garbage", "not pem": notPEM, "bad der": badDER} {
		t.Run(name, func(t *testing.T) {
			email := strings.ReplaceAll(name, " ", "") + "@example.com"
			_ = s.Store.SaveACMEAccount(ctx, &store.ACMEAccount{Email: email, KeyPEM: keyPEM})
			id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: email}, "www.example.com")
			if err := s.issue(ctx, id); err == nil {
				t.Fatal("issuance with a corrupt account key must fail")
			}
			c, _ := s.Store.CertificateByID(ctx, id)
			if c.Status != "error" || c.LastError == "" {
				t.Fatalf("status = %q %q", c.Status, c.LastError)
			}
		})
	}
	if _, _, err := s.client(ctx, "notpem@example.com", false, nil); err == nil || !strings.Contains(err.Error(), "corrupt") {
		t.Fatalf("corrupt key message = %v", err)
	}
	if f.accounts != 0 {
		t.Fatal("no registration for corrupt accounts")
	}
}

func TestCovRenewalFailureKeepsValidCertificate(t *testing.T) {
	s, f, issued := acmeEnv(t)
	ctx := context.Background()
	f.rejectAll = true
	valid := newACMECert(t, s, "valid", time.Now().Add(10*24*time.Hour), store.ACMEInfo{Email: "ops@example.com"}, "www.example.com")
	if err := s.issue(ctx, valid); err == nil {
		t.Fatal("rejected order must fail")
	}
	c, _ := s.Store.CertificateByID(ctx, valid)
	if c.Status != "valid" || !strings.Contains(c.LastError, "policy forbids") {
		t.Fatalf("renewal failure of a still-valid certificate: %q %q", c.Status, c.LastError)
	}
	expired := newACMECert(t, s, "valid", time.Now().Add(-time.Hour), store.ACMEInfo{Email: "ops@example.com"}, "old.example.com")
	_ = s.issue(ctx, expired)
	if c, _ = s.Store.CertificateByID(ctx, expired); c.Status != "error" {
		t.Fatalf("expired certificate renewal failure must show error, got %q", c.Status)
	}
	if issued.Load() != 0 {
		t.Fatal("nothing was issued")
	}
}

func TestCovIssueDNS01WithoutCredentials(t *testing.T) {
	s, _, _ := acmeEnv(t)
	ctx := context.Background()
	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com", Challenge: "dns-01", DNSProvider: "cloudflare"}, "*.example.com")
	if err := s.issue(ctx, id); err == nil || !strings.Contains(err.Error(), "no DNS credentials") {
		t.Fatalf("dns-01 without credentials = %v", err)
	}
	// an unknown provider fails before any DNS change
	raw, _ := json.Marshal(Credentials{APIToken: "x"})
	sealed, _ := seal.Seal(s.Key, string(raw))
	id = newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com", Challenge: "dns-01", DNSProvider: "route53", Credentials: sealed}, "*.example.com")
	if err := s.issue(ctx, id); err == nil || !strings.Contains(err.Error(), "not supported") {
		t.Fatalf("unknown provider = %v", err)
	}
	c, _ := s.Store.CertificateByID(ctx, id)
	if c.Status != "error" {
		t.Fatalf("status = %q", c.Status)
	}
	if err := s.issue(ctx, 9999); err == nil {
		t.Fatal("unknown certificate must fail")
	}
}

func TestCovClientUnreachableCA(t *testing.T) {
	s := testService(t)
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	s.Directory = dead.URL + "/dir"
	if _, _, err := s.client(context.Background(), "ops@example.com", true, nil); err == nil {
		t.Fatal("unreachable directory must fail")
	}
	if _, err := s.Store.ACMEAccount(context.Background(), "ops@example.com", true); err == nil {
		t.Fatal("no account may be stored when registration failed")
	}
}

func TestCovStartRunsOnce(t *testing.T) {
	s, f, _ := acmeEnv(t)
	ctx := context.Background()
	id := newACMECert(t, s, "pending", time.Time{}, store.ACMEInfo{Email: "ops@example.com"}, "www.example.com")
	f.mu.Lock()
	f.gate = make(chan struct{})
	gate := f.gate
	f.mu.Unlock()
	s.Start(id)
	s.Start(id) // ignored: an order for id is already running
	deadline := time.Now().Add(10 * time.Second)
	for {
		s.mu.Lock()
		running := s.running[id]
		s.mu.Unlock()
		if !running {
			t.Fatal("Start must mark the certificate as running")
		}
		f.mu.Lock()
		accounts := f.accounts
		f.mu.Unlock()
		if accounts > 0 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("no registration")
		}
		time.Sleep(10 * time.Millisecond)
	}
	close(gate)
	for {
		c, _ := s.Store.CertificateByID(ctx, id)
		s.mu.Lock()
		running := s.running[id]
		s.mu.Unlock()
		if c.Status == "valid" && !running {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("certificate not issued: %+v", c)
		}
		time.Sleep(10 * time.Millisecond)
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.orders != 1 {
		t.Fatalf("orders = %d, want 1", f.orders)
	}
}

func TestCovHTTP01Handler(t *testing.T) {
	s := &Service{}
	get := func(path string) (int, string) {
		rec := httptest.NewRecorder()
		s.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		return rec.Code, rec.Body.String()
	}
	if code, _ := get("/.well-known/acme-challenge/abc"); code != 404 {
		t.Fatalf("unknown token = %d", code)
	}
	_ = s.Present("example.com", "abc", "abc.thumb")
	_ = s.Present("example.com", "a/b", "nested")
	_ = s.Present("example.com", "", "empty")
	if code, body := get("/.well-known/acme-challenge/abc"); code != 200 || body != "abc.thumb" {
		t.Fatalf("token = %d %q", code, body)
	}
	if code, _ := get("/.well-known/acme-challenge/a/b"); code != 404 {
		t.Fatal("tokens with a slash are never served")
	}
	if code, _ := get("/.well-known/acme-challenge/"); code != 404 {
		t.Fatal("empty token is never served")
	}
	_ = s.CleanUp("example.com", "abc", "")
	if code, _ := get("/.well-known/acme-challenge/abc"); code != 404 {
		t.Fatal("cleaned-up token still served")
	}
	acc := &account{email: "a@example.com"}
	if acc.GetEmail() != "a@example.com" || acc.GetRegistration() != nil || acc.GetPrivateKey() != nil {
		t.Fatal("account accessors")
	}
}

func TestCovDirectorySelection(t *testing.T) {
	s := &Service{}
	if s.directory(true, nil) == s.directory(false, nil) || !strings.Contains(s.directory(true, nil), "staging") {
		t.Fatal("staging and production directories differ")
	}
	s.Directory = "https://ca.internal/dir"
	if s.directory(true, []string{"a.example.com"}) != s.Directory {
		t.Fatal("an override without suffix applies to every domain")
	}
	s.DirectorySuffix = ".Corvus.Test"
	if s.directory(false, nil) == s.Directory {
		t.Fatal("no domains: the suffix override does not apply")
	}
	if !overrideApplies(".corvus.test", []string{"*.X.CORVUS.TEST"}) {
		t.Fatal("suffix match ignores case and wildcard prefix")
	}
	if sameCA("://bad", "https://x") || sameCA("https://X.example/acct", "https://x.example:443/dir") {
		t.Fatal("sameCA compares hosts including port")
	}
	if !sameCA("https://X.example/acct", "https://x.example/dir") {
		t.Fatal("sameCA ignores case")
	}
}
