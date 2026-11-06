// Package compiler renders the stored configuration into a self-contained
// OpenResty configuration directory (nginx.conf, hosts/*.conf, policy.json).
// Every version directory references only its own files, so it can be tested
// with `openresty -t` before it becomes live and kept for rollback.
package compiler

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"

	"corvus/control/internal/netcfg"
	"corvus/control/internal/sigs"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

//go:embed templates/*.tmpl
var templateFS embed.FS

var tmpl = template.Must(template.New("").Funcs(template.FuncMap{
	"join": strings.Join,
	// rxpath turns a validated path (letters, digits, ._~/-) into a regex prefix.
	"rxpath": func(p string) string {
		p = strings.TrimRight(p, "/")
		return strings.NewReplacer(".", `\.`).Replace(p)
	},
}).ParseFS(templateFS, "templates/*.tmpl"))

// Paths are absolute locations inside the gateway container.
type Paths struct {
	DataDir     string // e.g. /data
	LuaDir      string // e.g. /opt/corvus/lua
	HTMLDir     string // e.g. /opt/corvus/html
	MimeTypes   string // e.g. /usr/local/openresty/nginx/conf/mime.types
	Resolver    string // DNS resolver for upstream names, e.g. 127.0.0.11 (Docker)
	HTTPPort    int    // container port for HTTP (80)
	HTTPSPort   int    // container port for HTTPS (443)
	StatusPort  int    // loopback-only status port used to confirm reloads (9180)
	ACMEPort    int    // loopback port where corvus-control answers HTTP-01 challenges (9181)
	DefaultCert string // directory holding the default fullchain.pem/privkey.pem
	// IPv6 adds `listen [::]:port` to every public server (only when the system has
	// IPv6: nginx fails to start if it cannot bind an IPv6 socket).
	IPv6 bool
	// ResolverIPv6 lets the resolver return IPv6 (AAAA) answers for forward hosts.
	// Off by default: on a host without IPv6 routes a name with both A and AAAA
	// records would intermittently fail.
	ResolverIPv6 bool
	AdminPort    int // admin console/API port inside the container (9443); 0 = unknown
	RedisPort    int // loopback port of the bundled Valkey (6379); 0 = none
}

// ReservedPorts are the container ports Corvus itself listens on: streams may not use them.
func (p Paths) ReservedPorts() map[int]string {
	out := map[int]string{}
	for _, r := range []struct {
		port int
		what string
	}{{p.HTTPPort, "HTTP"}, {p.HTTPSPort, "HTTPS"}, {p.StatusPort, "gateway status"}, {p.ACMEPort, "ACME challenges"},
		{p.AdminPort, "admin console"}, {p.RedisPort, "access-queue store (Valkey)"}} {
		if r.port > 0 {
			out[r.port] = r.what
		}
	}
	return out
}

// CertDir is where certificate files for a certificate id live.
func (p Paths) CertDir(id int64) string {
	return filepath.ToSlash(filepath.Join(p.DataDir, "certs", fmt.Sprint(id)))
}

type Compiler struct {
	P Paths
	// Cloudflare returns the trusted Cloudflare edge ranges (IPv4 + IPv6).
	Cloudflare func(ctx context.Context, s *store.Store) []string
	// Bundle returns the active managed rule bundle (body signatures); nil = built-in.
	Bundle func() *sigs.Bundle
	// Hosts returns static name → IP entries (e.g. StaticHosts("/etc/hosts")); forward
	// hosts found there are rendered as their address. nil = none.
	Hosts func() map[string]string
	// HostGateway returns the Docker host's address (see HostGateway); loopback forward
	// hosts are rendered with it. "" or nil = loopback stays loopback.
	HostGateway func() string
}

