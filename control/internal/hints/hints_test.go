package hints

import "testing"

func TestACME(t *testing.T) {
	cases := []struct{ name, msg, want string }{
		{"issue 200 servfail",
			"issuance failed: error: one or more domains had a problem:\n[*.example.com] propagation: time limit exceeded: last error: authoritative nameservers: NS ns1.example.net. returned SERVFAIL for _acme-challenge.example.com.",
			ACMEDNSServfail},
		{"refused", "[example.com] propagation: time limit exceeded: last error: authoritative nameservers: NS a.ns. returned REFUSED for _acme-challenge.example.com.", ACMEDNSServfail},
		{"propagation", "issuance failed: error: one or more domains had a problem:\n[example.com] propagation: time limit exceeded: last error: NS ns1.example.net.:53 did not return the expected TXT record [fqdn: _acme-challenge.example.com., value: abc]: ", ACMEDNSPropagation},
		{"propagation no last error", "[example.com] propagation: time limit exceeded", ACMEDNSPropagation},
		{"incorrect txt", "acme: error: 403 :: urn:ietf:params:acme:error:unauthorized :: Incorrect TXT record \"x\" found at _acme-challenge.example.com", ACMEDNSPropagation},
		{"rate limited", "acme: error: 429 :: POST :: https://acme-v02.api.letsencrypt.org/acme/new-order :: urn:ietf:params:acme:error:rateLimited :: too many certificates (5) already issued for this exact set of identifiers in the last 168h0m0s", ACMERateLimited},
		{"rate limited wins over propagation", "propagation: time limit exceeded urn:ietf:params:acme:error:rateLimited", ACMERateLimited},
		{"cloudflare zone", "[example.com] acme: error presenting token: cloudflare: failed to find zone example.com.: zone could not be found", ACMEDNSCredential},
		{"status code 403", "[example.com] acme: error presenting token: digitalocean: unexpected status code: [status code: 403] body: {\"id\":\"forbidden\"}", ACMEDNSCredential},
		{"invalid token", "[example.com] acme: error presenting token: duckdns: invalid API token", ACMEDNSCredential},
		{"present other", "[example.com] acme: error presenting token: could not write file", ""},
		{"soa", "[example.com] acme: error presenting token: cloudflare: could not find zone for domain \"example.com\": could not find the start of authority for 'example.com.': NXDOMAIN", ACMEDNSLookup},
		{"recursive", "[example.com] propagation: initial recursive nameserver: read udp: i/o timeout", ACMEDNSLookup},
		{"ns call", "authoritative nameservers: NS call failed: dial udp: i/o timeout", ACMEDNSLookup},
		{"caa", "acme: error: 403 :: urn:ietf:params:acme:error:caa :: CAA record for example.com prevents issuance", ACMECAA},
		{"rejected", "acme: error: 400 :: POST :: https://x/acme/new-order :: urn:ietf:params:acme:error:rejectedIdentifier :: Invalid identifiers requested :: Cannot issue for \"localhost\"", ACMERejectedName},
		{"email", "acme: error: 400 :: POST :: https://x/acme/new-acct :: urn:ietf:params:acme:error:invalidContact :: Error validating contact(s)", ACMEInvalidEmail},
		{"email unsupported", "urn:ietf:params:acme:error:unsupportedContact", ACMEInvalidEmail},
		{"dns", "acme: error: 400 :: urn:ietf:params:acme:error:dns :: DNS problem: NXDOMAIN looking up A for example.com", ACMEDomainDNS},
		{"connection", "acme: error: 400 :: urn:ietf:params:acme:error:connection :: 203.0.113.5: Fetching http://example.com/.well-known/acme-challenge/x: Timeout during connect (likely firewall problem)", ACMEPort80},
		{"wrong server", "acme: error: 403 :: urn:ietf:params:acme:error:unauthorized :: 203.0.113.5: Invalid response from http://example.com/.well-known/acme-challenge/x: 404", ACMEWrongServer},
		{"ca", "get directory at 'https://acme-v02.api.letsencrypt.org/directory': Get \"https://acme-v02.api.letsencrypt.org/directory\": dial tcp: lookup acme-v02.api.letsencrypt.org: no such host", ACMECAUnreachable},
		{"empty", "", ""},
		{"unrelated", "certificate file is not valid PEM", ""},
	}
	for _, c := range cases {
		if got := ACME(c.msg); got != c.want {
			t.Errorf("%s: ACME() = %q, want %q", c.name, got, c.want)
		}
	}
}

