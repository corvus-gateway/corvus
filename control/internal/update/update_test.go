package update

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestCheck(t *testing.T) {
	body := ""
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") == "" {
			t.Error("no User-Agent")
		}
		_, _ = w.Write([]byte(body))
	}))
	defer srv.Close()
	ctx := context.Background()
	for _, c := range []struct {
		current, body string
		want          *Release
	}{
		{"1.2.0", `{"tag_name":"v1.2.1","html_url":"https://github.com/corvus-gateway/corvus/releases/tag/v1.2.1"}`,
			&Release{"1.2.1", "https://github.com/corvus-gateway/corvus/releases/tag/v1.2.1"}},
		{"1.2.0", `{"tag_name":"v1.10.0","html_url":"https://evil.example/x"}`,
			&Release{"1.10.0", "https://github.com/corvus-gateway/corvus/releases/latest"}},
		{"1.2.0", `{"tag_name":"v1.2.0","html_url":"https://github.com/corvus-gateway/corvus/releases/tag/v1.2.0"}`, nil},
		{"1.3.0", `{"tag_name":"v1.2.9"}`, nil},
		{"1.2.0", `{"tag_name":"v1.3.0-rc.1","prerelease":true}`, nil},
		{"1.2.0", `{"tag_name":"nightly"}`, nil},
	} {
		body = c.body
		ch := &Checker{Current: c.current, URL: srv.URL}
		if err := ch.Check(ctx); err != nil {
			t.Fatal(err)
		}
		got := ch.Available()
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("%s / %s: got %+v, want %+v", c.current, c.body, got, c.want)
		}
	}
	if (*Checker)(nil).Available() != nil {
		t.Error("nil checker")
	}
}