type hostView struct {
	store.ProxyHost
	Room            *store.Waitroom
	// RoomEndpoints is the room the host is attached to, also while it is disabled, so
	// visitors still on the waiting page learn that they can go in.
	RoomEndpoints *store.Waitroom
	CertDir         string
	HasRootLocation bool
	Upstream        string // forward host address (see upstreamAddr)
	Host            target // the host's forward host
	Root            target // what `location /` (and the access queue location) proxies to
	Locations       []target
	LocUpstreams    []target // upstream blocks of the custom locations (including "/")
	OwnAccessLog    bool     // the advanced configuration sets access_log (keep it working)
	RateLimit       *store.RateLimit
	Access          *store.AccessList
	AccessFile      string
	Cloudflare      []string
	TrustedProxies  []string
	RealIPHeader    string
	ErrorCodes      []int  // upstream errors answered with the error page (see UpstreamErrorCodes)
	UploadLimit     string // client_max_body_size for the server ("" = global default)
	GzipOff         bool   // the host turned compression off
	P               Paths
}

// target is an upstream block and how a location proxies to it: the host's forward
// host or a custom location.
type target struct {
	Prefix    string // custom location path ("" = the host's forward host)
	Name      string // upstream block name
	Scheme    string // http | https
	Addr      string // server address (see upstreamAddr)
	Port      int
	Resolve   bool   // Addr is a name: resolved at run time (server … resolve)
	Path      string // forward path replacing the location prefix ("" = URI unchanged)
	Websocket bool
	// Console: the target is on the admin console port, so the request carries the
	// gateway key that lets the console trust X-Forwarded-For (see api.ConsoleProxies).
	Console bool
	// UpstreamTimeoutS is the host's upstream read/send timeout (nil = default).
	UpstreamTimeoutS *int
	ErrorCodes       []int
	RateLimit        *store.RateLimit
	Advanced         string // location advanced configuration
}

// targets builds the host's upstream targets: its forward host, the custom locations,
// and the target of the root location (a custom location "/" replaces the forward host).
func (v *hostView) targets(static map[string]string, hostGW string) {
	mk := func(name, scheme, host string, port int) target {
		addr := upstreamAddr(host, static, hostGW)
		return target{Name: name, Scheme: scheme, Addr: addr, Port: port, Resolve: !isIP(addr),
			Console:   v.P.AdminPort > 0 && port == v.P.AdminPort,
			Websocket: v.Websocket, UpstreamTimeoutS: v.UpstreamTimeoutS, ErrorCodes: v.ErrorCodes, RateLimit: v.RateLimit}
	}
	v.Host = mk(fmt.Sprintf("corvus_up_%d", v.ID), v.ForwardScheme, v.ForwardHost, v.ForwardPort)
	v.Upstream = v.Host.Addr
	v.Root = v.Host
	for i, l := range v.ProxyHost.Locations {
		t := mk(fmt.Sprintf("corvus_up_%d_%d", v.ID, i+1), l.ForwardScheme, l.ForwardHost, l.ForwardPort)
		t.Prefix, t.Path, t.Advanced = l.Path, l.ForwardPath, l.AdvancedConfig
		if strings.Contains(l.AdvancedConfig, "error_page") {
			t.ErrorCodes = nil // keep the operator's error_page (see the host's advanced config)
		}
		v.LocUpstreams = append(v.LocUpstreams, t)
		if l.Path == "/" {
			v.Root = t
		} else {
			v.Locations = append(v.Locations, t)
		}
	}
}

// gzipDirective finds a gzip on/off directive in an advanced configuration.
var gzipDirective = regexp.MustCompile(`(?m)(?:^|[;{])\s*gzip\s`)

// UpstreamErrorCodes are the nginx-generated upstream errors (connection refused,
// timeouts, invalid responses) answered with the error page. Responses the upstream
// itself sends with these codes are passed through unchanged (no proxy_intercept_errors).
var UpstreamErrorCodes = []int{502, 503, 504}

// errorCodes leaves out a code a rate limit policy answers with: that status keeps
// its own error_page (@corvus_limited).
func errorCodes(rl *store.RateLimit) []int {
	var out []int
	for _, c := range UpstreamErrorCodes {
		if rl == nil || rl.StatusCode != c {
			out = append(out, c)
		}
	}
	return out
}

