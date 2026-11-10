package alerts

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

// Delivery errors are stored in the alert history; they must not carry the secret
// parts of a webhook URL or the receiver's response body.
func TestSendErrorsHideWebhookSecrets(t *testing.T) {
	allowLoopback = true
	t.Cleanup(func() { allowLoopback = false })
	m := Message{Title: "t", Time: time.Now()}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "invalid token for "+r.URL.Path, http.StatusForbidden)
	}))
	secret := "/api/webhooks/123/S3CR3T-PATH"
	code, err := Send(context.Background(), "discord", Target{URL: srv.URL + secret + "?key=QUERYSECRET"}, m)
	if code != 403 || err == nil || err.Error() != "HTTP 403" {
		t.Fatalf("code=%d err=%v", code, err)
	}
	srv.Close() // now the connection is refused: a transport error
	client.CloseIdleConnections()
	_, err = Send(context.Background(), "slack", Target{URL: srv.URL + secret + "?key=QUERYSECRET"}, m)
	if err == nil {
		t.Fatal("closed server reachable")
	}
	host := strings.TrimPrefix(srv.URL, "http://")
	if strings.Contains(err.Error(), "S3CR3T") || strings.Contains(err.Error(), "QUERYSECRET") || !strings.Contains(err.Error(), "Post http://"+host+"/…: ") {
		t.Fatalf("transport error: %v", err)
	}
}

func TestTransportErrorRedaction(t *testing.T) {
	dest, _ := url.Parse("https://hooks.example.com/services/T0/B0/XYZ?token=abc123")
	err := transportError(&url.Error{Op: "Post", URL: dest.String(), Err: errors.New("dial tcp: i/o timeout")}, dest, Target{})
	if err.Error() != "Post https://hooks.example.com/…: dial tcp: i/o timeout" {
		t.Fatalf("got %q", err)
	}
	// errors not wrapped in *url.Error still lose the path, query and secrets
	err = transportError(errors.New("failed at "+dest.String()+" with hmac-key"), dest, Target{Secret: "hmac-key"})
	if s := err.Error(); strings.Contains(s, "XYZ") || strings.Contains(s, "abc123") || strings.Contains(s, "hmac-key") {
		t.Fatalf("got %q", s)
	}
}

func TestBlockedIPTransitionAddresses(t *testing.T) {
	for ip, want := range map[string]bool{
		"64:ff9b::7f00:1":                      true,  // NAT64 of loopback
		"64:ff9b::a9fe:a9fe":                   true,  // NAT64 of link-local metadata
		"2002:a9fe:a9fe::1":                    true,  // 6to4 of link-local metadata
		"64:ff9b:1::1":                         true,  // local-use NAT64
		"2001:0:4136:e378:8000:63bf:3fff:fdd2": true,  // Teredo
		"64:ff9b::a00:5":                       false, // NAT64 of a private address (allowed like 10.0.0.5)
		"2002:808:808::1":                      false,
		"2606:4700::1":                         false,
	} {
		if got := blockedIP(net.ParseIP(ip)); got != want {
			t.Errorf("blockedIP(%s) = %v, want %v", ip, got, want)
		}
	}
	if err := CheckTarget("slack", Target{URL: "https://[64:ff9b::7f00:1]/hook"}); err == nil {
		t.Error("embedded loopback webhook accepted")
	}
}

func TestAlertsClientIgnoresProxyEnvironment(t *testing.T) {
	if tr, ok := client.Transport.(*http.Transport); !ok || tr.Proxy != nil {
		t.Fatal("alert deliveries must connect directly so the destination check applies")
	}
}
