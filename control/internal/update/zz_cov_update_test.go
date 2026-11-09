package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCovRunReturns(t *testing.T) {
	done := make(chan struct{})
	go func() { (&Checker{Current: "dev"}).Run(context.Background()); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("a development build must not poll")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	done = make(chan struct{})
	go func() { (&Checker{Current: "1.0.0"}).Run(ctx); close(done) }()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run must stop when the context ends")
	}
	var nilChecker *Checker
	if nilChecker.Available() != nil {
		t.Fatal("nil checker has nothing available")
	}
}

func TestCovCheckErrors(t *testing.T) {
	ctx := context.Background()
	body, code := "", 200
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "corvus-awpg" || !strings.Contains(r.Header.Get("Accept"), "github") {
			w.WriteHeader(400)
			return
		}
		w.WriteHeader(code)
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	c := &Checker{Current: "1.0.0", URL: srv.URL, Client: srv.Client()}

	code = 403
	err := c.Check(ctx)
	if err == nil || err.Error() != "GitHub answered 403" {
		t.Fatalf("status error: %v", err)
	}
	code, body = 200, "{not json"
	if err := c.Check(ctx); err == nil {
		t.Fatal("invalid JSON must fail")
	}
	// a release link outside the project is replaced by the project's release page
	body = `{"tag_name":"v1.1.0","html_url":"https://evil.example.com/x"}`
	if err := c.Check(ctx); err != nil {
		t.Fatal(err)
	}
	if r := c.Available(); r == nil || r.Version != "1.1.0" || r.URL != releasePrefix+"latest" {
		t.Fatalf("release: %+v", r)
	}
	// a later check that finds nothing newer clears it
	body = `{"tag_name":"v1.0.0"}`
	_ = c.Check(ctx)
	if c.Available() != nil {
		t.Fatal("no longer newer")
	}
	bad := &Checker{Current: "1.0.0", URL: "://bad url"}
	if err := bad.Check(ctx); err == nil {
		t.Fatal("malformed URL must fail")
	}
	dead := httptest.NewServer(http.NotFoundHandler())
	dead.Close()
	if err := (&Checker{Current: "1.0.0", URL: dead.URL}).Check(ctx); err == nil {
		t.Fatal("unreachable server must fail")
	}
}
