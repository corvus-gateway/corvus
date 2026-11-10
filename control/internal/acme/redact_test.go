package acme

import (
	"strings"
	"testing"
)

func TestRedactError(t *testing.T) {
	duck := Credentials{Values: map[string]string{"dns_duckdns_token": "a1b2c3d4-e5f6-7890-abcd-ef0123456789"}}
	msg := `duckdns: request failed: Get "https://www.duckdns.org/update?domains=x&token=a1b2c3d4-e5f6-7890-abcd-ef0123456789&txt=abc": dial tcp: i/o timeout`
	got := redactError(msg, duck)
	if strings.Contains(got, "a1b2c3d4") || strings.Contains(got, "token=") || !strings.Contains(got, `"https://www.duckdns.org/update?…"`) {
		t.Fatalf("got %q", got)
	}

	nc := Credentials{Values: map[string]string{"dns_namecheap_username": "alice", "dns_namecheap_api_key": "k3y/with+chars"}}
	msg = "namecheap: key k3y/with+chars rejected; encoded k3y%2Fwith%2Bchars; GET https://api.namecheap.com/xml.response#frag; user alice"
	got = redactError(msg, nc)
	for _, leak := range []string{"k3y", "alice", "#frag"} {
		if strings.Contains(got, leak) {
			t.Fatalf("%q leaked in %q", leak, got)
		}
	}
	if !strings.Contains(got, "https://api.namecheap.com/xml.response?…") {
		t.Fatalf("got %q", got)
	}

	cf := Credentials{APIToken: "cf-token-123456", Email: "ops@example.com", APIKey: "x"}
	got = redactError("cloudflare: token cf-token-123456 for ops@example.com invalid (x)", cf)
	if got != "cloudflare: token *** for *** invalid (x)" {
		t.Fatalf("got %q", got)
	}
	if got := redactError("plain error", Credentials{}); got != "plain error" {
		t.Fatalf("got %q", got)
	}
}
