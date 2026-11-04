package feeds

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

func TestParseIPFeed(t *testing.T) {
	feed := "\xef\xbb\xbf# Tor exit nodes\n" +
		"1.2.3.4\n" +
		"  5.6.7.0/24 ; SBL12345\n" +
		"9.9.9.9 # resolver\n" +
		"; comment\n" +
		"\n" +
		"2001:db8::1\n" +
		"not-an-ip\n" +
		"300.1.1.1\n" +
		"1.2.3.4\n" + // duplicate
		"10.0.0.0/8\tprivate\r\n"
	items, warn, err := Parse(store.ListKindIP, []byte(feed))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"1.2.3.4/32", "5.6.7.0/24", "9.9.9.9/32", "2001:db8::1/128", "10.0.0.0/8"}
	if strings.Join(items, ",") != strings.Join(want, ",") {
		t.Fatalf("items = %v, want %v", items, want)
	}
	if !strings.Contains(warn, "2 invalid lines") {
		t.Fatalf("warning = %q", warn)
	}
}

func TestParseRejectsJunkAndEmpty(t *testing.T) {
	if _, _, err := Parse(store.ListKindIP, []byte("<html><body>Not found</body></html>\n")); err == nil {
		t.Fatal("HTML page accepted as an IP feed")
	}
	if _, _, err := Parse(store.ListKindIP, []byte("# only comments\n\n")); err == nil {
		t.Fatal("empty feed accepted")
	}
}

func TestParseTruncates(t *testing.T) {
	var b strings.Builder
	for i := 0; i < validate.MaxIPListItems+10; i++ {
		fmt.Fprintf(&b, "10.%d.%d.%d\n", i>>16&255, i>>8&255, i&255)
	}
	items, warn, err := Parse(store.ListKindIP, []byte(b.String()))
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != validate.MaxIPListItems || !strings.Contains(warn, "only the first") {
		t.Fatalf("len = %d, warning = %q", len(items), warn)
	}
}

func TestParseUAFeed(t *testing.T) {
	feed := "# bad bots\nAhrefsBot\nIndy\\ Library\n1h4x\\.com\nahrefsbot\nab\n\x01bad\nMozilla/5.0 (compatible; Foo)\n"
	items, warn, err := Parse(store.ListKindUserAgent, []byte(feed))
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"AhrefsBot", "Indy Library", "1h4x.com", "Mozilla/5.0 (compatible; Foo)"}
	if strings.Join(items, "|") != strings.Join(want, "|") {
		t.Fatalf("items = %q, want %q", items, want)
	}
	if !strings.Contains(warn, "2 invalid lines") {
		t.Fatalf("warning = %q", warn)
	}
}

func TestFetchSizeLimitAndStatus(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/big":
			chunk := strings.Repeat("1.2.3.4\n", 1<<14)
			for n := 0; n <= MaxFeedBytes; n += len(chunk) {
				_, _ = w.Write([]byte(chunk))
			}
		case "/ok":
			_, _ = w.Write([]byte("1.2.3.4\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	ctx := context.Background()
	if _, _, err := Fetch(ctx, srv.Client(), srv.URL+"/big", store.ListKindIP); err == nil || !strings.Contains(err.Error(), "larger") {
		t.Fatalf("oversized feed: err = %v", err)
	}
	if _, _, err := Fetch(ctx, srv.Client(), srv.URL+"/missing", store.ListKindIP); err == nil || !strings.Contains(err.Error(), "404") {
		t.Fatalf("404: err = %v", err)
	}
	if items, _, err := Fetch(ctx, srv.Client(), srv.URL+"/ok", store.ListKindIP); err != nil || len(items) != 1 {
		t.Fatalf("ok: %v %v", items, err)
	}
	if _, _, err := Fetch(ctx, srv.Client(), strings.Replace(srv.URL, "https", "http", 1)+"/ok", store.ListKindIP); err == nil {
		t.Fatal("http:// URL fetched")
	}
}

func TestSafeClientRefusesPrivateTargets(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("1.2.3.4\n"))
	}))
	defer srv.Close()
	// srv listens on 127.0.0.1: the guard must refuse the connection after resolution.
	_, _, err := Fetch(context.Background(), SafeClient(), srv.URL+"/x", store.ListKindIP)
	if err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("loopback target: err = %v", err)
	}
	for _, u := range []string{"https://127.0.0.1/x", "https://10.1.2.3/x", "https://[::1]/x", "https://169.254.169.254/latest",
		"https://localhost/x", "http://example.com/x", "https://user:pw@example.com/x", "ftp://example.com/x"} {
		if validate.FeedURL(u) == nil {
			t.Errorf("FeedURL(%q) accepted", u)
		}
	}
	if err := validate.FeedURL("https://check.torproject.org/torbulkexitlist"); err != nil {
		t.Errorf("public URL rejected: %v", err)
	}
	for _, a := range []string{"127.0.0.1:443", "10.0.0.1:443", "192.168.1.1:443", "172.16.0.1:443", "169.254.169.254:80", "100.64.0.1:443", "[::1]:443", "[fe80::1]:443", "[fd00::1]:443", "0.0.0.0:443"} {
		if dialGuard("tcp", a, nil) == nil {
			t.Errorf("dialGuard(%s) allowed", a)
		}
	}
	if err := dialGuard("tcp", "1.1.1.1:443", nil); err != nil {
		t.Errorf("public address refused: %v", err)
	}
}

