package traffic

import (
	"strings"
	"testing"
)

func TestReferrerOrigin(t *testing.T) {
	for in, want := range map[string]string{
		"":                                       "",
		"https://www.google.com/search?q=secret": "https://www.google.com",
		"https://www.google.com":                 "https://www.google.com",
		"HTTPS://WWW.Example.COM:8443/a#frag":    "https://www.example.com:8443",
		"http://user:pass@shop.example.com/reset": "http://shop.example.com",
		"https://a@b@evil.example/?token=abc":     "https://evil.example",
		"http://[2001:db8::1]:8080/x":             "http://[2001:db8::1]:8080",
		"android-app://com.google.android.gm/":    "android-app://com.google.android.gm",
		"https://example.com?code=oauth-secret":   "https://example.com",
		"https://example.com#access_token=secret": "https://example.com",
		"/relative/path":                          "",
		// the "?" ends the authority: "bob:pa" is no host with a port, so nothing of
		// the credentials is logged
		"https://bob:pa?ss@evil.example/":           "",
		"https://bob:pa#ss@evil.example/":           "",
		"https://bob:pa@evil.example:443/":          "https://evil.example:443",
		"https://evil.example:/":                    "",
		"https://evil.example:123456/":              "",
		"https://evil.example:80:80/":               "",
		"https://[2001:db8::1/":                     "",
		"https://[]/":                               "",
		"https://[2001:db8::1]x/":                   "",
		"https://[2001:db8::g]/":                    "",
		"https://[::1]/":                            "https://[::1]",
		"https://ex[a]mple.com/":                    "",
		"://nohost":                                 "",
		"https://":                                  "",
		"https:///path":                             "",
		"javascript:alert(1)":                       "",
		"1http://example.com/":                      "",
		"ht tp://example.com/":                      "",
		"https://exa mple.com/":                     "",
		"https://exa\"mple.com/":                    "",
		"https://ex%41mple.com/":                    "",
		"https://" + strings.Repeat("a", 193) + "/": "",
		"https://" + strings.Repeat("a", 192) + "/": "https://" + strings.Repeat("a", 192),
	} {
		if got := ReferrerOrigin(in); got != want {
			t.Errorf("ReferrerOrigin(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestUTMValue(t *testing.T) {
	for in, want := range map[string]string{
		"":                   "",
		"ig":                 "ig",
		"spring+sale":        "spring sale",
		"spring%20sale%21":   "spring sale!",
		"%ED%95%9C%EA%B8%80": "한글",
		"bad%zzescape":       "bad%zzescape",
		"a%0Ab%09c%00d":      "abcd",
		"  padded%20 ":       "padded",
		"x\x01y\x7f":         "xy",
		// format and invisible characters (Cf, Zl, Zp) cannot reorder or hide text
		"%E2%80%AEcod.exe":                "cod.exe", // U+202E RIGHT-TO-LEFT OVERRIDE
		"ev%E2%80%8Bil":                   "evil",    // U+200B ZERO WIDTH SPACE
		"%EF%BB%BFbom%E2%81%A6x%E2%81%A9": "bomx",    // U+FEFF, U+2066, U+2069
		"a%E2%80%A8b%E2%80%A9c":           "abc",     // U+2028, U+2029
		"‮raw​":                           "raw",     // not URL-encoded
		"caf%C3%A9%E3%80%80bar":           "café　bar",
		"\xff\xfe":                        "�", // one replacement character per invalid run
		strings.Repeat("a", 150):          strings.Repeat("a", 100),
		strings.Repeat("%ED%95%9C", 50):   strings.Repeat("한", 33), // cut at a character boundary
		strings.Repeat("b", 299) + "%41":  strings.Repeat("b", 100),
	} {
		if got := UTMValue(in); got != want {
			t.Errorf("UTMValue(%q) = %q, want %q", in, got, want)
		}
	}
}

// The source fields are parsed when present, records without them (an older gateway
// configuration) still parse, and Add reduces and bounds whatever arrives.
func TestParseSourceFields(t *testing.T) {
	base := "c1\t1762243516.123\th.example\tGET\t200\t5\t0.001\t203.0.113.9\t-\t\t\t\t\t\t\t/p\tUA"
	var r Record
	if !parseRecord([]byte(base+"\thttps://www.google.com\tig\t\tfall%20sale\r"), &r) {
		t.Fatal("not parsed")
	}
	if r.UA != "UA" || r.RF != "https://www.google.com" || r.US != "ig" || r.UM != "" || r.UC != "fall%20sale" {
		t.Fatalf("source fields: %+v", r)
	}
	r = Record{}
	if !parseRecord([]byte(base+"\r"), &r) || r.UA != "UA" || r.RF != "" {
		t.Fatalf("record without source fields: %+v", r)
	}
	r = Record{}
	if !parseRecord([]byte(base+"\tref\tus"), &r) || r.RF != "ref" || r.US != "us" || r.UM != "" || r.UC != "" {
		t.Fatalf("record with some source fields: %+v", r)
	}
	// escaped as nginx does with escape=json
	r = Record{}
	if !parseRecord([]byte(base+"\t\tcamp\\\"x\\t\t\t"), &r) || r.US != "camp\"x\t" {
		t.Fatalf("escaped UTM: %+v", r)
	}

	c := New()
	var sunk []Record
	c.Sink = func(r *Record, _ bool) { sunk = append(sunk, *r) }
	c.Add(&Record{T: 1762243516, H: "h.example", M: "GET", U: "/p", S: 200, IP: "203.0.113.9",
		RF: "https://WWW.Google.com/search?q=secret", US: "ig", UM: "social%0A", UC: "fall"})
	c.Add(&Record{T: 1762243517, H: "h.example", M: "GET", U: "/q", S: 200, IP: "203.0.113.8"})
	if len(sunk) != 2 || sunk[0].RF != "https://www.google.com" || sunk[0].UM != "social" || sunk[1].RF != "" {
		t.Fatalf("normalised record: %+v", sunk)
	}
	logs := c.Logs(LogQuery{})
	if len(logs) != 2 || logs[1].Ref != "https://www.google.com" || logs[1].UTMSrc != "ig" || logs[1].UTMMed != "social" || logs[1].UTMCmp != "fall" {
		t.Fatalf("log lines: %+v", logs)
	}
	for q, n := range map[string]int{"google": 1, "fall": 1, "social": 1, "ig": 1, "/q": 1, "203.0.113": 2, "secret": 0} {
		if got := len(c.Logs(LogQuery{Q: q})); got != n {
			t.Errorf("search %q: %d lines, want %d", q, got, n)
		}
	}
}

func TestAppendCombined(t *testing.T) {
	l := LogLine{TS: 1762243516.123, Method: "GET", Path: "/a b/\"c\"", Status: 200, Bytes: 512, IP: "203.0.113.9",
		Ref: "https://www.google.com", UA: `Mozilla/5.0 "quoted" back\slash` + "\x01", UTMSrc: "ig", UTMCmp: "fall & winter"}
	want := `203.0.113.9 - - [04/Nov/2025:08:05:16 +0000] "GET /a%20b/%22c%22?utm_source=ig&utm_campaign=fall+%26+winter HTTP/1.1" 200 512 ` +
		`"https://www.google.com" "Mozilla/5.0 \"quoted\" back\\slash\x01"` + "\n"
	if got := string(AppendCombined(nil, &l)); got != want {
		t.Errorf("combined:\n got %s\nwant %s", got, want)
	}
	// nothing optional recorded: "-" for referrer and User-Agent, no query
	l = LogLine{TS: 1762243516, Method: "POST", Path: "/login", Status: 403, IP: "2001:db8::1"}
	want = `2001:db8::1 - - [04/Nov/2025:08:05:16 +0000] "POST /login HTTP/1.1" 403 0 "-" "-"` + "\n"
	if got := string(AppendCombined([]byte("x"), &l)); got != "x"+want {
		t.Errorf("minimal:\n got %s\nwant %s", got, want)
	}
	l = LogLine{TS: 1, UTMMed: "email"}
	if got := string(AppendCombined(nil, &l)); !strings.HasPrefix(got, `- - - [01/Jan/1970:00:00:01 +0000] "- /?utm_medium=email HTTP/1.1" 0 0`) {
		t.Errorf("empty fields: %s", got)
	}
}

// With masked IPs the search sees only what the console shows: guessing a full address
// does not reveal whether it is in the log.
func TestLogsSearchMaskedIPs(t *testing.T) {
	c := New()
	masked := true
	c.MaskIPs = func() bool { return masked }
	c.Add(&Record{T: 1762243516, H: "h.example", M: "GET", U: "/p", S: 200, IP: "203.0.113.9"})
	c.Add(&Record{T: 1762243517, H: "h.example", M: "GET", U: "/q", S: 200, IP: "2001:db8:1:2::5"})
	for q, n := range map[string]int{"203.0.113.9": 0, ".9": 0, "203.0.113.0": 1, "2001:db8:1:2::5": 0, "::5": 0, "2001:db8:1::/48": 1, "/p": 1} {
		got := c.Logs(LogQuery{Q: q})
		if len(got) != n {
			t.Errorf("masked, search %q: %d lines, want %d", q, len(got), n)
		}
		for _, l := range got {
			if l.IP == "203.0.113.9" || l.IP == "2001:db8:1:2::5" {
				t.Errorf("masked, search %q: full IP %s returned", q, l.IP)
			}
		}
	}
	masked = false
	for q, n := range map[string]int{"203.0.113.9": 1, "::5": 1, "203.0.113.0": 0} {
		if got := len(c.Logs(LogQuery{Q: q})); got != n {
			t.Errorf("unmasked, search %q: %d lines, want %d", q, got, n)
		}
	}
}