// customPages validates the stored custom pages and writes them into the version
// directory (pages/), so they are tested, activated and rolled back with the rest of
// the configuration. Built-in pages are referenced from the HTML directory.
func (c *Compiler) customPages(ctx context.Context, s *store.Store, dir string, rules *rulesFile) error {
	pages, err := s.CustomPages(ctx)
	if err != nil {
		return err
	}
	for _, p := range []struct {
		field, html, file string
		target            *string
	}{
		{"block", pages.Block, "blocked.html", &rules.BlockPage},
		{"error", pages.Error, "error.html", &rules.ErrorPage},
	} {
		if p.html == "" {
			continue
		}
		if err := validate.CustomPage(p.field, p.html); err != nil {
			return fmt.Errorf("custom %s page: %w", p.field, err)
		}
		if err := os.MkdirAll(filepath.Join(dir, "pages"), 0o755); err != nil {
			return err
		}
		file := filepath.Join(dir, "pages", p.file)
		if err := os.WriteFile(file, []byte(p.html), 0o644); err != nil {
			return err
		}
		*p.target = filepath.ToSlash(file)
	}
	return nil
}

type mainView struct {
	Dir        string
	Name       string // version directory name, served on the status port
	P          Paths
	Hosts      int
	Redirects  int
	Streams    int // enabled streams; the stream {} block is rendered only when > 0
	RateLimits []store.RateLimit
	Unknown    store.UnknownHosts // what the HTTP default server answers (validated)
}

type redirectView struct {
	store.RedirectionHost
	CertDir         string
	HasRootLocation bool
	RuleKey         string
	Target          string
	P               Paths
}

type policyRoom struct {
	Capacity  int    `json:"capacity"`
	ActiveTTL int    `json:"active_ttl"`
	GrantTTL  int    `json:"grant_ttl"`
	QueueTTL  int    `json:"queue_ttl"`
	Refresh   int    `json:"refresh"`
	AdmitRate int    `json:"admit_rate"`
	Fail      string `json:"fail"`
	MaxQueue  int    `json:"max_queue"`
	MaxNewIP  int    `json:"max_new_per_ip"`
	BindIP    bool   `json:"bind_ip"`
	Enabled   bool   `json:"enabled"`
}

