package validate

import (
	"errors"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func fieldCodes(err error) map[string]string {
	out := map[string]string{}
	var errs Errors
	if errors.As(err, &errs) {
		for _, e := range errs {
			out[e.Field] = e.Code
		}
	}
	return out
}

// A host that serves a page needs no forward target; the page gets defaults per
// template; forward mode is unchanged (forward target required).
func TestProxyHostStaticPage(t *testing.T) {
	page := func(p store.StaticPage) *store.ProxyHost {
		return &store.ProxyHost{Domains: []string{"a.example.com"}, Content: store.ContentPage, ForwardPort: 80, Page: p,
			ExtraTargets: []store.Target{{ForwardHost: "x", ForwardPort: 1}}}
	}
	h := page(store.StaticPage{Template: store.PageMaintenance, Title: "  Back at 6  ", Message: "line 1\r\nline 2", HTML: "<p>dropped</p>"})
	if err := ProxyHost(h); err != nil {
		t.Fatal(err)
	}
	if h.Page.Status != 503 || h.Page.Title != "Back at 6" || h.Page.Message != "line 1\nline 2" || h.Page.HTML != "" {
		t.Fatalf("maintenance defaults: %+v", h.Page)
	}
	if h.ForwardHost != "" || h.ForwardPort != 0 || len(h.ExtraTargets) != 0 || h.ForwardScheme != "http" {
		t.Fatalf("forward target of a page host without one: %q %d %+v", h.ForwardHost, h.ForwardPort, h.ExtraTargets)
	}
	h = page(store.StaticPage{})
	if err := ProxyHost(h); err != nil || h.Page.Template != store.PageComingSoon || h.Page.Status != 200 {
		t.Fatalf("empty page: %+v %v", h.Page, err)
	}
	h = page(store.StaticPage{Template: store.PageCustom, Status: 410, Title: "x", Message: "y", HTML: "<h1>Gone</h1>"})
	if err := ProxyHost(h); err != nil || h.Page.Title != "" || h.Page.Message != "" || h.Page.HTML != "<h1>Gone</h1>" {
		t.Fatalf("custom: %+v %v", h.Page, err)
	}
	// a forward target entered on a page host is kept (and checked) for switching back
	h = page(store.StaticPage{Template: store.PageParked})
	h.ForwardHost, h.ForwardPort, h.ExtraTargets = " App ", 8080, nil
	if err := ProxyHost(h); err != nil || h.ForwardHost != "app" || h.ForwardPort != 8080 {
		t.Fatalf("kept forward target: %+v %v", h, err)
	}
	h.ForwardHost = "a b"
	if c := fieldCodes(ProxyHost(h)); c["forward_host"] != "invalid" {
		t.Fatalf("invalid kept forward host: %v", c)
	}
	// forward mode still requires the forward target; "" is forward
	f := &store.ProxyHost{Domains: []string{"a.example.com"}}
	if c := fieldCodes(ProxyHost(f)); c["forward_host"] != "invalid" || c["forward_port"] != "invalid" || f.Content != store.ContentForward {
		t.Fatalf("forward mode: %v %q", c, f.Content)
	}
	// a page kept while the host forwards is still checked
	f = &store.ProxyHost{Domains: []string{"a.example.com"}, ForwardHost: "app", ForwardPort: 80, Content: store.ContentForward,
		Page: store.StaticPage{Template: store.PageCustom, HTML: "<p>kept</p>"}}
	if err := ProxyHost(f); err != nil || f.Page.HTML != "<p>kept</p>" || f.Page.Status != 200 {
		t.Fatalf("page kept in forward mode: %+v %v", f.Page, err)
	}
	f.Page.Status = 302
	if c := fieldCodes(ProxyHost(f)); c["page.status"] != "invalid" {
		t.Fatalf("page checked in forward mode: %v", c)
	}

	big := strings.Repeat("a", MaxPageHTMLBytes+1)
	cases := map[string]struct {
		h           *store.ProxyHost
		field, code string
	}{
		"content":           {&store.ProxyHost{Domains: []string{"a.example.com"}, ForwardHost: "app", ForwardPort: 80, Content: "redirect"}, "content", "invalid"},
		"template":          {page(store.StaticPage{Template: "fancy"}), "page.template", "invalid"},
		"status":            {page(store.StaticPage{Template: store.PageParked, Status: 500}), "page.status", "invalid"},
		"custom empty":      {page(store.StaticPage{Template: store.PageCustom, HTML: " \n"}), "page.html", "required"},
		"custom too large":  {page(store.StaticPage{Template: store.PageCustom, HTML: big}), "page.html", "too_large"},
		"custom not utf-8":  {page(store.StaticPage{Template: store.PageCustom, HTML: "\xff\xfe"}), "page.html", "invalid"},
		"custom NUL":        {page(store.StaticPage{Template: store.PageCustom, HTML: "a\x00b"}), "page.html", "invalid"},
		"title too long":    {page(store.StaticPage{Template: store.PageParked, Title: strings.Repeat("가", MaxPageTitleLen+1)}), "page.title", "too_long"},
		"title two lines":   {page(store.StaticPage{Template: store.PageParked, Title: "a\nb"}), "page.title", "invalid"},
		"message too long":  {page(store.StaticPage{Template: store.PageParked, Message: strings.Repeat("m", MaxPageMessageLen+1)}), "page.message", "too_long"},
		"message not utf-8": {page(store.StaticPage{Template: store.PageParked, Message: "\xff"}), "page.message", "invalid"},
	}
	for name, c := range cases {
		if got := fieldCodes(ProxyHost(c.h))[c.field]; got != c.code {
			t.Errorf("%s: %s = %q, want %q", name, c.field, got, c.code)
		}
	}
	// exactly at the limits
	ok := page(store.StaticPage{Template: store.PageCustom, HTML: strings.Repeat("a", MaxPageHTMLBytes)})
	if err := ProxyHost(ok); err != nil {
		t.Errorf("256 KB page: %v", err)
	}
	ok = page(store.StaticPage{Template: store.PageParked, Title: strings.Repeat("가", MaxPageTitleLen), Message: strings.Repeat("m", MaxPageMessageLen)})
	if err := ProxyHost(ok); err != nil {
		t.Errorf("title/message at the limit: %v", err)
	}
	for _, s := range PageStatuses {
		if err := ProxyHost(page(store.StaticPage{Template: store.PageParked, Status: s})); err != nil {
			t.Errorf("status %d: %v", s, err)
		}
	}
	if err := StaticPage(&store.StaticPage{Template: "x"}); err == nil {
		t.Error("StaticPage: unknown template accepted")
	}
}

// Custom locations still forward on a page host, except "/", which would replace the
// page (as would a location / in the advanced configuration).
func TestProxyHostStaticPageLocations(t *testing.T) {
	h := &store.ProxyHost{Domains: []string{"a.example.com"}, Content: store.ContentPage, Page: store.StaticPage{Template: store.PageMaintenance},
		Locations: []store.Location{{Path: "/api", ForwardHost: "api", ForwardPort: 8080}}}
	if err := ProxyHost(h); err != nil {
		t.Fatalf("page host with a location: %v", err)
	}
	h.Locations = append(h.Locations, store.Location{Path: " / ", ForwardHost: "web", ForwardPort: 80})
	if c := fieldCodes(ProxyHost(h)); c["locations[1].path"] != "page_host" {
		t.Fatalf("location / on a page host: %v", c)
	}
	h.Locations = h.Locations[:1]
	h.AdvancedConfig = "location / { return 204; }"
	if c := fieldCodes(ProxyHost(h)); c["advanced_config"] != "page_host" {
		t.Fatalf("advanced location / on a page host: %v", c)
	}
	// the same is fine for a host that forwards
	h.Content, h.ForwardHost, h.ForwardPort = store.ContentForward, "web", 80
	h.Locations = []store.Location{{Path: "/", ForwardHost: "web2", ForwardPort: 80}}
	h.AdvancedConfig = ""
	if err := ProxyHost(h); err != nil {
		t.Fatalf("forward host with location /: %v", err)
	}
}