type fixture struct {
	st      *store.Store
	svc     *Service
	applies int
	body    string
	status  int
	mu      sync.Mutex
	srv     *httptest.Server
}

func newFixture(t *testing.T) *fixture {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "corvus.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	f := &fixture{st: st, status: 200}
	f.srv = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		w.WriteHeader(f.status)
		_, _ = w.Write([]byte(f.body))
	}))
	t.Cleanup(f.srv.Close)
	f.svc = &Service{Store: st, Client: f.srv.Client(),
		Apply: func(ctx context.Context, msg string, mutate func(tx *store.Store) error) error {
			f.applies++
			return st.WithTx(ctx, mutate)
		}}
	return f
}

func (f *fixture) serve(status int, body string) {
	f.mu.Lock()
	f.status, f.body = status, body
	f.mu.Unlock()
}

func TestRefreshKeepsLastGoodAndRedeploysOnlyOnChange(t *testing.T) {
	f := newFixture(t)
	ctx := context.Background()
	l := &store.IPList{Name: "tor", Kind: store.ListKindIP, Items: []string{}, SourceURL: f.srv.URL + "/list", RefreshS: 3600}
	if err := f.st.SaveIPList(ctx, l); err != nil {
		t.Fatal(err)
	}

	f.serve(200, "1.2.3.4\n5.6.7.8\n")
	got, err := f.svc.Refresh(ctx, l.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Items) != 2 || got.FetchedAt == nil || got.FetchError != "" || f.applies != 1 {
		t.Fatalf("first fetch: %+v applies=%d", got, f.applies)
	}

	// Same content in another order: no redeploy.
	f.serve(200, "# header\n5.6.7.8\n1.2.3.4\n")
	if got, err = f.svc.Refresh(ctx, l.ID); err != nil || f.applies != 1 {
		t.Fatalf("unchanged: err=%v applies=%d", err, f.applies)
	}

	// Server error: entries stay, error recorded, no redeploy.
	f.serve(500, "oops")
	got, err = f.svc.Refresh(ctx, l.ID)
	if err == nil || got == nil || len(got.Items) != 2 || !strings.Contains(got.FetchError, "500") || got.FetchedAt == nil || f.applies != 1 {
		t.Fatalf("failed fetch: err=%v list=%+v applies=%d", err, got, f.applies)
	}
	// Junk instead of a list (e.g. a captive portal page): same.
	f.serve(200, "<html>login</html>")
	if got, _ = f.svc.Refresh(ctx, l.ID); len(got.Items) != 2 || got.FetchError == "" || f.applies != 1 {
		t.Fatalf("junk: %+v applies=%d", got, f.applies)
	}

	// Changed content: redeploy, error cleared.
	f.serve(200, "1.2.3.4\n")
	got, err = f.svc.Refresh(ctx, l.ID)
	if err != nil || len(got.Items) != 1 || got.FetchError != "" || f.applies != 2 {
		t.Fatalf("changed: err=%v list=%+v applies=%d", err, got, f.applies)
	}

	// Lists without a URL are not fetched.
	m := &store.IPList{Name: "manual", Items: []string{"1.1.1.1/32"}}
	if err := f.st.SaveIPList(ctx, m); err != nil {
		t.Fatal(err)
	}
	if _, err := f.svc.Refresh(ctx, m.ID); err != ErrNoFeed {
		t.Fatalf("manual list: err = %v", err)
	}
}

func TestDue(t *testing.T) {
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	ago := func(d time.Duration) *time.Time { t := now.Add(-d); return &t }
	cases := []struct {
		l    store.IPList
		want bool
	}{
		{store.IPList{}, false},
		{store.IPList{SourceURL: "https://x/", RefreshS: 3600}, true},
		{store.IPList{SourceURL: "https://x/", RefreshS: 3600, CheckedAt: ago(30 * time.Minute)}, false},
		{store.IPList{SourceURL: "https://x/", RefreshS: 3600, CheckedAt: ago(61 * time.Minute)}, true},
		{store.IPList{SourceURL: "https://x/", RefreshS: 86400, CheckedAt: ago(20 * time.Minute), FetchError: "x"}, true},
	}
	for i, c := range cases {
		if got := Due(c.l, now); got != c.want {
			t.Errorf("case %d: Due = %v, want %v", i, got, c.want)
		}
	}
}
