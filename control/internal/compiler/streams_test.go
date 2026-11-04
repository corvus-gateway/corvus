package compiler

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

func streamEnv(t *testing.T) (*Compiler, *store.Store, Paths, string) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	p := Paths{DataDir: filepath.ToSlash(dir), LuaDir: "/opt/corvus/lua", HTMLDir: "/opt/corvus/html", MimeTypes: "mime.types",
		Resolver: "127.0.0.11", HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181, AdminPort: 9443, RedisPort: 6379}
	return &Compiler{P: p, HostGateway: func() string { return "172.17.0.1" }}, st, p, dir
}

func TestRenderNoStreams(t *testing.T) {
	c, st, _, dir := streamEnv(t)
	out := filepath.Join(dir, "v1")
	if err := c.Render(context.Background(), st, out); err != nil {
		t.Fatal(err)
	}
	b, _ := os.ReadFile(filepath.Join(out, "nginx.conf"))
	if strings.Contains(string(b), "stream {") {
		t.Error("stream block rendered without streams (existing installs must render the same configuration)")
	}
	if _, err := os.Stat(filepath.Join(out, "streams")); err == nil {
		t.Error("streams directory created without streams")
	}
}

func TestRenderStreams(t *testing.T) {
	ctx := context.Background()
	c, st, p, dir := streamEnv(t)
	issued := &store.Certificate{Name: "db", Kind: "custom", Domains: []string{"db.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	pending := &store.Certificate{Name: "p", Kind: "acme", Domains: []string{"p.example.com"}, NotAfter: time.Now(), Status: "pending"}
	for _, cert := range []*store.Certificate{issued, pending} {
		if err := st.CreateCertificate(ctx, cert); err != nil {
			t.Fatal(err)
		}
	}
	certDir := filepath.FromSlash(p.CertDir(issued.ID))
	os.MkdirAll(certDir, 0o755)
	os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("CERT"), 0o644)
	os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("KEY"), 0o600)

	streams := []*store.Stream{
		{IncomingPort: 8090, TCP: true, ForwardHost: "demo", ForwardPort: 80, Enabled: true},
		{IncomingPort: 5353, TCP: true, UDP: true, ForwardHost: "10.0.0.53", ForwardPort: 53, Enabled: true},
		{IncomingPort: 5433, TCP: true, ForwardHost: "localhost", ForwardPort: 5432, CertificateID: &issued.ID, Enabled: true},
		{IncomingPort: 5434, TCP: true, ForwardHost: "db", ForwardPort: 5432, CertificateID: &pending.ID, Enabled: true},
		{IncomingPort: 8090, TCP: true, ForwardHost: "other", ForwardPort: 80, Enabled: false},
	}
	for _, s := range streams {
		if err := st.SaveStream(ctx, s); err != nil {
			t.Fatal(err)
		}
	}
	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string {
		b, err := os.ReadFile(filepath.Join(out, name))
		if err != nil {
			t.Fatal(err)
		}
		return string(b)
	}
	mustHave := func(name, conf string, want ...string) {
		for _, w := range want {
			if !strings.Contains(conf, w) {
				t.Errorf("%s: missing %q\n%s", name, w, conf)
			}
		}
	}
	mustNot := func(name, conf string, bad ...string) {
		for _, b := range bad {
			if strings.Contains(conf, b) {
				t.Errorf("%s: unexpected %q\n%s", name, b, conf)
			}
		}
	}
	main := read("nginx.conf")
	mustHave("nginx.conf", main, "stream {", "include "+filepath.ToSlash(out)+"/streams/*.conf;", "4 stream(s)", "resolver 127.0.0.11")

	s1 := read("streams/1.conf")
	mustHave("1.conf", s1, "zone corvus_stream_1 64k;", "server demo:80 max_fails=0 resolve;", "listen 8090;", "proxy_pass corvus_stream_1;")
	mustNot("1.conf", s1, "udp", "ssl")

	s2 := read("streams/2.conf")
	mustHave("2.conf", s2, "server 10.0.0.53:53 max_fails=0;", "listen 5353;", "listen 5353 udp;")
	mustNot("2.conf", s2, "resolve;", "zone")

	// Loopback means the Docker host; TLS termination with the issued certificate.
	s3 := read("streams/3.conf")
	mustHave("3.conf", s3, "server 172.17.0.1:5432 max_fails=0;", "listen 5433 ssl;", "ssl_certificate     "+p.CertDir(issued.ID)+"/fullchain.pem;")

	// Certificate not issued yet: never served as plain TCP.
	s4 := read("streams/4.conf")
	mustNot("4.conf", s4, "listen", "server {")

	if _, err := os.Stat(filepath.Join(out, "streams", "5.conf")); err == nil {
		t.Error("disabled stream was rendered")
	}
}

func TestStreamConflict(t *testing.T) {
	p := Paths{HTTPPort: 80, HTTPSPort: 443, StatusPort: 9180, ACMEPort: 9181, AdminPort: 9443, RedisPort: 6379}
	others := []store.Stream{
		{ID: 1, IncomingPort: 5000, TCP: true, Enabled: true},
		{ID: 2, IncomingPort: 6000, UDP: true, Enabled: true},
		{ID: 3, IncomingPort: 7000, TCP: true, Enabled: false},
	}
	cases := []struct {
		st       store.Stream
		conflict bool
	}{
		{store.Stream{IncomingPort: 80, TCP: true, Enabled: true}, true},
		{store.Stream{IncomingPort: 443, UDP: true, Enabled: true}, true},
		{store.Stream{IncomingPort: 9443, TCP: true}, true}, // reserved even when disabled
		{store.Stream{IncomingPort: 6379, TCP: true, Enabled: true}, true},
		{store.Stream{IncomingPort: 9180, TCP: true, Enabled: true}, true},
		{store.Stream{IncomingPort: 5000, TCP: true, Enabled: true}, true},
		{store.Stream{IncomingPort: 5000, UDP: true, Enabled: true}, false}, // other protocol
		{store.Stream{ID: 1, IncomingPort: 5000, TCP: true, Enabled: true}, false},
		{store.Stream{IncomingPort: 6000, TCP: true, UDP: true, Enabled: true}, true},
		{store.Stream{IncomingPort: 5000, TCP: true, Enabled: false}, false},
		{store.Stream{IncomingPort: 7000, TCP: true, Enabled: true}, false}, // the other one is disabled
	}
	for _, c := range cases {
		st := c.st
		if got := StreamConflict(p, others, &st) != ""; got != c.conflict {
			t.Errorf("%+v: conflict=%v, want %v", c.st, got, c.conflict)
		}
	}
}
