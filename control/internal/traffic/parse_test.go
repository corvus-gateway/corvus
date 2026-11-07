package traffic

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"
	"testing"
)

// nginxEscape mimics nginx's escape=json (ngx_escape_json).
func nginxEscape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		c := s[i]
		switch {
		case c == '"' || c == '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case c == '\n':
			b.WriteString(`\n`)
		case c == '\r':
			b.WriteString(`\r`)
		case c == '\t':
			b.WriteString(`\t`)
		case c == '\b':
			b.WriteString(`\b`)
		case c == '\f':
			b.WriteString(`\f`)
		case c < 0x20:
			fmt.Fprintf(&b, `\u%04x`, c)
		default:
			b.WriteByte(c)
		}
	}
	return b.String()
}

// tsvLine renders a record as nginx would with log_format corvus_log.
func tsvLine(r *Record) string {
	f := []string{"c1", strconv.FormatFloat(r.T, 'f', 3, 64), r.H, r.M, strconv.Itoa(r.S), strconv.FormatInt(r.B, 10),
		strconv.FormatFloat(r.RT, 'f', 3, 64), r.IP, r.WR, r.X, r.RL, r.R, r.CC, r.BD, r.BN, r.U, r.UA}
	for i := 2; i < len(f); i++ {
		if i != 4 && i != 5 && i != 6 {
			f[i] = nginxEscape(f[i])
		}
	}
	return strings.Join(f, "\t")
}

func TestParseDatagram(t *testing.T) {
	want := Record{T: 1762243516.123, H: "shop.example.com", M: "GET", U: "/a\"b\\c\td", S: 403, B: 1234, RT: 0.012,
		IP: "203.0.113.9", WR: "-", X: "sqli", RL: "REJECTED_DRY_RUN", R: "7:block", CC: "KR", BD: "sig:log", BN: "1",
		UA: "Mozilla/5.0 \x01 ünïcode"}
	line := tsvLine(&want)
	json := `{"t":1762243516.123,"h":"shop.example.com","m":"GET","u":"/a\"b\\c\td","s":403,"b":1234,"rt":0.012,"ip":"203.0.113.9","wr":"-","x":"sqli","rl":"REJECTED_DRY_RUN","r":"7:block","cc":"KR","bd":"sig:log","bn":"1","ua":"Mozilla/5.0 \u0001 ünïcode"}`
	for name, msg := range map[string]string{
		"syslog":      "<190>Nov  4 08:05:16 corvus: " + line,
		"syslog-json": "<190>Nov  4 08:05:16 corvus: " + json,
		"bare":        line,
		"batch":       line + "\n" + line + "\n",
	} {
		var got []Record
		bad := parseDatagram([]byte(msg), func(r *Record) { got = append(got, *r) })
		n := 1
		if name == "batch" {
			n = 2
		}
		if bad != 0 || len(got) != n {
			t.Fatalf("%s: %d records, %d bad", name, len(got), bad)
		}
		for _, g := range got {
			if g != want {
				t.Errorf("%s:\n got %+v\nwant %+v", name, g, want)
			}
		}
	}
	// empty numeric fields ($body_bytes_sent, $request_time can be empty for some requests)
	var r Record
	if !parseRecord([]byte("c1\t1.5\th\tGET\t400\t\t\t1.2.3.4\t-\t\t\t\t\t\t\t/\t"), &r) || r.S != 400 || r.B != 0 || r.IP != "1.2.3.4" || r.U != "/" {
		t.Errorf("empty fields: %+v", r)
	}
	for _, bad := range []string{"garbage", "c1\t1\th", "c1\tx\th\tGET\t/\t200\t0\t0\tip\t\t\t\t\t\t\t\t\tua", "<190>corvus: {not json"} {
		if parseDatagram([]byte(bad), func(*Record) { t.Errorf("parsed %q", bad) }) != 1 {
			t.Errorf("%q not counted as bad", bad)
		}
	}
}

// Records as corvus/accesslog.lua writes them (the same lines as in
// gateway/test/accesslog_test.lua; "|" stands for a tab).
func TestParseLuaRecord(t *testing.T) {
	msg := strings.ReplaceAll(`c1|1762243516.123|shop.example.com|GET|403|1234|0.012|203.0.113.9|queue|`+
		`sqli|REJECTED_DRY_RUN|7:block|KR|sig:log|1|/a\"b\\c\td|Mozilla/5.0 \u0001 ünïcode`+"\n"+
		`c1|1762243516.123|h|GET|200|0|0|1.1.1.1|-|||||||/|`+"\n", "|", "\t")
	want := []Record{
		{T: 1762243516.123, H: "shop.example.com", M: "GET", U: "/a\"b\\c\td", S: 403, B: 1234, RT: 0.012, IP: "203.0.113.9",
			WR: "queue", X: "sqli", RL: "REJECTED_DRY_RUN", R: "7:block", CC: "KR", BD: "sig:log", BN: "1", UA: "Mozilla/5.0 \x01 ünïcode"},
		{T: 1762243516.123, H: "h", M: "GET", U: "/", S: 200, IP: "1.1.1.1", WR: "-"},
	}
	var got []Record
	if bad := parseDatagram([]byte(msg), func(r *Record) { got = append(got, *r) }); bad != 0 || len(got) != 2 {
		t.Fatalf("%d records, %d bad", len(got), bad)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("record %d:\n got %+v\nwant %+v", i, got[i], want[i])
		}
	}
}

func FuzzParseTSV(f *testing.F) {
	f.Add("shop.example.com", "/path?x", "Mozilla/5.0", "7:block", 200)
	f.Add("h", "\"\\\t\n\x00\x7f\xff", "\xc3\x28", "", 999)
	f.Fuzz(func(t *testing.T, host, uri, ua, rule string, status int) {
		if status < 0 {
			status = -status
		}
		in := Record{T: 1.5, H: host, M: "GET", U: uri, S: status, B: 7, RT: 0.25, IP: "1.2.3.4", WR: "-", R: rule, UA: ua}
		var out Record
		if !parseRecord([]byte(tsvLine(&in)), &out) {
			t.Fatalf("not parsed: %q", tsvLine(&in))
		}
		valid := func(s string) string { return string(bytes.ToValidUTF8([]byte(s), []byte("\uFFFD"))) }
		in.H, in.U, in.UA, in.R = valid(in.H), valid(in.U), valid(in.UA), valid(in.R)
		if out != in {
			t.Fatalf("round trip:\n got %+v\nwant %+v", out, in)
		}
	})
}

var benchLine = []byte("<190>Nov  4 08:05:16 corvus: c1\t1762243516.123\tbench-plain.test\tGET\t200\t3\t0.001\t127.0.0.1\t-\t\t\t\t\t\t\t/\twrk/4.2.0 (Linux) some longer user agent string Mozilla/5.0")
var benchJSON = []byte(`<190>Nov  4 08:05:16 corvus: {"t":1762243516.123,"h":"bench-plain.test","m":"GET","u":"/","s":200,"b":3,"rt":0.001,"ip":"127.0.0.1","wr":"-","x":"","rl":"","r":"","cc":"","bd":"","bn":"","ua":"wrk/4.2.0 (Linux) some longer user agent string Mozilla/5.0"}`)

func BenchmarkParse(b *testing.B) {
	for name, msg := range map[string][]byte{"tsv": benchLine, "json": benchJSON} {
		b.Run(name, func(b *testing.B) {
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				parseDatagram(msg, func(*Record) {})
			}
		})
	}
}

func BenchmarkIngest(b *testing.B) {
	c := New()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		c.Ingest(benchLine)
	}
}
