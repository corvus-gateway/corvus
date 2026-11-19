package traffic

import (
	"net/url"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Traffic sources are recorded without secrets: URLs often carry reset tokens, OAuth
// codes or API keys in their path or query, so of the Referer only the origin is kept
// and of the request's query string only the three UTM values. The gateway already
// reduces them (map $corvus_ref in nginx.conf, corvus/accesslog.lua); these functions
// canonicalise and bound whatever arrives, so a full Referer or an oversized value
// never reaches the request log even from an older gateway configuration.

const (
	maxOrigin = 200 // bytes of a referrer origin
	maxUTM    = 100 // bytes of a decoded UTM value
)

// ReferrerOrigin reduces a Referer (or an origin) to "scheme://host[:port]": lowercased,
// without userinfo, path, query or fragment. It returns "" when the value is absent,
// not an absolute URL, contains unexpected characters or is longer than 200 bytes.
func ReferrerOrigin(s string) string {
	i := strings.Index(s, "://")
	if i < 1 || i > 32 {
		return ""
	}
	scheme, rest := s[:i], s[i+3:]
	for j := 0; j < len(scheme); j++ {
		c := scheme[j]
		if !(isAlpha(c) || j > 0 && (c >= '0' && c <= '9' || c == '+' || c == '-' || c == '.')) {
			return ""
		}
	}
	if j := strings.IndexAny(rest, "/?#"); j >= 0 {
		rest = rest[:j]
	}
	if j := strings.LastIndexByte(rest, '@'); j >= 0 {
		rest = rest[j+1:]
	}
	if !validHostPort(rest) || len(scheme)+3+len(rest) > maxOrigin {
		return ""
	}
	return strings.ToLower(scheme + "://" + rest)
}

// validHostPort reports whether s is a host name ([A-Za-z0-9._-]+) or a bracketed IPv6
// address ([0-9a-fA-F:.]+), optionally followed by ":" and a port of 1-5 digits. The
// same pattern as map $corvus_ref in nginx.conf.tmpl: a malformed authority such as
// "user:pa" (from "https://user:pa?ss@host/", whose "?" ends the authority) yields "",
// never a part of the credentials.
func validHostPort(s string) bool {
	i := 0
	if strings.HasPrefix(s, "[") {
		j := strings.IndexByte(s, ']')
		if j < 2 {
			return false
		}
		for k := 1; k < j; k++ {
			if c := s[k]; !(isHex(c) || c == ':' || c == '.') {
				return false
			}
		}
		i = j + 1
	} else {
		for i < len(s) && (isAlpha(s[i]) || s[i] >= '0' && s[i] <= '9' || s[i] == '.' || s[i] == '_' || s[i] == '-') {
			i++
		}
		if i == 0 {
			return false
		}
	}
	if i == len(s) {
		return true
	}
	port := s[i:]
	if port[0] != ':' || len(port) < 2 || len(port) > 6 {
		return false
	}
	for k := 1; k < len(port); k++ {
		if port[k] < '0' || port[k] > '9' {
			return false
		}
	}
	return true
}

func isHex(c byte) bool { return c >= '0' && c <= '9' || c >= 'a' && c <= 'f' || c >= 'A' && c <= 'F' }

func isAlpha(c byte) bool { return c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// UTMValue cleans a raw utm_* query value: URL-decoded (kept raw when it is not valid
// URL encoding), control and invisible characters removed (Cc; Cf such as U+202E
// RIGHT-TO-LEFT OVERRIDE or U+200B ZERO WIDTH SPACE; the separators U+2028/U+2029),
// surrounding spaces trimmed, at most 100 bytes (cut at a character boundary).
func UTMValue(s string) string {
	if s == "" {
		return ""
	}
	if len(s) > 3*maxUTM {
		s = s[:3*maxUTM]
		// do not leave half an escape sequence at the end
		if n := len(s); s[n-1] == '%' {
			s = s[:n-1]
		} else if s[n-2] == '%' {
			s = s[:n-2]
		}
	}
	if plain(s) {
		return s // the common case: short, printable ASCII, nothing to decode
	}
	if d, err := url.QueryUnescape(s); err == nil {
		s = d
	}
	s = strings.ToValidUTF8(s, "�")
	s = strings.Map(func(r rune) rune {
		if unicode.In(r, unicode.Cc, unicode.Cf, unicode.Zl, unicode.Zp) {
			return -1
		}
		return r
	}, s)
	s = strings.TrimSpace(s)
	if len(s) > maxUTM {
		n := maxUTM
		for n > 0 && !utf8.RuneStart(s[n]) {
			n--
		}
		s = strings.TrimSpace(s[:n])
	}
	return s
}

func plain(s string) bool {
	if len(s) > maxUTM || s[0] == ' ' || s[len(s)-1] == ' ' {
		return false
	}
	for i := 0; i < len(s); i++ {
		if c := s[i]; c < 0x20 || c > 0x7e || c == '%' || c == '+' {
			return false
		}
	}
	return true
}
