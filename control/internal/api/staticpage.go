package api

import (
	"net/http"

	"corvus/control/internal/compiler"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// previewStaticPage renders the static page of a proxy host form (POST
// /api/proxy-hosts/page-preview): {page, host, lang} → {html}. Built-in templates use the
// console language and show the host name; custom HTML comes back as entered. The
// console shows it in a sandboxed frame (no scripts).
func (s *Server) previewStaticPage(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Page store.StaticPage `json:"page"`
		Host string           `json:"host"`
		Lang string           `json:"lang"`
	}
	if !decode(w, r, &in) {
		return
	}
	h := store.ProxyHost{Content: store.ContentPage, Page: in.Page}
	if err := validate.StaticPage(&h.Page); err != nil {
		fail(w, err)
		return
	}
	b, err := compiler.PreviewStaticPage(h.Page, in.Lang, in.Host)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"html": string(b), "status": h.Page.Status})
}
