package validate

import (
	"fmt"
	"strings"
	"unicode/utf8"

	"corvus/control/internal/store"
)

// Limits of a proxy host's static page.
const (
	MaxPageHTMLBytes  = 256 * 1024 // custom HTML
	MaxPageTitleLen   = 200        // characters
	MaxPageMessageLen = 2000       // characters
)

// PageStatuses are the HTTP status codes a static page may answer with.
var PageStatuses = []int{200, 404, 410, 503}

// PageTemplates are the static page templates (store.PageCustom: the operator's HTML).
var PageTemplates = []string{store.PageParked, store.PageComingSoon, store.PageMaintenance, store.PageCustom}

// content checks the host's content mode and its page. In forward mode the forward
// target is required (as always); in page mode it is optional, and kept when given (so
// that switching back keeps it). The page is checked whenever it is set, also while the
// host forwards: switching modes keeps it as well.
func content(errs *Errors, h *store.ProxyHost) {
	switch h.Content {
	case "":
		h.Content = store.ContentForward
	case store.ContentForward, store.ContentPage:
	default:
		errs.add("content", "invalid", "content must be forward or page")
	}
	if h.IsPage() || !h.Page.IsZero() {
		staticPage(errs, &h.Page)
	}
	if !h.IsPage() {
		return
	}
	for i, l := range h.Locations {
		if strings.TrimSpace(l.Path) == "/" {
			errs.add(fmt.Sprintf("locations[%d].path", i), "page_host", "a host that serves a page cannot have a location /: it would replace the page (forward the host instead)")
		}
	}
	if HasRootLocation(h.AdvancedConfig) {
		errs.add("advanced_config", "page_host", "a host that serves a page cannot define location / in its advanced configuration")
	}
}

// StaticPage normalizes and validates a single static page in place.
func StaticPage(p *store.StaticPage) error {
	var errs Errors
	staticPage(&errs, p)
	return errs.OrNil()
}

// staticPage normalizes and validates a static page in place.
func staticPage(errs *Errors, p *store.StaticPage) {
	defer func() { p.HTMLBytes = len(p.HTML) }()
	p.Template = strings.TrimSpace(p.Template)
	if p.Template == "" {
		p.Template = store.PageComingSoon
	}
	known := false
	for _, t := range PageTemplates {
		known = known || p.Template == t
	}
	if !known {
		errs.add("page.template", "invalid", "template must be parked, coming_soon, maintenance or custom")
	}
	if p.Status == 0 {
		p.Status = 200
		if p.Template == store.PageMaintenance {
			p.Status = 503
		}
	}
	ok := false
	for _, s := range PageStatuses {
		ok = ok || p.Status == s
	}
	if !ok {
		errs.add("page.status", "invalid", "status must be 200, 404, 410 or 503")
	}
	p.Title, p.Message = strings.TrimSpace(p.Title), strings.TrimSpace(strings.ReplaceAll(p.Message, "\r\n", "\n"))
	if p.Template == store.PageCustom {
		// title and message belong to the built-in templates
		p.Title, p.Message = "", ""
		switch {
		case strings.TrimSpace(p.HTML) == "":
			errs.add("page.html", "required", "enter the HTML of the page")
		case len(p.HTML) > MaxPageHTMLBytes:
			errs.add("page.html", "too_large", "the page must be at most %d KB", MaxPageHTMLBytes/1024)
		case !utf8.ValidString(p.HTML) || strings.ContainsRune(p.HTML, 0):
			errs.add("page.html", "invalid", "the page must be UTF-8 text")
		}
		return
	}
	p.HTML = "" // built-in templates have no HTML of their own
	text := func(field, v string, max int) {
		switch {
		case !utf8.ValidString(v) || strings.ContainsRune(v, 0):
			errs.add(field, "invalid", "must be UTF-8 text")
		case utf8.RuneCountInString(v) > max:
			errs.add(field, "too_long", "at most %d characters", max)
		}
	}
	text("page.title", p.Title, MaxPageTitleLen)
	text("page.message", p.Message, MaxPageMessageLen)
	if strings.ContainsAny(p.Title, "\n\r") {
		errs.add("page.title", "invalid", "the title must be a single line")
	}
}
