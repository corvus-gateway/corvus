package acme

import "testing"

func TestParseCredentials(t *testing.T) {
	c, err := ParseCredentials("cloudflare", "# Cloudflare API token\ndns_cloudflare_api_token = abc123\n")
	if err != nil || c.APIToken != "abc123" {
		t.Fatalf("token: %+v %v", c, err)
	}
	if _, err := ParseCredentials("cloudflare", Providers[0].Template); err == nil {
		t.Fatal("unchanged template placeholder must be rejected")
	}
	if c, err := ParseCredentials("cloudflare", "dns_cloudflare_email=a@b.c\ndns_cloudflare_api_key=k"); err != nil || c.APIKey != "k" {
		t.Fatalf("legacy key: %v", err)
	}
	if _, err := ParseCredentials("cloudflare", "# empty"); err == nil {
		t.Fatal("missing token accepted")
	}
}

func TestDirectoryScope(t *testing.T) {
	s := &Service{Directory: "https://pebble:14000/dir", DirectorySuffix: ".corvus.test"}
	if got := s.directory(false, []string{"a.corvus.test", "*.b.corvus.test"}); got != s.Directory {
		t.Fatalf("test domains: %s", got)
	}
	if got := s.directory(false, []string{"www.example.com"}); got == s.Directory {
		t.Fatal("real domain sent to the test CA")
	}
	if got := s.directory(false, []string{"a.corvus.test", "www.example.com"}); got == s.Directory {
		t.Fatal("mixed domains sent to the test CA")
	}
	if !sameCA("https://pebble:14000/my-account/1", "https://pebble:14000/dir") || sameCA("https://pebble:14000/my-account/1", "https://acme-v02.api.letsencrypt.org/directory") {
		t.Fatal("sameCA")
	}
}

func TestProviders(t *testing.T) {
	creds := map[string]string{
		"digitalocean": "dns_digitalocean_token = do-token",
		"duckdns":      "dns_duckdns_token = duck-token",
		"gandi":        "dns_gandi_token = pat",
		"godaddy":      "dns_godaddy_key = k\ndns_godaddy_secret = s",
		"hetzner":      "dns_hetzner_api_token = legacy", // NPM's certbot-dns-hetzner key
		"namecheap":    "dns_namecheap_username = u\ndns_namecheap_api_key = k",
	}
	for _, p := range Providers {
		if _, err := ParseCredentials(p.ID, p.Template); err == nil {
			t.Errorf("%s: unchanged template accepted", p.ID)
		}
		if p.ID == "cloudflare" {
			continue
		}
		text, ok := creds[p.ID]
		if !ok {
			t.Fatalf("%s: no test credentials", p.ID)
		}
		c, err := ParseCredentials(p.ID, "# comment\n"+text+"\nunrelated_key = x")
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		if _, ok := c.Values["unrelated_key"]; ok {
			t.Errorf("%s: unrelated key kept", p.ID)
		}
		if _, err := ParseCredentials(p.ID, "# empty"); err == nil {
			t.Errorf("%s: missing credentials accepted", p.ID)
		}
		if p.ID == "namecheap" {
			continue // building it looks up the public IP over the network
		}
		if _, err := dnsProvider(p.ID, c); err != nil {
			t.Errorf("%s: build: %v", p.ID, err)
		}
	}
	// stored Cloudflare credentials (named fields) still build
	if _, err := dnsProvider("cloudflare", Credentials{APIToken: "t"}); err != nil {
		t.Fatal(err)
	}
	if _, err := dnsProvider("route53", Credentials{}); err == nil {
		t.Fatal("unknown provider accepted")
	}
}