func TestDeploy(t *testing.T) {
	cases := []struct{ out, want string }{
		{"nginx: [emerg] unknown directive \"proxy_foo\" in /data/nginx/versions/v000012/hosts/3.conf:25\nnginx: configuration file /data/nginx/versions/v000012/nginx.conf test failed", DeployAdvanced},
		{"nginx: [emerg] \"proxy_pass\" directive is duplicate in /data/nginx/versions/v000002/hosts/redirect-7.conf:10", DeployAdvanced},
		{"nginx: [emerg] invalid number of arguments in \"add_header\" directive in /data/v/streams/4.conf:3", DeployAdvanced},
		{"nginx: [emerg] unknown directive \"x\" in /data/nginx/versions/v1/nginx.conf:10", ""},
		{"nginx: [emerg] cannot load certificate \"/data/certs/3/fullchain.pem\" in /data/v/hosts/3.conf:5", ""},
		{"nginx: [emerg] bind() to 0.0.0.0:8443 failed (98: Address already in use)", DeployPortInUse},
		{"", ""},
	}
	for _, c := range cases {
		if got := Deploy(c.out); got != c.want {
			t.Errorf("Deploy(%q) = %q, want %q", c.out, got, c.want)
		}
	}
}

func TestReload(t *testing.T) {
	for msg, want := range map[string]string{
		"nginx: [emerg] bind() to 0.0.0.0:8443 failed (98: Address already in use)":                                                                 DeployPortInUse,
		"the gateway kept the previous configuration (v000008) instead of v000009; see the gateway log (for example a port that is already in use)": "",
		"": "",
	} {
		if got := Reload(msg); got != want {
			t.Errorf("Reload(%q) = %q, want %q", msg, got, want)
		}
	}
}

func TestNet(t *testing.T) {
	cases := []struct{ msg, want string }{
		{`Get "https://lists.example/x.txt": dial tcp: lookup lists.example on 127.0.0.11:53: no such host`, NetDNS},
		{`dial tcp: lookup x on 127.0.0.11:53: server misbehaving`, NetDNS},
		{`Get "http://10.0.0.1/": dial tcp 10.0.0.1:80: connect: connection refused`, NetRefused},
		{`Get "https://x/": dial tcp 1.2.3.4:443: i/o timeout`, NetTimeout},
		{`Get "https://x/": context deadline exceeded (Client.Timeout exceeded while awaiting headers)`, NetTimeout},
		{`Get "https://x/": tls: failed to verify certificate: x509: certificate signed by unknown authority`, NetTLS},
		{`refusing to connect to non-public address 10.0.0.1`, NetBlocked},
		{`Post "https://x/": destination 192.168.0.1 is not allowed`, NetBlocked},
		{`HTTP 401`, NetAuth},
		{`https://download.example/x: HTTP 403`, NetAuth},
		{`HTTP 404`, NetNotFound},
		{`discovery failed: 404 Not Found: nope`, NetNotFound},
		{`HTTP 500`, ""},
		{`Error 1045 (28000): Access denied for user 'npm'@'172.18.0.3' (using password: YES)`, NetMySQLAuth},
		{`Error 1049 (42000): Unknown database 'npmx'`, NetMySQLDB},
		{`discovery failed: oidc: issuer URL provided to client ("https://a/") did not match the issuer URL returned by provider ("https://a")`, NetOIDCIssuer},
		{`the list has 60000 entries (max 50000)`, ""},
		{``, ""},
	}
	for _, c := range cases {
		if got := Net(c.msg); got != c.want {
			t.Errorf("Net(%q) = %q, want %q", c.msg, got, c.want)
		}
	}
}
