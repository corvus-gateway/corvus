package api

import (
	"encoding/json"
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
	s.audit(r, "traffic.log_download", name, nil)
	ctype := "application/x-ndjson"
	if strings.HasSuffix(name, ".gz") {
		ctype = "application/gzip"
	}
	extendDeadlines(w, 30*time.Minute) // archives can be large
	w.Header().Set("Content-Type", ctype)
	w.Header().Set("Content-Disposition", `attachment; filename="`+filepath.Base(p)+`"`)
	http.ServeFile(w, r, p)
}
