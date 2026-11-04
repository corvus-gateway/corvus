// Package validate checks user input before it is stored or rendered into
// nginx configuration. Everything rendered into config must pass through here:
// values are whitelisted so they cannot inject directives.
package validate

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"

	"corvus/control/internal/store"
)

// FieldError describes a single invalid field (field name + message key for i18n).
type FieldError struct {
	Field   string `json:"field"`
	Code    string `json:"code"`
	Message string `json:"message"`
}

type Errors []FieldError

func (e Errors) Error() string {
	parts := make([]string, len(e))
	for i, f := range e {
		parts[i] = f.Field + ": " + f.Message
	}
	return "invalid input: " + strings.Join(parts, "; ")
}

func (e *Errors) add(field, code, format string, a ...any) {
	*e = append(*e, FieldError{Field: field, Code: code, Message: fmt.Sprintf(format, a...)})
}

func (e Errors) OrNil() error {
	if len(e) == 0 {
		return nil
	}
	return e
}

var (
	labelRe    = `[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?`
	domainRe   = regexp.MustCompile(`^(\*\.)?(` + labelRe + `\.)*` + labelRe + `$`)
	upstreamRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9_.-]{0,251}[a-z0-9])?$`)
	pathRe     = regexp.MustCompile(`^/[A-Za-z0-9._~/-]{0,199}$`)
	roomRe     = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)
)

// Domain reports whether d is a valid (optionally wildcard) DNS name.
func Domain(d string) bool {
	return len(d) <= 253 && domainRe.MatchString(d)
}

// ProxyHost normalizes and validates a proxy host in place.
func ProxyHost(h *store.ProxyHost) error {
	var errs Errors
	switch h.BodyInspection {
	case "":
		h.BodyInspection = "log" // baseline: detect (log only) unless explicitly turned off
	case "off", "log", "block":
	default:
		errs.add("body_inspection", "invalid", "body inspection must be off, log or block")
	}
	switch h.SecurityHeaders {
	case "":
		h.SecurityHeaders = "off" // never changes responses unless explicitly chosen
	case "off", "basic", "strict":
	default:
		errs.add("security_headers", "invalid", "security headers must be off, basic or strict")
	}
	underAttack(&errs, h)
	if h.BodyMaxKB == 0 {
		h.BodyMaxKB = 1024
	}
	if h.BodyMaxKB < 16 || h.BodyMaxKB > 8192 {
		errs.add("body_max_kb", "invalid", "body inspection limit must be 16-8192 KiB")
	}
	h.Domains = hostDomains(&errs, h.Domains)

	h.ForwardScheme = strings.ToLower(strings.TrimSpace(h.ForwardScheme))
	if h.ForwardScheme == "" {
		h.ForwardScheme = "http"
	}
	if h.ForwardScheme != "http" && h.ForwardScheme != "https" {
		errs.add("forward_scheme", "invalid", "scheme must be http or https")
	}
	h.ForwardHost = strings.ToLower(strings.TrimSpace(h.ForwardHost))
	if ip := net.ParseIP(h.ForwardHost); ip != nil {
		if ip.To4() == nil {
			errs.add("forward_host", "ipv6_unsupported", "IPv6 upstreams are not supported yet")
		}
	} else if !upstreamRe.MatchString(h.ForwardHost) {
		errs.add("forward_host", "invalid", "invalid upstream host %q", h.ForwardHost)
	}
	if h.ForwardPort < 1 || h.ForwardPort > 65535 {
		errs.add("forward_port", "invalid", "port must be 1-65535")
	}

	if h.WaitroomPath == "" {
		h.WaitroomPath = "/"
	}
	if !pathRe.MatchString(h.WaitroomPath) || strings.Contains(h.WaitroomPath, "//") || strings.Contains(h.WaitroomPath, "/../") {
		errs.add("waitroom_path", "invalid", "path must start with / and contain only letters, digits and ._~/-")
	}
	if strings.HasPrefix(h.WaitroomPath, "/__corvus") {
		errs.add("waitroom_path", "reserved", "/__corvus is reserved")
	}
	if h.SSLForced && h.CertificateID == nil {
		errs.add("ssl_forced", "needs_certificate", "HTTPS redirect requires a certificate")
	}
	if h.ClientIP == "" {
		h.ClientIP = "direct"
	}
	if !ClientIPMode(h.ClientIP) {
		errs.add("client_ip", "invalid", "client_ip must be direct, cloudflare or trusted_proxies")
	}
	if err := AdvancedConfig(h.AdvancedConfig); err != nil {
		errs.add("advanced_config", "invalid", "%s", err.Error())
	}
	return errs.OrNil()
}

// hostDomains normalizes a host's server names (lower case, trimmed, deduplicated).
func hostDomains(errs *Errors, in []string) []string {
	seen := map[string]bool{}
	var domains []string
	for _, d := range in {
		d = strings.ToLower(strings.TrimSpace(d))
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		if !Domain(d) {
			errs.add("domains", "invalid_domain", "invalid domain name %q", d)
		}
		domains = append(domains, d)
	}
	if len(domains) == 0 {
		errs.add("domains", "required", "at least one domain is required")
	}
	if len(domains) > 50 {
		errs.add("domains", "too_many", "at most 50 domains per host")
	}
	return domains
}

// RedirectCodes are the HTTP status codes a redirection host may answer with.
var RedirectCodes = []int{301, 302, 303, 307, 308}

// redirectTargetRe: host (DNS name or IPv4), optional port, optional path/query. The
// target is rendered unquoted into `return`, so whitespace, quotes, ';', '{', '}', '$',
// '\' and '#' are never allowed.
var redirectTargetRe = regexp.MustCompile(`^(` + labelRe + `\.)*` + labelRe + `(:[0-9]{1,5})?(/[A-Za-z0-9._~/%+=&?,:@!*()-]*)?$`)

// RedirectionHost normalizes and validates a redirection host in place.
func RedirectionHost(h *store.RedirectionHost) error {
	var errs Errors
	h.Domains = hostDomains(&errs, h.Domains)
	h.ForwardScheme = strings.ToLower(strings.TrimSpace(h.ForwardScheme))
	if h.ForwardScheme == "" || h.ForwardScheme == "$scheme" {
		h.ForwardScheme = "auto"
	}
	if h.ForwardScheme != "auto" && h.ForwardScheme != "http" && h.ForwardScheme != "https" {
		errs.add("forward_scheme", "invalid", "scheme must be auto, http or https")
	}
	fd := strings.TrimSpace(h.ForwardDomain)
	host, rest := fd, ""
	if i := strings.IndexAny(fd, ":/"); i >= 0 {
		host, rest = fd[:i], fd[i:]
	}
	h.ForwardDomain = strings.ToLower(host) + rest
	switch {
	case h.ForwardDomain == "":
		errs.add("forward_domain", "required", "a target domain is required")
	case strings.Contains(h.ForwardDomain, "://"):
		errs.add("forward_domain", "invalid", "enter the domain without http:// or https:// (choose the scheme separately)")
	case len(h.ForwardDomain) > 500 || !redirectTargetRe.MatchString(h.ForwardDomain):
		errs.add("forward_domain", "invalid", "invalid target %q (domain, optional :port and /path)", h.ForwardDomain)
	case strings.HasPrefix(rest, ":"):
		port, _, _ := strings.Cut(rest[1:], "/")
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			errs.add("forward_domain", "invalid", "port must be 1-65535")
		}
	}
	if h.HTTPCode == 0 {
		h.HTTPCode = 301
	}
	okCode := false
	for _, c := range RedirectCodes {
		okCode = okCode || c == h.HTTPCode
	}
	if !okCode {
		errs.add("http_code", "invalid", "status code must be 301, 302, 303, 307 or 308")
	}
	if h.SSLForced && h.CertificateID == nil {
		errs.add("ssl_forced", "needs_certificate", "HTTPS redirect requires a certificate")
	}
	if err := AdvancedConfig(h.AdvancedConfig); err != nil {
		errs.add("advanced_config", "invalid", "%s", err.Error())
	}
	return errs.OrNil()
}

// deniedDirectives may not appear in operator snippets: they would run code in
// the gateway (Lua/JS/Perl), read secrets from the environment, or pull in
// arbitrary files (F10).
var deniedDirective = regexp.MustCompile(`^(?:.*_by_lua(?:_block|_file)?|lua_\S+|access_by_lua\S*|js_\S+|perl\S*|include|load_module|env|ssl_engine|daemon|master_process|user|pid|working_directory|worker_\S+|thread_pool|lock_file)$`)

// sensitivePath: root/alias may not expose the data volume or system directories.
var sensitivePath = regexp.MustCompile(`^/(?:$|data|etc|proc|sys|root|run|usr|opt|tmp|dev|bin|sbin|lib|lib64|home|var/(?:lib|run|log|cache|spool|tmp))(?:/|$)`)

// AdvancedConfig checks raw nginx snippets inserted into a server block. It
// tokenizes the way nginx does (quotes with escapes, "#" comments only at a
// token start, ${var} inside tokens) so braces cannot be hidden from the check,
// rejects dangerous directives, and keeps the snippet inside its server block.
// Final validation is `openresty -t`.
func AdvancedConfig(s string) error {
	if len(s) > 20000 {
		return fmt.Errorf("advanced config is limited to 20000 characters")
	}
	if strings.ContainsRune(s, 0) {
		return fmt.Errorf("invalid character")
	}
	depth := 0
	var stmt []string // tokens of the current statement
	var tok strings.Builder
	inTok := false
	flush := func() {
		if inTok {
			stmt = append(stmt, tok.String())
			tok.Reset()
			inTok = false
		}
	}
	check := func() error {
		if len(stmt) == 0 {
			return nil
		}
		name := strings.ToLower(stmt[0])
		if deniedDirective.MatchString(name) {
			return fmt.Errorf("directive %q is not allowed in advanced configuration", stmt[0])
		}
		if (name == "root" || name == "alias") && len(stmt) > 1 && sensitivePath.MatchString(strings.Trim(stmt[1], `"'`)) {
			return fmt.Errorf("%s may not point to %s", name, stmt[1])
		}
		return nil
	}
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\'':
			if inTok && tok.Len() > 0 { // quote inside a token is literal for nginx; treat as part of it
				tok.WriteByte(c)
				continue
			}
			flush()
			q := c
			var v strings.Builder
			j := i + 1
			for ; j < len(s); j++ {
				if s[j] == '\\' && j+1 < len(s) {
					v.WriteByte(s[j+1])
					j++
					continue
				}
				if s[j] == q {
					break
				}
				v.WriteByte(s[j])
			}
			if j >= len(s) {
				return fmt.Errorf("unterminated quote")
			}
			stmt = append(stmt, v.String())
			i = j
		case c == '#' && !inTok:
			for i < len(s) && s[i] != '\n' {
				i++
			}
		case c == '$' && i+1 < len(s) && s[i+1] == '{':
			end := strings.IndexByte(s[i:], '}')
			if end < 0 {
				return fmt.Errorf("unterminated ${...} variable")
			}
			tok.WriteString(s[i : i+end+1])
			inTok = true
			i += end
		case c == '\\' && i+1 < len(s):
			tok.WriteByte(s[i+1])
			inTok = true
			i++
		case c == ';':
			flush()
			if err := check(); err != nil {
				return err
			}
			stmt = stmt[:0]
		case c == '{':
			flush()
			if err := check(); err != nil {
				return err
			}
			stmt = stmt[:0]
			depth++
		case c == '}':
			flush()
			if len(stmt) > 0 {
				return fmt.Errorf("missing ';' before '}'")
			}
			depth--
			if depth < 0 {
				return fmt.Errorf("unbalanced '}' (would close the server block)")
			}
		case c == ' ' || c == '\t' || c == '\r' || c == '\n':
			flush()
		default:
			tok.WriteByte(c)
			inTok = true
		}
	}
	flush()
	if len(stmt) > 0 {
		return fmt.Errorf("missing ';' at the end")
	}
	if depth != 0 {
		return fmt.Errorf("unbalanced '{'")
	}
	return nil
}

