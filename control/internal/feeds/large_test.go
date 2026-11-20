package feeds

import (
	"bytes"
	"context"
	"encoding/binary"
	"encoding/pem"
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/netip"
	"os"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// largeFeed returns a blocklist-like feed: n4 random IPv4 addresses (some as /24
// networks), n6 IPv6 networks, comments.
func largeFeed(n4, n6 int) []byte {
	r := rand.New(rand.NewPCG(42, 43))
	var b bytes.Buffer
	b.Grow(n4*15 + n6*30)
	b.WriteString("# generated test blocklist\n")
	var a [4]byte
	for i := 0; i < n4; i++ {
		binary.BigEndian.PutUint32(a[:], r.Uint32()|0x01000000)
		if i%16 == 0 {
			a[3] = 0
			fmt.Fprintf(&b, "%s/24\n", netip.AddrFrom4(a))
		} else {
			fmt.Fprintf(&b, "%s\n", netip.AddrFrom4(a))
		}
	}
	for i := 0; i < n6; i++ {
		fmt.Fprintf(&b, "2001:db8:%x::/48 ; listed\n", i)
	}
	return b.Bytes()
}

// peakHeap runs f while sampling the heap; it returns the highest heap use above the
// level before f.
func peakHeap(f func()) (peak uint64) {
	runtime.GC()
	var ms runtime.MemStats
	runtime.ReadMemStats(&ms)
	base := ms.HeapInuse
	var top atomic.Uint64
	done := make(chan struct{})
	go func() {
		var m runtime.MemStats
		t := time.NewTicker(2 * time.Millisecond)
		defer t.Stop()
		for {
			select {
			case <-done:
				return
			case <-t.C:
				runtime.ReadMemStats(&m)
				if m.HeapInuse > top.Load() {
					top.Store(m.HeapInuse)
				}
			}
		}
	}()
	f()
	close(done)
	runtime.ReadMemStats(&ms)
	if ms.HeapInuse > top.Load() {
		top.Store(ms.HeapInuse)
	}
	if top.Load() < base {
		return 0
	}
	return top.Load() - base
}

func refreshFeed(t *testing.T, body []byte) {
	t.Helper()
	f := newFixture(t)
	// streamed in chunks without a Content-Length (no size hint), from the test's copy
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		for off := 0; off < len(body); off += 64 << 10 {
			_, _ = w.Write(body[off:min(off+64<<10, len(body))])
		}
	}))
	defer srv.Close()
	f.svc.Client = srv.Client()
	ctx := context.Background()
	l := &store.IPList{Name: "big", Kind: store.ListKindIP, Items: []string{}, SourceURL: srv.URL + "/list", RefreshS: 3600}
	if err := f.st.SaveIPList(ctx, l); err != nil {
		t.Fatal(err)
	}
	var got *store.IPList
	var err error
	start := time.Now()
	peak := peakHeap(func() { got, err = f.svc.Refresh(ctx, l.ID) })
	took := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	fi, serr := os.Stat(f.svc.Lists.File(got.DataHash))
	if serr != nil {
		t.Fatal(serr)
	}
	t.Logf("feed %.1f MB: %d entries → %d ranges, file %.1f MB, refresh %v, peak heap +%.1f MB, warning %q",
		float64(len(body))/1e6, got.ItemCount, got.RangeCount, float64(fi.Size())/1e6, took.Round(time.Millisecond), float64(peak)/1e6, got.FetchWarning)
	if got.ItemCount == 0 || len(got.Items) != 0 || f.applies != 1 {
		t.Fatalf("list %+v", got)
	}
	// The database row stays small whatever the size of the list.
	if peak > 48*uint64(got.ItemCount)+64<<20 {
		t.Errorf("peak heap %d MB for %d entries", peak>>20, got.ItemCount)
	}
}

// A feed of 1.2 million entries is parsed while it downloads into a range file.
func TestRefreshLargeFeed(t *testing.T) {
	if testing.Short() {
		t.Skip("large feed")
	}
	body := largeFeed(1_200_000, 3000)
	refreshFeed(t, body)
	// the body is held by the test server, not by Refresh: the limit applies to the parse
	if !strings.HasPrefix(string(body[:10]), "# ") {
		t.Fatal("unexpected body")
	}
}

// CORVUS_FEED_FILE=<path> measures a real feed (for example a downloaded copy of a
// large community blocklist).
func TestRefreshFeedFile(t *testing.T) {
	p := os.Getenv("CORVUS_FEED_FILE")
	if p == "" {
		t.Skip("CORVUS_FEED_FILE not set")
	}
	body, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	refreshFeed(t, body)
}

// The integration-test client reaches the named private host and trusts the given CA;
// other private targets stay refused.
func TestTestClient(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("192.0.2.1\n"))
	}))
	defer srv.Close()
	ca := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: srv.Certificate().Raw})
	c, err := TestClient([]string{"127.0.0.1"}, ca)
	if err != nil {
		t.Fatal(err)
	}
	if f, err := Fetch(context.Background(), c, srv.URL+"/l", store.ListKindIP); err != nil || f.Count != 1 {
		t.Fatalf("allowed host: %v %v", f, err)
	}
	c, _ = TestClient([]string{"fixtures"}, ca)
	if _, err := Fetch(context.Background(), c, srv.URL+"/l", store.ListKindIP); err == nil || !strings.Contains(err.Error(), "non-public") {
		t.Fatalf("other host: %v", err)
	}
	if _, err := TestClient(nil, []byte("junk")); err == nil {
		t.Fatal("CA without a certificate accepted")
	}
}
