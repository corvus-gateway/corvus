package acme

import (
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/go-acme/lego/v4/challenge"
	"github.com/go-acme/lego/v4/providers/dns/cloudflare"
	"github.com/go-acme/lego/v4/providers/dns/digitalocean"
	"github.com/go-acme/lego/v4/providers/dns/duckdns"
	"github.com/go-acme/lego/v4/providers/dns/gandiv5"
	"github.com/go-acme/lego/v4/providers/dns/godaddy"
	"github.com/go-acme/lego/v4/providers/dns/hetzner"
	"github.com/go-acme/lego/v4/providers/dns/namecheap"
)

// Provider describes a supported DNS-01 provider and the credential template
// shown in the UI. Credentials use certbot's ini keys, so values copied from
// Nginx Proxy Manager work unchanged (NPM uses the same provider ids).
//
// Only lego providers that need no extra Go modules are included; providers
// with large SDKs (Route 53, Azure, Google Cloud) or extra modules (IONOS) are
// intentionally left out.
type Provider struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Template string `json:"credentials_template"`

	// oneOf lists alternative required key sets; any complete set is accepted.
	oneOf [][]string
	build func(v map[string]string) (challenge.Provider, error)
}

// PlaceholderToken is the example value in the templates; it is rejected so a
// forgotten edit fails at save time instead of during issuance.
const PlaceholderToken = "0123456789abcdef0123456789abcdef01234567"

func tmpl(lines ...string) string {
	return strings.ReplaceAll(strings.Join(lines, "\n"), "TOKEN", PlaceholderToken)
}

// MinPropagation is the shortest time Corvus waits for the DNS-01 record to appear on the
// domain's authoritative name servers. lego's defaults (1–2 minutes for several providers)
// are too short for some DNS hosts; issuance runs in the background, so waiting longer
// only delays a failure.
const MinPropagation = 10 * time.Minute

func atLeast(d *time.Duration) {
	if *d < MinPropagation {
		*d = MinPropagation
	}
}

var Providers = []Provider{
	{ID: "cloudflare", Name: "Cloudflare",
		Template: tmpl("# Cloudflare API token (Zone · DNS · Edit)", "dns_cloudflare_api_token=TOKEN"),
		oneOf:    [][]string{{"dns_cloudflare_api_token"}, {"dns_cloudflare_email", "dns_cloudflare_api_key"}},
		build: func(v map[string]string) (challenge.Provider, error) {
			cfg := cloudflare.NewDefaultConfig()
			cfg.AuthToken = v["dns_cloudflare_api_token"]
			if cfg.AuthToken == "" {
				cfg.AuthEmail, cfg.AuthKey = v["dns_cloudflare_email"], v["dns_cloudflare_api_key"]
			}
			atLeast(&cfg.PropagationTimeout)
			return cloudflare.NewDNSProviderConfig(cfg)
		}},
	{ID: "digitalocean", Name: "DigitalOcean",
		Template: tmpl("# DigitalOcean API token (read & write)", "dns_digitalocean_token=TOKEN"),
		oneOf:    [][]string{{"dns_digitalocean_token"}},
		build: func(v map[string]string) (challenge.Provider, error) {
			cfg := digitalocean.NewDefaultConfig()
			cfg.AuthToken = v["dns_digitalocean_token"]
			atLeast(&cfg.PropagationTimeout)
			return digitalocean.NewDNSProviderConfig(cfg)
		}},
	{ID: "duckdns", Name: "DuckDNS",
		Template: tmpl("# DuckDNS token", "dns_duckdns_token=TOKEN"),
		oneOf:    [][]string{{"dns_duckdns_token"}},
		build: func(v map[string]string) (challenge.Provider, error) {
			cfg := duckdns.NewDefaultConfig()
			cfg.Token = v["dns_duckdns_token"]
			atLeast(&cfg.PropagationTimeout)
			return duckdns.NewDNSProviderConfig(cfg)
		}},
	{ID: "gandi", Name: "Gandi",
		Template: tmpl("# Gandi personal access token (Domain · Manage technical configurations)", "dns_gandi_token=TOKEN", "# or the legacy API key: dns_gandi_api_key="),
		oneOf:    [][]string{{"dns_gandi_token"}, {"dns_gandi_api_key"}},
		build: func(v map[string]string) (challenge.Provider, error) {
			cfg := gandiv5.NewDefaultConfig()
			cfg.PersonalAccessToken, cfg.APIKey = v["dns_gandi_token"], v["dns_gandi_api_key"]
			atLeast(&cfg.PropagationTimeout)
			return gandiv5.NewDNSProviderConfig(cfg)
		}},
	{ID: "godaddy", Name: "GoDaddy",
		Template: tmpl("# GoDaddy production API key and secret", "dns_godaddy_key=TOKEN", "dns_godaddy_secret="),
		oneOf:    [][]string{{"dns_godaddy_key", "dns_godaddy_secret"}},
		build: func(v map[string]string) (challenge.Provider, error) {
			cfg := godaddy.NewDefaultConfig()
			cfg.APIKey, cfg.APISecret = v["dns_godaddy_key"], v["dns_godaddy_secret"]
			atLeast(&cfg.PropagationTimeout)
			return godaddy.NewDNSProviderConfig(cfg)
		}},
	{ID: "hetzner", Name: "Hetzner",
		Template: tmpl("# Hetzner Cloud API token (read & write)", "dns_hetzner_cloud_token=TOKEN", "# or a legacy DNS Console token: dns_hetzner_api_token="),
		oneOf:    [][]string{{"dns_hetzner_cloud_token"}, {"dns_hetzner_api_token"}},
		build: func(v map[string]string) (challenge.Provider, error) {
			cfg := hetzner.NewDefaultConfig()
			// certbot-dns-hetzner (used by NPM) calls the legacy DNS Console token dns_hetzner_api_token.
			cfg.APIToken, cfg.APIKey = v["dns_hetzner_cloud_token"], v["dns_hetzner_api_token"]
			if cfg.APIToken != "" {
				cfg.APIKey = ""
			}
			atLeast(&cfg.PropagationTimeout)
			return hetzner.NewDNSProviderConfig(cfg)
		}},
	{ID: "namecheap", Name: "Namecheap",
		Template: tmpl("# Namecheap API user and key (the server's public IP must be whitelisted)", "dns_namecheap_username=", "dns_namecheap_api_key=TOKEN"),
		oneOf:    [][]string{{"dns_namecheap_username", "dns_namecheap_api_key"}},
		build: func(v map[string]string) (challenge.Provider, error) {
			cfg := namecheap.NewDefaultConfig()
			cfg.APIUser, cfg.APIKey = v["dns_namecheap_username"], v["dns_namecheap_api_key"]
			atLeast(&cfg.PropagationTimeout)
			return namecheap.NewDNSProviderConfig(cfg)
		}},
}

