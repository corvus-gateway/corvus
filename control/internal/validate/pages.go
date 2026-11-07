package validate

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf8"

	"corvus/control/internal/store"
)

// MaxUnknownRedirectLen caps the unknown host redirect URL.
const MaxUnknownRedirectLen = 500

// UnknownHosts normalizes and validates the unknown host setting in place. The
// redirect URL is rendered unquoted into `return 302`, so it is restricted to
// http(s)://host[:port][/path] with the characters a redirection host target allows.
func UnknownHosts(u *store.UnknownHosts) error {
	u.Mode = strings.TrimSpace(u.Mode)
	if u.Mode == "" {
		u.Mode = store.UnknownNotFound
	}
	u.RedirectURL = strings.TrimSpace(u.RedirectURL)
	switch u.Mode {
	case store.UnknownNotFound, store.UnknownClose:
		u.RedirectURL = ""
		return nil
	case store.UnknownRedirect:
	default:
		return Errors{{Field: "mode", Code: "invalid", Message: "mode must be not_found, close or redirect"}}
	}
	bad := func(msg string) error { return Errors{{Field: "redirect_url", Code: "invalid", Message: msg}} }
	if u.RedirectURL == "" {
		return Errors{{Field: "redirect_url", Code: "required", Message: "a redirect URL is required"}}
	}
	scheme, rest, ok := strings.Cut(u.RedirectURL, "://")
	scheme = strings.ToLower(scheme)
	if !ok || (scheme != "http" && scheme != "https") {
		return bad("enter an absolute http:// or https:// URL")
	}
	host, path := rest, ""
	if i := strings.IndexAny(rest, ":/"); i >= 0 {
		host, path = rest[:i], rest[i:]
	}
	target := strings.ToLower(host) + path
	if len(scheme)+3+len(target) > MaxUnknownRedirectLen || !redirectTargetRe.MatchString(target) {
		return bad("invalid URL (http(s)://domain, optional :port and /path)")
	}
	if strings.HasPrefix(path, ":") {
		port, _, _ := strings.Cut(path[1:], "/")
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return bad("port must be 1-65535")
		}
	}
	u.RedirectURL = scheme + "://" + target
	return nil
}

// MaxCustomPageBytes caps an operator-supplied block or error page.
const MaxCustomPageBytes = 64 * 1024

// PagePlaceholders are replaced by the gateway when a block or error page is served
// (values are HTML-escaped). Unknown {{NAME}} placeholders render empty.
var PagePlaceholders = []string{"{{STATUS}}", "{{RID}}", "{{HOST}}", "{{IP}}", "{{TIME}}"}

// CustomPage checks an operator-supplied HTML page. The page is written verbatim to
// a file the gateway serves (it is never interpreted as nginx configuration), so
// only presence, size and encoding are restricted.
func CustomPage(field, html string) error {
	switch {
	case strings.TrimSpace(html) == "":
		return Errors{{Field: field, Code: "required", Message: "the page must not be empty"}}
	case len(html) > MaxCustomPageBytes:
		return Errors{{Field: field, Code: "too_large", Message: fmt.Sprintf("the page must be at most %d KB", MaxCustomPageBytes/1024)}}
	case !utf8.ValidString(html) || strings.ContainsRune(html, 0):
		return Errors{{Field: field, Code: "invalid", Message: "the page must be UTF-8 text"}}
	}
	return nil
}