// Render writes a complete configuration into dir (which must not exist yet).
func (c *Compiler) Render(ctx context.Context, s *store.Store, dir string) error {
	hosts, err := s.ListProxyHosts(ctx)
	if err != nil {
		return err
	}
	rooms, err := s.ListWaitrooms(ctx)
	if err != nil {
		return err
	}
	byID := map[int64]*store.Waitroom{}
	policy := map[string]map[string]policyRoom{"waitrooms": {}}
	for i := range rooms {
		r := &rooms[i]
		byID[r.ID] = r
		// Every room is published so status endpoints keep working while a room is disabled.
		rc := *r
		if err := validate.Waitroom(&rc); err != nil {
			return fmt.Errorf("access queue %d: %w", r.ID, err)
		}
		policy["waitrooms"][r.Name] = policyRoom{r.Capacity, r.ActiveTTL, r.GrantTTL, r.QueueTTL, r.Refresh, r.AdmitRate, r.FailMode, r.MaxQueue, r.MaxNewIP, r.BindIP, r.Enabled}
	}

	limits, err := s.ListRateLimits(ctx)
	if err != nil {
		return err
	}
	limitByID := map[int64]*store.RateLimit{}
	var enabledLimits []store.RateLimit
	for i := range limits {
		rc := limits[i]
		if err := validate.RateLimit(&rc); err != nil {
			return fmt.Errorf("rate limit %d: %w", limits[i].ID, err)
		}
		if _, err := netcfg.NormalizeTrustedCIDRs(limits[i].ExemptCIDRs); err != nil {
			return fmt.Errorf("rate limit %d: %w", limits[i].ID, err)
		}
		if limits[i].Enabled {
			limitByID[limits[i].ID] = &limits[i]
			enabledLimits = append(enabledLimits, limits[i])
		}
	}
	acls, err := s.ListAccessLists(ctx)
	if err != nil {
		return err
	}
	aclByID := map[int64]*store.AccessList{}
	for i := range acls {
		if len(acls[i].Users) == 0 {
			continue // nothing to enforce (IP rules live in security rules)
		}
		ac := acls[i]
		if err := validate.AccessList(&ac); err != nil {
			return fmt.Errorf("access list %d: %w", acls[i].ID, err)
		}
		for _, u := range acls[i].Users {
			if strings.ContainsAny(u.PasswordHash, ":\n\r") || !htpasswdHash(u.PasswordHash) {
				return fmt.Errorf("access list %d: invalid password hash", acls[i].ID)
			}
		}
		aclByID[acls[i].ID] = &acls[i]
	}
	network, err := s.Network(ctx)
	if err != nil {
		return err
	}
	if _, err := netcfg.NormalizeTrustedCIDRs(network.TrustedProxies); err != nil {
		return fmt.Errorf("network settings: %w", err)
	}
	if !validate.RealIPHeader(network.RealIPHeader) {
		return fmt.Errorf("network settings: invalid real IP header")
	}
	unknown, err := s.UnknownHosts(ctx)
	if err != nil {
		return err
	}
	if err := validate.UnknownHosts(&unknown); err != nil {
		return fmt.Errorf("unknown host setting: %w", err)
	}
	var cf []string
	if c.Cloudflare != nil {
		cf = c.Cloudflare(ctx, s)
	}

	for _, d := range []string{"hosts", "access"} {
		if err := os.MkdirAll(filepath.Join(dir, d), 0o755); err != nil {
			return err
		}
	}
	slash := filepath.ToSlash(dir)

	redirects, err := s.ListRedirectionHosts(ctx)
	if err != nil {
		return err
	}
	rules, err := c.rulePolicy(ctx, s, hosts, redirects)
	if err != nil {
		return err
	}
	if err := c.customPages(ctx, s, dir, rules); err != nil {
		return err
	}

	var static map[string]string
	if c.Hosts != nil {
		static = c.Hosts()
	}
	hostGW := ""
	if c.HostGateway != nil {
		hostGW = c.HostGateway()
	}

	n := 0
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		// Defense in depth: never render a host that would not pass validation.
		hc := h
		if err := validate.ProxyHost(&hc); err != nil {
			return fmt.Errorf("proxy host %d: %w", h.ID, err)
		}
		v := hostView{ProxyHost: hc, P: c.P, HasRootLocation: validate.HasRootLocation(hc.AdvancedConfig)}
		v.OwnAccessLog = strings.Contains(hc.AdvancedConfig, "access_log")
		// Upload size and gzip are server-level directives: an advanced configuration that
		// sets them itself (common in NPM setups) keeps its own (nginx rejects duplicates).
		if hc.MaxUploadMB != nil && !strings.Contains(hc.AdvancedConfig, "client_max_body_size") {
			v.UploadLimit = "0"
			if *hc.MaxUploadMB > 0 {
				v.UploadLimit = fmt.Sprintf("%dm", *hc.MaxUploadMB)
			}
		}
		v.GzipOff = !hc.Gzip && !gzipDirective.MatchString(hc.AdvancedConfig)
		v.CertDir = c.certDir(hc.CertificateID)
		if hc.WaitroomID != nil {
			if r := byID[*hc.WaitroomID]; r != nil {
				v.RoomEndpoints = r
				if r.Enabled {
					v.Room = r
				}
			}
		}
		if hc.RateLimitID != nil {
			v.RateLimit = limitByID[*hc.RateLimitID]
		}
		// Advanced configuration with its own error_page (common in NPM setups) keeps
		// it: a location-level error_page would override the server-level one.
		if !strings.Contains(hc.AdvancedConfig, "error_page") {
			v.ErrorCodes = errorCodes(v.RateLimit)
		}
		v.targets(static, hostGW)
		for _, t := range append([]target{v.Host}, v.LocUpstreams...) {
			if why := c.P.InternalTarget(t.Addr, t.Port); why != "" {
				return fmt.Errorf("proxy host %d: %s", h.ID, why)
			}
		}
		if hc.AccessListID != nil {
			if a := aclByID[*hc.AccessListID]; a != nil {
				v.Access = a
				if len(a.Users) > 0 {
					file := filepath.Join(dir, "access", fmt.Sprintf("%d.htpasswd", a.ID))
					if err := writeHtpasswd(file, a.Users); err != nil {
						return err
					}
					v.AccessFile = filepath.ToSlash(file)
				}
			}
		}
		switch hc.ClientIP {
		case "cloudflare":
			v.Cloudflare = cf
		case "trusted_proxies":
			v.TrustedProxies, v.RealIPHeader = network.TrustedProxies, network.RealIPHeader
		}
		if err := writeTemplate(filepath.Join(dir, "hosts", fmt.Sprintf("%d.conf", h.ID)), "host.conf.tmpl", v); err != nil {
			return err
		}
		n++
	}

	nr := 0
	for _, h := range redirects {
		if !h.Enabled {
			continue
		}
		hc := h
		if err := validate.RedirectionHost(&hc); err != nil {
			return fmt.Errorf("redirection host %d: %w", h.ID, err)
		}
		v := redirectView{RedirectionHost: hc, P: c.P, HasRootLocation: validate.HasRootLocation(hc.AdvancedConfig),
			RuleKey: redirectRuleKey(hc.ID), Target: redirectTarget(hc), CertDir: c.certDir(hc.CertificateID)}
		if err := writeTemplate(filepath.Join(dir, "hosts", fmt.Sprintf("redirect-%d.conf", h.ID)), "redirect.conf.tmpl", v); err != nil {
			return err
		}
		nr++
	}

	ns, err := c.renderStreams(ctx, s, dir, static, hostGW)
	if err != nil {
		return err
	}

	rj, _ := json.Marshal(rules)
	if err := os.WriteFile(filepath.Join(dir, "rules.json"), rj, 0o644); err != nil {
		return err
	}
	pj, _ := json.MarshalIndent(policy, "", "  ")
	if err := os.WriteFile(filepath.Join(dir, "policy.json"), pj, 0o644); err != nil {
		return err
	}
	return writeTemplate(filepath.Join(dir, "nginx.conf"), "nginx.conf.tmpl", mainView{Dir: slash, Name: filepath.Base(dir), P: c.P, Hosts: n, Redirects: nr, Streams: ns, RateLimits: enabledLimits, Unknown: unknown})
}

