package api

import (
	"net/http"
	"os"
	"path/filepath"

	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ custom block / error pages

// pageFiles maps the API page names to the built-in files in the HTML directory.
var pageFiles = map[string]string{"block": "blocked.html", "error": "error.html"}

type pageView struct {
	Custom      bool   `json:"custom"`
	HTML        string `json:"html"`         // the page served now (custom or built-in)
	DefaultHTML string `json:"default_html"` // built-in page ("" when unavailable)
}

func pageOf(p *store.CustomPages, name string) *string {
	if name == "block" {
		return &p.Block
	}
	return &p.Error
}

func (s *Server) pageView(p store.CustomPages, name string) pageView {
	v := pageView{Custom: *pageOf(&p, name) != ""}
	if s.Paths.HTMLDir != "" {
		if b, err := os.ReadFile(filepath.Join(s.Paths.HTMLDir, pageFiles[name])); err == nil {
			v.DefaultHTML = string(b)
		}
	}
	v.HTML = v.DefaultHTML
	if v.Custom {
		v.HTML = *pageOf(&p, name)
	}
	return v
}

func (s *Server) getPages(w http.ResponseWriter, r *http.Request) {
	p, err := s.Store.CustomPages(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{
		"pages":        map[string]pageView{"block": s.pageView(p, "block"), "error": s.pageView(p, "error")},
		"max_bytes":    validate.MaxCustomPageBytes,
		"placeholders": validate.PagePlaceholders,
	})
}

// putPage stores a custom page (PUT, {"html": "..."}) or restores the built-in
// page (POST …/reset) and deploys the result.
func (s *Server) putPage(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("page")
	if _, ok := pageFiles[name]; !ok {
		writeErr(w, http.StatusNotFound, "not_found", "unknown page")
		return
	}
	var in struct {
		HTML string `json:"html"`
	}
	reset := r.Method == http.MethodPost
	if !reset {
		if !decode(w, r, &in) {
			return
		}
		if err := validate.CustomPage("html", in.HTML); err != nil {
			fail(w, err)
			return
		}
	}
	msg := "update custom " + name + " page"
	if reset {
		msg = "reset " + name + " page to default"
	}
	var before store.CustomPages
	ver, err := s.Deployer.Apply(r.Context(), userID(r), msg, func(tx *store.Store) error {
		var err error
		if before, err = tx.CustomPages(r.Context()); err != nil {
			return err
		}
		p := before
		*pageOf(&p, name) = in.HTML
		return tx.SetCustomPages(r.Context(), p)
	})
	if err != nil {
		fail(w, err)
		return
	}
	action := "page.update"
	if reset {
		action = "page.reset"
	}
	// The page bodies can be large: the audit log records sizes, not content.
	s.audit(r, action, "settings:page:"+name, map[string]any{"before_bytes": len(*pageOf(&before, name)), "after_bytes": len(in.HTML)})
	p, _ := s.Store.CustomPages(r.Context())
	writeJSON(w, 200, map[string]any{"page": s.pageView(p, name), "version": ver})
}
