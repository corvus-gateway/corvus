package acme

import (
	"testing"
	"time"

	"github.com/go-acme/lego/v4/challenge"
)

// Every DNS provider waits at least MinPropagation for the record (#200); longer
// provider defaults (Gandi, Namecheap) are kept.
func TestDNSPropagationTimeout(t *testing.T) {
	values := map[string]string{
		"dns_cloudflare_api_token": "t", "dns_digitalocean_token": "t", "dns_duckdns_token": "t",
		"dns_gandi_token": "t", "dns_godaddy_key": "k", "dns_godaddy_secret": "s",
		"dns_hetzner_cloud_token": "t", "dns_namecheap_username": "u", "dns_namecheap_api_key": "k",
	}
	longer := map[string]time.Duration{"gandi": 20 * time.Minute, "namecheap": time.Hour}
	for _, p := range Providers {
		prov, err := p.build(values)
		if err != nil {
			t.Fatalf("%s: %v", p.ID, err)
		}
		pt, ok := prov.(challenge.ProviderTimeout)
		if !ok {
			t.Fatalf("%s: no propagation timeout", p.ID)
		}
		timeout, interval := pt.Timeout()
		want := MinPropagation
		if d, ok := longer[p.ID]; ok {
			want = d
		}
		if timeout != want || interval <= 0 || interval > time.Minute {
			t.Errorf("%s: timeout %v interval %v, want timeout %v", p.ID, timeout, interval, want)
		}
	}
	d := 30 * time.Minute
	if atLeast(&d); d != 30*time.Minute {
		t.Fatal("a longer timeout was shortened")
	}
}
