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

// A stream that accepts the PROXY protocol listens with proxy_protocol on its TCP
// listeners (plain, TLS, IPv6) and believes the header from the trusted PROXY protocol
// sources (stream realip); sending it is proxy_protocol on. UDP listeners never change.
// Independent of the HTTP PROXY protocol listeners switch.
func TestRenderStreamsProxyProtocol(t *testing.T) {
	ctx := context.Background()
	c, st, p, dir := streamEnv(t)
	c.P.IPv6 = true
	cert := &store.Certificate{Name: "db", Kind: "custom", Domains: []string{"db.example.com"}, NotAfter: time.Now().Add(time.Hour)}
	if err := st.CreateCertificate(ctx, cert); err != nil {
		t.Fatal(err)
	}
	certDir := filepath.FromSlash(p.CertDir(cert.ID))
	os.MkdirAll(certDir, 0o755)
	os.WriteFile(filepath.Join(certDir, "fullchain.pem"), []byte("CERT"), 0o644)
	os.WriteFile(filepath.Join(certDir, "privkey.pem"), []byte("KEY"), 0o600)

	for _, s := range []*store.Stream{
		{IncomingPort: 2525, TCP: true, ForwardHost: "mail", ForwardPort: 25, Enabled: true, ProxyProtocolIn: true},
		{IncomingPort: 5353, TCP: true, UDP: true, ForwardHost: "10.0.0.53", ForwardPort: 53, Enabled: true, ProxyProtocolIn: true, ProxyProtocolOut: true},
		{IncomingPort: 9993, TCP: true, ForwardHost: "imap", ForwardPort: 143, CertificateID: &cert.ID, Enabled: true, ProxyProtocolIn: true},
		{IncomingPort: 2526, TCP: true, ForwardHost: "mail", ForwardPort: 25, Enabled: true, ProxyProtocolOut: true},
		{IncomingPort: 2527, TCP: true, ForwardHost: "mail", ForwardPort: 25, Enabled: true},
	} {
		if err := st.SaveStream(ctx, s); err != nil {
			t.Fatal(err)
		}
	}

	// Without trusted PROXY protocol sources, a stream accepting it is not rendered.
	if err := c.Render(ctx, st, filepath.Join(dir, "v0")); err == nil || !strings.Contains(err.Error(), "stream 1") ||
		!strings.Contains(err.Error(), "trusted PROXY protocol sources") {
		t.Fatalf("rendered without trusted sources: %v", err)
	}
	// The HTTP listeners stay off: only the trusted list is used.
	if err := st.SetNetwork(ctx, store.NetworkSettings{RealIPHeader: "X-Forwarded-For",
		ProxyProtocol: store.ProxyProtocol{HTTPPort: 8080, HTTPSPort: 8443, Trusted: []string{"10.8.0.2", "fd00::/64"}}}); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(dir, "v1")
	if err := c.Render(ctx, st, out); err != nil {
		t.Fatal(err)
	}
	read := func(name string) string { return readFile(t, filepath.Join(out, name)) }
	check := func(name string, want, deny []string) {
		t.Helper()
		conf := read(name)
		for _, w := range want {
			if !strings.Contains(conf, w) {
				t.Errorf("%s: missing %q\n%s", name, w, conf)
			}
		}
		for _, d := range deny {
			if strings.Contains(conf, d) {
				t.Errorf("%s: unexpected %q\n%s", name, d, conf)
			}
		}
	}
	trusted := []string{"set_real_ip_from 10.8.0.2/32;", "set_real_ip_from fd00::/64;"}
	check("streams/1.conf", append([]string{"listen 2525 proxy_protocol;", "listen [::]:2525 proxy_protocol;"}, trusted...),
		[]string{"proxy_protocol on;"})
	// TCP and UDP: only the TCP listener expects (and the TCP proxy sends) the header.
	check("streams/2.conf", append([]string{"listen 5353 proxy_protocol;", "listen [::]:5353 proxy_protocol;",
		"listen 5353 udp;", "listen [::]:5353 udp;", "proxy_protocol on;"}, trusted...),
		[]string{"udp proxy_protocol", "listen 5353;"})
	if s2 := read("streams/2.conf"); strings.Index(s2, "proxy_protocol on;") > strings.Index(s2, "udp;") {
		t.Errorf("proxy_protocol on outside the TCP server:\n%s", s2)
	}
	check("streams/3.conf", append([]string{"listen 9993 ssl proxy_protocol;", "listen [::]:9993 ssl proxy_protocol;"}, trusted...), nil)
	check("streams/4.conf", []string{"listen 2526;", "listen [::]:2526;", "proxy_protocol on;"}, []string{"set_real_ip_from", "listen 2526 proxy_protocol"})
	check("streams/5.conf", []string{"listen 2527;"}, []string{"proxy_protocol", "set_real_ip_from"})
	// The HTTP listeners are untouched: no PROXY protocol at http level.
	check("nginx.conf", nil, []string{"real_ip_header proxy_protocol", "listen 8080"})
}

func TestStreamPPTrusted(t *testing.T) {
	if _, err := StreamPPTrusted(store.ProxyProtocol{}); err == nil {
		t.Error("empty list accepted")
	}
	if _, err := StreamPPTrusted(store.ProxyProtocol{Trusted: []string{"0.0.0.0/0"}}); err == nil {
		t.Error("too broad range accepted")
	}
	if got, err := StreamPPTrusted(store.ProxyProtocol{Trusted: []string{"192.168.1.10"}}); err != nil || len(got) != 1 || got[0] != "192.168.1.10/32" {
		t.Errorf("got %v %v", got, err)
	}
}

// The trusted list can't be emptied while a stream accepts the PROXY protocol, also
// with the HTTP listeners off.
func TestCheckProxyProtocolStreams(t *testing.T) {
	p := Paths{HTTPPort: 80, HTTPSPort: 443}
	pp := store.ProxyProtocol{HTTPPort: 8080, HTTPSPort: 8443}
	if errs := CheckProxyProtocol(p, pp, []store.Stream{{ID: 3, IncomingPort: 25, TCP: true, ProxyProtocolOut: true}}); len(errs) != 0 {
		t.Errorf("sending stream needs no trusted sources: %v", errs)
	}
	errs := CheckProxyProtocol(p, pp, []store.Stream{{ID: 3, IncomingPort: 25, TCP: true, ProxyProtocolIn: true}})
	if len(errs) != 1 || errs[0].Field != "proxy_protocol.trusted" || !strings.Contains(errs[0].Message, "stream #3") {
		t.Errorf("emptied list with an accepting stream: %v", errs)
	}
	pp.Trusted = []string{"10.0.0.1"}
	if errs := CheckProxyProtocol(p, pp, []store.Stream{{ID: 3, IncomingPort: 25, TCP: true, ProxyProtocolIn: true}}); len(errs) != 0 {
		t.Errorf("trusted list set: %v", errs)
	}
}
