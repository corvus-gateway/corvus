package autoban

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestBanKeys(t *testing.T) {
	for _, c := range []struct{ ip, key, gw string }{
		{"192.0.2.7", "192.0.2.7", "192.0.2.7"},
		{"::ffff:192.0.2.7", "192.0.2.7", "192.0.2.7"},
		// must match the gateway's ipkey.client() (gateway/test/ipkey_test.lua)
		{"2001:db8:1:2:aaaa::1", "2001:db8:1:2::/64", "2001:0db8:0001:0002"},
		{"2001:DB8:1:2:bbbb:cccc:dddd:eeee", "2001:db8:1:2::/64", "2001:0db8:0001:0002"},
		{"nope", "", ""},
	} {
		if got := BanKey(c.ip); got != c.key {
			t.Errorf("BanKey(%q) = %q, want %q", c.ip, got, c.key)
		}
		if got := GatewayKey(c.key); got != c.gw {
			t.Errorf("GatewayKey(%q) = %q, want %q", c.key, got, c.gw)
		}
	}
	// manual bans of single addresses keep their exact key
	if got := GatewayKey("2001:db8::1"); got != "2001:db8::1" {
		t.Errorf("exact IPv6: %q", got)
	}
	// hits from the gateway: address or /64 key
	for in, want := range map[string]string{"192.0.2.7": "192.0.2.7", "2001:0db8:0001:0002": "2001:db8:1:2::/64", "2001:db8::5": "2001:db8::/64", "x:y": ""} {
		if got, _ := parseHitKey(in); got != want {
			t.Errorf("parseHitKey(%q) = %q, want %q", in, got, want)
		}
	}
	_, ex, _ := net.ParseCIDR("2001:db8:1:2::5/128")
	if !exemptKey("2001:db8:1:2::/64", []*net.IPNet{ex}) {
		t.Error("a /64 containing an exempt address must not be banned")
	}
	if exemptKey("2001:db8:1:3::/64", []*net.IPNet{ex}) || !exemptKey("127.0.0.1", nil) {
		t.Error("exemptKey")
	}
}

// Detections from several addresses of one IPv6 /64 add up to one ban of the /64,
// which the gateway gets under its /64 key.
func TestAutoBanIPv6Prefix(t *testing.T) {
	s, tc, st := setup(t)
	ctx := context.Background()
	set := DefaultSettings()
	raw, _ := json.Marshal(set)
	_ = st.SetMeta(ctx, "autoban", string(raw))
	now := time.Now()
	attacks(tc, "2001:db8:1:2::a", set.Threshold/2+1, now)
	attacks(tc, "2001:db8:1:2::b", set.Threshold/2+1, now)
	got := s.Evaluate(ctx, now)
	if len(got) != 1 || got[0] != "2001:db8:1:2::/64" {
		t.Fatalf("banned %v", got)
	}
	var body map[string]any
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(b, &body)
	}))
	defer gw.Close()
	s.PushURL = gw.URL
	if err := s.Push(ctx, now); err != nil {
		t.Fatal(err)
	}
	if ips, _ := body["ips"].(map[string]any); ips["2001:0db8:0001:0002"] == nil {
		t.Fatalf("pushed %v", body["ips"])
	}
}
