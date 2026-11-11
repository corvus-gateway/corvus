// Package hints classifies stored error messages into stable codes that the console
// and alerts turn into one actionable sentence and a link to the troubleshooting guide.
//
// Classification runs on the stored (already redacted) text when it is read, so no
// database change is needed and older errors get hints too. Every function returns ""
// when the message is not recognised; the raw message always stays available.
package hints

import (
	"regexp"
	"strings"
)

// ACME hint codes.
const (
	ACMERateLimited     = "acme.rate_limited"
	ACMEDNSServfail     = "acme.dns_servfail"
	ACMEDNSPropagation  = "acme.dns_propagation"
	ACMEDNSCredential   = "acme.dns_credential"
	ACMEDNSLookup       = "acme.dns_lookup"
	ACMECAA             = "acme.caa"
	ACMERejectedName    = "acme.rejected_name"
	ACMEInvalidEmail    = "acme.invalid_email"
	ACMEDomainDNS       = "acme.domain_dns"
	ACMEPort80          = "acme.port80_unreachable"
	ACMEWrongServer     = "acme.wrong_server"
	ACMECAUnreachable   = "acme.ca_unreachable"
	DeployPortInUse     = "deploy.port_in_use"
	DeployAdvanced      = "deploy.advanced_config"
	NetDNS              = "net.dns"
	NetRefused          = "net.refused"
	NetTimeout          = "net.timeout"
	NetTLS              = "net.tls"
	NetBlocked          = "net.blocked"
	NetAuth             = "net.auth"
	NetNotFound         = "net.not_found"
	NetMySQLAuth        = "net.mysql_auth"
	NetMySQLDB          = "net.mysql_db"
	NetOIDCIssuer       = "net.oidc_issuer"
	acmeProblem         = "urn:ietf:params:acme:error:"
	propagationTimedOut = "propagation: time limit exceeded"
)

var (
	// lego dns01 precheck: "NS ns1.example.net. returned SERVFAIL for _acme-challenge.example.com."
	nsFailure = regexp.MustCompile(`returned (servfail|refused) for`)
	// DNS provider API status codes as printed by lego providers ("status code: 403", "status code 401")
	apiStatusAuth = regexp.MustCompile(`status code:? ?40[13]\b`)
	apiAuthWords  = regexp.MustCompile(`unauthori[sz]ed|authenticat|forbidden|access denied|invalid.*(token|key)`)
	// nginx -t: "[emerg] unknown directive "foo" in /data/.../hosts/3.conf:25"
	advancedConf = regexp.MustCompile(`\[emerg\].*(unknown directive|not allowed here|invalid number of arguments|unexpected|is duplicate).*/(hosts|streams)/(redirect-)?\d+\.conf`)
	httpAuth     = regexp.MustCompile(`\bhttp 40[13]\b|\b401 unauthorized\b|\b403 forbidden\b`)
	httpNotFound = regexp.MustCompile(`\bhttp 404\b|\b404 not found\b`)
)

func has(s string, subs ...string) bool {
	for _, x := range subs {
		if strings.Contains(s, x) {
			return true
		}
	}
	return false
}

// ACME classifies a certificate issuance or renewal error (lego / Let's Encrypt).
func ACME(msg string) string {
	m := strings.ToLower(msg)
	if m == "" {
		return ""
	}
	switch {
	case has(m, acmeProblem+"ratelimited"):
		return ACMERateLimited
	case has(m, propagationTimedOut) && nsFailure.MatchString(m):
		return ACMEDNSServfail
	case has(m, propagationTimedOut),
		has(m, acmeProblem+"unauthorized") && has(m, "_acme-challenge"):
		return ACMEDNSPropagation
	case has(m, "error presenting token") &&
		(has(m, "zone could not be found") || apiStatusAuth.MatchString(m) || apiAuthWords.MatchString(m)):
		return ACMEDNSCredential
	case has(m, "could not find the start of authority", "initial recursive nameserver:", "ns call failed", "could not determine authoritative nameservers"):
		return ACMEDNSLookup
	case has(m, acmeProblem+"caa"):
		return ACMECAA
	case has(m, acmeProblem+"rejectedidentifier"):
		return ACMERejectedName
	case has(m, acmeProblem+"invalidcontact", acmeProblem+"unsupportedcontact"):
		return ACMEInvalidEmail
	case has(m, acmeProblem+"dns"):
		return ACMEDomainDNS
	case has(m, acmeProblem+"connection"):
		return ACMEPort80
	case has(m, acmeProblem+"unauthorized"):
		return ACMEWrongServer
	case has(m, "get directory at"):
		return ACMECAUnreachable
	}
	return ""
}

// Deploy classifies the output of a rejected gateway configuration test (nginx -t).
func Deploy(output string) string {
	m := strings.ToLower(output)
	switch {
	case has(m, "address already in use"):
		return DeployPortInUse
	case advancedConf.MatchString(m):
		return DeployAdvanced
	}
	return ""
}

// Reload classifies a reload the gateway did not take (the configuration test passed):
// a port conflict only when the message names one (nginx: "bind() to … failed (98:
// Address already in use)").
func Reload(msg string) string {
	if m := strings.ToLower(msg); has(m, "address already in use", "bind()") {
		return DeployPortInUse
	}
	return ""
}

// Net classifies an outbound request error (list feeds, country database, signature
// updates, alert channels, Cloudflare ranges, NPM MySQL import, SSO discovery).
func Net(msg string) string {
	m := strings.ToLower(msg)
	switch {
	case m == "":
		return ""
	case has(m, "did not match the issuer url returned by provider"):
		return NetOIDCIssuer
	case has(m, "error 1045"):
		return NetMySQLAuth
	case has(m, "error 1049"):
		return NetMySQLDB
	case has(m, "is not allowed", "non-public address"):
		return NetBlocked
	case has(m, "no such host", "server misbehaving"):
		return NetDNS
	case has(m, "connection refused"):
		return NetRefused
	case has(m, "x509:"):
		return NetTLS
	case has(m, "i/o timeout", "client.timeout exceeded", "context deadline exceeded"):
		return NetTimeout
	case httpAuth.MatchString(m):
		return NetAuth
	case httpNotFound.MatchString(m):
		return NetNotFound
	}
	return ""
}
