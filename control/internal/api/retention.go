package api

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"corvus/control/internal/history"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ data retention & log archives

func (s *Server) getRetention(w http.ResponseWriter, r *http.Request) {
	out := map[string]any{"settings": history.Load(r.Context(), s.Store)}
	if s.History != nil {
		out["usage"] = s.History.Usage()
	}
	writeJSON(w, 200, out)
}

func (s *Server) putRetention(w http.ResponseWriter, r *http.Request) {
	var in history.Settings
	if !decode(w, r, &in) {
		return
	}
	if err := history.Validate(&in); err != nil {
		fail(w, validate.Errors{{Field: "retention", Code: "invalid", Message: err.Error()}})
		return
	}
	before := history.Load(r.Context(), s.Store)
	raw, _ := json.Marshal(in)
	if err := s.Store.SetMeta(r.Context(), "retention", string(raw)); err != nil {
		fail(w, err)
		return
	}
	if s.History != nil {
		s.History.Apply(r.Context(), in)
	}
	s.audit(r, "retention.update", "settings:retention", map[string]any{"before": before, "after": in})
	writeJSON(w, 200, map[string]any{"settings": in})
}

func (s *Server) listArchives(w http.ResponseWriter, r *http.Request) {
	if s.History == nil {
		writeJSON(w, 200, []any{})
		return
	}
	writeJSON(w, 200, s.History.Archives())
}

func (s *Server) downloadArchive(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	if s.History == nil {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	p := s.History.ArchivePath(name)
	if p == "" {
		writeErr(w, http.StatusNotFound, "not_found", "not found")
		return
	}
	extendDeadlines(w, 30*time.Minute) // archives can be large
	if r.URL.Query().Get("format") == "combined" {
		// converted on the fly to nginx's "combined" format (GoAccess etc.), streamed.
		// The file is opened (and its gzip header read) before any header is sent, so a
		// missing or unreadable file is an error answer, not an empty download.
		in, err := history.OpenLog(p)
		if errors.Is(err, fs.ErrNotExist) {
			writeErr(w, http.StatusNotFound, "not_found", "not found")
			return
		}
		if err != nil {
			slog.Warn("request log conversion", "file", name, "err", err)
			writeErr(w, http.StatusInternalServerError, "unreadable", "the log file cannot be read")
			return
		}
		defer in.Close()
		s.audit(r, "traffic.log_download", name, map[string]string{"format": "combined"})
		base := strings.TrimSuffix(strings.TrimSuffix(filepath.Base(p), ".gz"), ".jsonl")
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Content-Disposition", `attachment; filename="`+base+`.log"`)
		cw := &countWriter{w: w}
		if err := history.WriteCombined(cw, in); err != nil {
			slog.Warn("request log conversion", "file", name, "err", err)
			if cw.n == 0 {
				w.Header().Del("Content-Disposition")
				writeErr(w, http.StatusInternalServerError, "unreadable", "the log file cannot be read")
				return
			}
			// part of the file is already sent: break the connection so the client sees
			// a failed download instead of a complete-looking, truncated file
			panic(http.ErrAbortHandler)
		}
		return
	}
	s.audit(r, "traffic.log_download", name, nil)
	ctype := "application/x-ndjson"
	if strings.HasSuffix(name, ".gz") {
		ctype = "application/gzip"
	}
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(p)+`"`)
	http.ServeFile(w, r, p)
}

// countWriter counts the bytes written through it.
type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}
