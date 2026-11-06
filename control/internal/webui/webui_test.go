package webui

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http/httptest"
	"strings"
	"testing"
	"testing/fstest"
)

func TestGzipAssets(t *testing.T) {
	js := strings.Repeat("console.log('corvus');\n", 200)
	h := handler(fstest.MapFS{
		"index.html":      {Data: []byte("<!doctype html>" + strings.Repeat("<div></div>", 200))},
		"assets/app.js":   {Data: []byte(js)},
		"assets/tiny.js":  {Data: []byte("x()")},
		"assets/logo.png": {Data: bytes.Repeat([]byte{1}, 4096)},
	})
	get := func(p, enc string) *httptest.ResponseRecorder {
		r := httptest.NewRequest("GET", p, nil)
		if enc != "" {
			r.Header.Set("Accept-Encoding", enc)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}

	w := get("/assets/app.js", "gzip, deflate, br")
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || !strings.Contains(w.Header().Get("Content-Type"), "javascript") ||
		w.Header().Get("Vary") != "Accept-Encoding" || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Fatalf("gzip js: %d %v", w.Code, w.Header())
	}
	zr, err := gzip.NewReader(w.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body, _ := io.ReadAll(zr); string(body) != js {
		t.Fatal("decompressed body differs")
	}

	for _, c := range []struct{ path, enc string }{
		{"/assets/app.js", ""}, {"/assets/app.js", "gzip;q=0"}, {"/assets/app.js", "br"},
		{"/assets/tiny.js", "gzip"}, {"/assets/logo.png", "gzip"},
	} {
		if w := get(c.path, c.enc); w.Code != 200 || w.Header().Get("Content-Encoding") != "" {
			t.Fatalf("%s with %q: %d %v", c.path, c.enc, w.Code, w.Header())
		}
	}

	// client-side routes fall back to index.html, compressed and not cached
	w = get("/proxy-hosts", "gzip")
	if w.Code != 200 || w.Header().Get("Content-Encoding") != "gzip" || !strings.HasPrefix(w.Header().Get("Content-Type"), "text/html") || w.Header().Get("Cache-Control") != "no-cache" {
		t.Fatalf("spa fallback: %d %v", w.Code, w.Header())
	}
}

func TestAcceptsGzip(t *testing.T) {
	for h, want := range map[string]bool{
		"gzip": true, "deflate, gzip;q=0.5": true, "GZIP": true, "*": true, "br": false, "": false, "gzip;q=0": false, "gzip; q=0.000": false,
	} {
		if got := acceptsGzip(h); got != want {
			t.Errorf("acceptsGzip(%q) = %v", h, got)
		}
	}
}
