package traffic

import (
	"net/url"
	"strconv"
	"time"
)

// AppendCombined appends l as one line (with "\n") in nginx's standard "combined" log
// format, which log analysers such as GoAccess read:
//
//	ip - - [02/Jan/2006:15:04:05 +0000] "METHOD /path?utm_source=… HTTP/1.1" status bytes "referrer" "user-agent"
//
// The request log does not keep the query string or the protocol: the query is rebuilt
// from the recorded UTM values only (omitted when there are none) and the protocol is
// always HTTP/1.1. The referrer is the recorded origin. Missing values are "-"; quotes,
// backslashes and control characters are escaped as nginx does (\" \\ \xHH).
func AppendCombined(b []byte, l *LogLine) []byte {
	ip := l.IP
	if ip == "" {
		ip = "-"
	}
	b = appendEsc(b, ip)
	b = append(b, " - - ["...)
	ts := TimeOf(l.TS)
	b = ts.UTC().AppendFormat(b, "02/Jan/2006:15:04:05 -0700")
	b = append(b, `] "`...)
	m := l.Method
	if m == "" {
		m = "-"
	}
	b = appendEsc(b, m)
	b = append(b, ' ')
	p := l.Path
	if p == "" {
		p = "/"
	}
	b = append(b, (&url.URL{Path: p}).EscapedPath()...)
	sep := byte('?')
	for _, kv := range [...][2]string{{"utm_source", l.UTMSrc}, {"utm_medium", l.UTMMed}, {"utm_campaign", l.UTMCmp}} {
		if kv[1] != "" {
			b = append(b, sep)
			b = append(b, kv[0]...)
			b = append(b, '=')
			b = append(b, url.QueryEscape(kv[1])...)
			sep = '&'
		}
	}
	b = append(b, ` HTTP/1.1" `...)
	b = strconv.AppendInt(b, int64(l.Status), 10)
	b = append(b, ' ')
	b = strconv.AppendInt(b, l.Bytes, 10)
	b = append(b, ` "`...)
	b = appendEscOr(b, l.Ref)
	b = append(b, `" "`...)
	b = appendEscOr(b, l.UA)
	return append(b, "\"\n"...)
}

// TimeOf converts a log timestamp (Unix seconds with milliseconds) to a time.
func TimeOf(ts float64) time.Time { return time.UnixMilli(int64(ts * 1000)) }

func appendEscOr(b []byte, s string) []byte {
	if s == "" {
		return append(b, '-')
	}
	return appendEsc(b, s)
}

// appendEsc escapes like nginx's default log escaping: " and \ with a backslash, control
// characters as \xHH. Other bytes (including UTF-8) are kept.
func appendEsc(b []byte, s string) []byte {
	const hex = "0123456789ABCDEF"
	for i := 0; i < len(s); i++ {
		switch c := s[i]; {
		case c == '"' || c == '\\':
			b = append(b, '\\', c)
		case c < 0x20 || c == 0x7f:
			b = append(b, '\\', 'x', hex[c>>4], hex[c&0xf])
		default:
			b = append(b, c)
		}
	}
	return b
}
