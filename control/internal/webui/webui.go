// Package webui serves the embedded admin single-page application (built from web/).
package webui

import (
	"bytes"
	"compress/gzip"
	"embed"
	"io/fs"
	"mime"
	"net/http"
	"path"
	"strconv"
	"strings"
)

//go:embed all:dist
var dist embed.FS

// minGzip is the smallest file worth compressing.
const minGzip = 1024

// textTypes are the file extensions served gzip-compressed to clients that accept it.
var textTypes = map[string]bool{".html": true, ".js": true, ".css": true, ".svg": true, ".json": true, ".txt": true, ".webmanifest": true}

// Handler serves static assets and falls back to index.html for client-side routes.
func Handler() http.Handler {
	root, _ := fs.Sub(dist, "dist")
	return handler(root)
}

func handler(root fs.FS) http.Handler {
	files := http.FileServer(http.FS(root))
	gz := compressAll(root)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(path.Clean(r.URL.Path), "/")
		if p == "" {
			p = "index.html"
		}
		if _, err := fs.Stat(root, p); err != nil {
			p = "index.html"
			r = r.Clone(r.Context())
			r.URL.Path = "/"
		}
		if strings.HasPrefix(p, "assets/") {
			w.Header().Set("Cache-Control", "public, max-age=31536000, immutable") // hashed file names
		} else {
			w.Header().Set("Cache-Control", "no-cache")
		}
		if _, err := fs.Stat(root, "index.html"); err != nil {
			http.Error(w, "admin UI is not built (run: npm --prefix web run build)", http.StatusNotFound)
			return
		}
		if body, ok := gz[p]; ok {
			w.Header().Add("Vary", "Accept-Encoding")
			if (r.Method == http.MethodGet || r.Method == http.MethodHead) && r.Header.Get("Range") == "" && acceptsGzip(r.Header.Get("Accept-Encoding")) {
				ct := mime.TypeByExtension(path.Ext(p))
				if ct == "" {
					ct = "application/octet-stream"
				}
				w.Header().Set("Content-Type", ct)
				w.Header().Set("Content-Encoding", "gzip")
				w.Header().Set("Content-Length", strconv.Itoa(len(body)))
				w.WriteHeader(http.StatusOK)
				if r.Method == http.MethodGet {
					_, _ = w.Write(body)
				}
				return
			}
		}
		files.ServeHTTP(w, r)
	})
}

// compressAll gzips the text files of the bundle once, at start-up (the bundle is embedded
// and never changes while running). Files that don't shrink are left out.
func compressAll(root fs.FS) map[string][]byte {
	out := map[string][]byte{}
	_ = fs.WalkDir(root, ".", func(p string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !textTypes[path.Ext(p)] {
			return nil
		}
		raw, err := fs.ReadFile(root, p)
		if err != nil || len(raw) < minGzip {
			return nil
		}
		var b bytes.Buffer
		zw, _ := gzip.NewWriterLevel(&b, gzip.BestCompression)
		_, _ = zw.Write(raw)
		_ = zw.Close()
		if b.Len() < len(raw) {
			out[p] = b.Bytes()
		}
		return nil
	})
	return out
}

// acceptsGzip reports whether an Accept-Encoding header allows gzip (q=0 refuses it).
func acceptsGzip(h string) bool {
	for _, part := range strings.Split(h, ",") {
		name, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		if !strings.EqualFold(strings.TrimSpace(name), "gzip") && strings.TrimSpace(name) != "*" {
			continue
		}
		q := strings.ReplaceAll(strings.TrimSpace(params), " ", "")
		return q != "q=0" && q != "q=0.0" && q != "q=0.00" && q != "q=0.000"
	}
	return false
}
