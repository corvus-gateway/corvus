package traffic

import (
	"bytes"
	"encoding/json"
	"strconv"
	"unicode/utf8"
)

// The gateway's access log format (log_format corvus_log in nginx.conf.tmpl): one line of
// tab-separated fields after the version tag "c1", every value escaped by nginx with
// escape=json (so a value never contains a raw tab or newline). It is parsed without
// reflection; the older JSON format (log_format corvus_json) is still accepted, e.g. from
// a gateway that still runs the previous configuration during an upgrade.
//
//	c1 \t $msec \t $corvus_host \t $request_method \t $status \t $body_bytes_sent \t
//	$request_time \t $remote_addr \t $corvus_wr \t $corvus_exploit \t $limit_req_status \t
//	$corvus_rule \t $corvus_cc \t $corvus_body \t $corvus_ban \t $uri \t $http_user_agent
//
// Proxy hosts send the same records from Lua (gateway/lua/corvus/accesslog.lua), several
// per datagram.
const logTag = "c1\t"

const logFields = 17 // including the tag

var syslogSep = []byte(": ")

// parseDatagram calls fn for every record in a datagram: an nginx syslog message
// ("<pri>timestamp tag: <record>") or bare records separated by newlines. It returns
// the number of records that could not be parsed.
func parseDatagram(msg []byte, fn func(*Record)) (bad int) {
	if len(msg) > 0 && msg[0] == '<' {
		// syslog header: the record follows the first ": " (the tag has no spaces)
		if i := bytes.Index(msg, syslogSep); i >= 0 {
			msg = msg[i+2:]
		}
	}
	for len(msg) > 0 {
		line := msg
		if i := bytes.IndexByte(msg, '\n'); i >= 0 {
			line, msg = msg[:i], msg[i+1:]
		} else {
			msg = nil
		}
		if len(line) == 0 {
			continue
		}
		var r Record
		if !parseRecord(line, &r) {
			bad++
			continue
		}
		fn(&r)
	}
	return bad
}

// parseRecord parses one record in the tab-separated format, or JSON (older format).
func parseRecord(line []byte, r *Record) bool {
	if bytes.HasPrefix(line, []byte(logTag)) {
		return parseTSV(line[len(logTag):], r)
	}
	if i := bytes.IndexByte(line, '{'); i >= 0 {
		return json.Unmarshal(line[i:], r) == nil
	}
	return false
}

func parseTSV(b []byte, r *Record) bool {
	var f [logFields - 1][]byte
	n := 0
	for n < len(f)-1 {
		i := bytes.IndexByte(b, '\t')
		if i < 0 {
			return false
		}
		f[n], b = b[:i], b[i+1:]
		n++
	}
	f[n] = bytes.TrimRight(b, "\r") // the User-Agent is last; syslog adds nothing after it
	var err error
	if r.T, err = parseFloat(f[0]); err != nil {
		return false
	}
	r.H = unescape(f[1])
	r.M = unescape(f[2])
	s, err := strconv.Atoi(string(f[3]))
	if err != nil {
		return false
	}
	r.S = s
	if len(f[4]) > 0 {
		if r.B, err = strconv.ParseInt(string(f[4]), 10, 64); err != nil {
			return false
		}
	}
	if r.RT, err = parseFloat(f[5]); err != nil {
		return false
	}
	r.IP = unescape(f[6])
	r.WR = unescape(f[7])
	r.X = unescape(f[8])
	r.RL = unescape(f[9])
	r.R = unescape(f[10])
	r.CC = unescape(f[11])
	r.BD = unescape(f[12])
	r.BN = unescape(f[13])
	r.U = unescape(f[14])
	r.UA = unescape(f[15])
	return true
}

func parseFloat(b []byte) (float64, error) {
	if len(b) == 0 {
		return 0, nil
	}
	return strconv.ParseFloat(string(b), 64)
}

// unescape reverses nginx's escape=json: \" \\ \n \r \t \b \f and \u00XX. Other bytes
// are copied; invalid UTF-8 is replaced (as encoding/json does).
func unescape(b []byte) string {
	if len(b) == 0 {
		return ""
	}
	if bytes.IndexByte(b, '\\') < 0 {
		if utf8.Valid(b) {
			return string(b)
		}
		return string(bytes.ToValidUTF8(b, []byte("�")))
	}
	out := make([]byte, 0, len(b))
	for i := 0; i < len(b); i++ {
		c := b[i]
		if c != '\\' || i+1 >= len(b) {
			out = append(out, c)
			continue
		}
		i++
		switch b[i] {
		case 'n':
			out = append(out, '\n')
		case 'r':
			out = append(out, '\r')
		case 't':
			out = append(out, '\t')
		case 'b':
			out = append(out, '\b')
		case 'f':
			out = append(out, '\f')
		case 'u':
			if i+4 < len(b) {
				if v, err := strconv.ParseUint(string(b[i+1:i+5]), 16, 16); err == nil {
					out = utf8.AppendRune(out, rune(v))
					i += 4
					continue
				}
			}
			out = append(out, '\\', 'u')
		default: // \" \\ \/ and anything unexpected: the character itself
			out = append(out, b[i])
		}
	}
	if !utf8.Valid(out) {
		out = bytes.ToValidUTF8(out, []byte("�"))
	}
	return string(out)
}