// certDir returns the certificate directory for id, or "" while its files don't exist
// yet (an ACME order may still be pending): TLS is enabled once they do.
func (c *Compiler) certDir(id *int64) string {
	if id == nil {
		return ""
	}
	dir := c.P.CertDir(*id)
	if _, err := os.Stat(filepath.Join(filepath.FromSlash(dir), "fullchain.pem")); err != nil {
		return ""
	}
	return dir
}

// redirectTarget is the URL a redirection host answers with ($scheme keeps the
// request's scheme; $request_uri keeps path and query).
func redirectTarget(h store.RedirectionHost) string {
	scheme := h.ForwardScheme
	if scheme == "auto" {
		scheme = "$scheme"
	}
	t := scheme + "://" + h.ForwardDomain
	if h.PreservePath {
		t = strings.TrimRight(t, "/") + "$request_uri"
	}
	return t
}

func writeTemplate(path, name string, data any) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	if err := tmpl.ExecuteTemplate(f, name, data); err != nil {
		f.Close()
		return fmt.Errorf("render %s: %w", name, err)
	}
	return f.Close()
}

// htpasswdHash reports whether h is a hash format the gateway accepts: bcrypt (current)
// or apr1 (created by earlier versions). Plain text and other crypt formats are refused.
func htpasswdHash(h string) bool {
	for _, p := range []string{"$2a$", "$2b$", "$2y$", "$apr1$"} {
		if strings.HasPrefix(h, p) {
			return true
		}
	}
	return false
}

// writeHtpasswd writes an nginx auth_basic user file (bcrypt or apr1 hashes).
func writeHtpasswd(path string, users []store.AccessListUser) error {
	if _, err := os.Stat(path); err == nil {
		return nil // shared by several hosts
	}
	var b strings.Builder
	for _, u := range users {
		b.WriteString(u.Username + ":" + u.PasswordHash + "\n")
	}
	return os.WriteFile(path, []byte(b.String()), 0o644)
}
