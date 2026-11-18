package autoban

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

// The gateway's internal status endpoints require the per-start key, so they are not
// reachable through a proxied target: every request to them carries it.
func TestStatusKeySent(t *testing.T) {
	s, _, _ := setup(t)
	var mu sync.Mutex
	got := map[string]string{}
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		got[r.URL.Path] = r.Header.Get(StatusKeyHeader)
		mu.Unlock()
		if r.Header.Get(StatusKeyHeader) != "per-start-key" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "forbidden")
			return
		}
		if r.URL.Path == "/bad-behaviour" {
			_, _ = io.WriteString(w, `{"hits":[],"epoch":"e1"}`)
			return
		}
		_, _ = io.WriteString(w, "0")
	}))
	defer gw.Close()
	s.PushURL, s.HitsURL, s.StatusKey = gw.URL+"/bans", gw.URL+"/bad-behaviour", "per-start-key"
	ctx := context.Background()
	if _, err := s.Pull(ctx); err != nil {
		t.Fatal(err)
	}
	if ok, err := s.sync(ctx, time.Now(), false); !ok || err != nil {
		t.Fatalf("push: %v %v", ok, err)
	}
	mu.Lock()
	if got["/bans"] != "per-start-key" || got["/bad-behaviour"] != "per-start-key" {
		t.Fatalf("headers: %v", got)
	}
	mu.Unlock()

	// a wrong key is refused by the gateway: the push reports it
	s.StatusKey = "wrong"
	s.Trigger()
	if _, err := s.sync(ctx, time.Now().Add(time.Minute), false); err == nil {
		t.Fatal("refused push not reported")
	}
}