// HasRootLocation reports whether the advanced config defines `location /`,
// in which case the generated default location is omitted (NPM-compatible).
var rootLocationRe = regexp.MustCompile(`(?m)^\s*location\s+/\s*\{`)

func HasRootLocation(s string) bool { return rootLocationRe.MatchString(s) }

// Waitroom normalizes and validates a access queue in place.
func Waitroom(w *store.Waitroom) error {
	var errs Errors
	w.Name = strings.ToLower(strings.TrimSpace(w.Name))
	if !roomRe.MatchString(w.Name) {
		errs.add("name", "invalid", "name must be 1-32 chars of a-z, 0-9 and '-' (starting with a letter or digit)")
	}
	rng := func(field string, v, lo, hi int) {
		if v < lo || v > hi {
			errs.add(field, "range", "must be between %d and %d", lo, hi)
		}
	}
	rng("capacity", w.Capacity, 1, 10_000_000)
	rng("active_ttl", w.ActiveTTL, 10, 86400)
	rng("grant_ttl", w.GrantTTL, 5, 3600)
	rng("queue_ttl", w.QueueTTL, 10, 86400)
	rng("refresh", w.Refresh, 1, 3600)
	if w.Refresh*2 > w.ActiveTTL {
		errs.add("refresh", "too_long", "refresh must be at most half of active_ttl")
	}
	rng("admit_rate", w.AdmitRate, 0, 100_000)
	rng("max_queue", w.MaxQueue, 0, 5_000_000)
	rng("max_new_per_ip", w.MaxNewIP, 0, 100_000)
	if w.FailMode != "open" && w.FailMode != "closed" {
		errs.add("fail_mode", "invalid", "fail_mode must be open or closed")
	}
	return errs.OrNil()
}