// ProviderByID returns the provider with the given id.
func ProviderByID(id string) (*Provider, bool) {
	for i := range Providers {
		if Providers[i].ID == id {
			return &Providers[i], true
		}
	}
	return nil, false
}

// ParseCredentials reads certbot-style "key = value" credentials for a provider and
// keeps the keys that provider uses.
func ParseCredentials(provider, text string) (Credentials, error) {
	p, ok := ProviderByID(provider)
	if !ok {
		return Credentials{}, errors.New("unsupported DNS provider")
	}
	kv := map[string]string{}
	for _, line := range strings.Split(text, "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") || strings.HasPrefix(line, ";") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			return Credentials{}, errors.New("each line must be key = value")
		}
		kv[strings.TrimSpace(k)] = strings.Trim(strings.TrimSpace(v), `"'`)
	}
	values := map[string]string{}
	for _, set := range p.oneOf {
		for _, k := range set {
			if kv[k] == PlaceholderToken {
				return Credentials{}, fmt.Errorf("replace the example value of %s with your %s credentials", k, p.Name)
			}
			if kv[k] != "" {
				values[k] = kv[k]
			}
		}
	}
	for _, set := range p.oneOf {
		complete := true
		for _, k := range set {
			complete = complete && values[k] != ""
		}
		if complete {
			c := Credentials{Values: values}
			if provider == "cloudflare" { // stored in the original fields for older versions
				c = Credentials{APIToken: values["dns_cloudflare_api_token"], Email: values["dns_cloudflare_email"], APIKey: values["dns_cloudflare_api_key"]}
			}
			return c, nil
		}
	}
	alts := make([]string, len(p.oneOf))
	for i, set := range p.oneOf {
		alts[i] = strings.Join(set, " + ")
	}
	return Credentials{}, fmt.Errorf("%s needs %s", p.Name, strings.Join(alts, " or "))
}

// values returns the credentials as certbot keys (Cloudflare's are stored in named fields).
func (c Credentials) values() map[string]string {
	if c.Values != nil {
		return c.Values
	}
	return map[string]string{"dns_cloudflare_api_token": c.APIToken, "dns_cloudflare_email": c.Email, "dns_cloudflare_api_key": c.APIKey}
}

// dnsProvider builds the lego DNS-01 provider for stored credentials.
func dnsProvider(provider string, c Credentials) (challenge.Provider, error) {
	p, ok := ProviderByID(provider)
	if !ok {
		return nil, fmt.Errorf("DNS provider %q is not supported", provider)
	}
	return p.build(c.values())
}