// ClientIPMode values accepted for proxy hosts.
func ClientIPMode(m string) bool { return m == "direct" || m == "cloudflare" || m == "trusted_proxies" }

// RealIPHeaders that may carry the client address from a trusted proxy.
var RealIPHeaders = []string{"X-Forwarded-For", "X-Real-IP", "CF-Connecting-IP", "True-Client-IP"}

func RealIPHeader(h string) bool {
	for _, v := range RealIPHeaders {
		if v == h {
			return true
		}
	}
	return false
}

var policyNameRe = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9 _.-]{0,63}$`)

// RateLimit validates a rate limit policy in place.
func RateLimit(r *store.RateLimit) error {
	var errs Errors
	r.Name = strings.TrimSpace(r.Name)
	if !policyNameRe.MatchString(r.Name) {
		errs.add("name", "invalid", "name must be 1-64 characters (letters, digits, space, _ . -)")
	}
	if r.RequestsPerSecond < 1 || r.RequestsPerSecond > 100000 {
		errs.add("requests_per_second", "range", "must be between 1 and 100000")
	}
	if r.Burst < 0 || r.Burst > 100000 {
		errs.add("burst", "range", "must be between 0 and 100000")
	}
	if r.MaxConnections < 0 || r.MaxConnections > 100000 {
		errs.add("max_connections", "range", "must be between 0 and 100000")
	}
	if r.StatusCode == 0 {
		r.StatusCode = 429
	}
	if r.StatusCode != 429 && r.StatusCode != 503 && r.StatusCode != 403 {
		errs.add("status_code", "invalid", "status code must be 429, 503 or 403")
	}
	return errs.OrNil()
}

var usernameRe = regexp.MustCompile(`^[A-Za-z0-9._@-]{1,64}$`)

// AccessList validates an access list in place (rules must already be normalized CIDRs or "all").
func AccessList(a *store.AccessList) error {
	var errs Errors
	a.Name = strings.TrimSpace(a.Name)
	if !policyNameRe.MatchString(a.Name) {
		errs.add("name", "invalid", "name must be 1-64 characters (letters, digits, space, _ . -)")
	}
	// Basic-auth only: IP rules moved to security rules (allow/deny are always empty).
	a.Satisfy, a.Allow, a.Deny = "all", []string{}, []string{}
	seen := map[string]bool{}
	for _, u := range a.Users {
		if !usernameRe.MatchString(u.Username) {
			errs.add("users", "invalid", "invalid username %q", u.Username)
		}
		if seen[u.Username] {
			errs.add("users", "duplicate", "duplicate username %q", u.Username)
		}
		seen[u.Username] = true
	}
	if len(a.Users) == 0 {
		errs.add("users", "empty", "add at least one user")
	}
	return errs.OrNil()
}
