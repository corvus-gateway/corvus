package validate

import (
	"errors"
	"strings"
	"testing"

	"corvus/control/internal/store"
)

func TestUnknownHosts(t *testing.T) {
	for _, c := range []struct {
		name, mode, url string
		field, code     string // "" = valid
		wantMode, want  string // normalized
	}{
		{"default", "", "", "", "", "not_found", ""},
		{"not found drops url", "not_found", "https://example.com", "", "", "not_found", ""},
		{"close", "close", "", "", "", "close", ""},
		{"redirect", "redirect", " HTTPS://WWW.Example.com/Landing?a=1 ", "", "", "redirect", "https://www.example.com/Landing?a=1"},
		{"redirect port", "redirect", "http://example.com:8080/", "", "", "redirect", "http://example.com:8080/"},
		{"bad mode", "drop", "", "mode", "invalid", "", ""},
		{"missing url", "redirect", "", "redirect_url", "required", "", ""},
		{"no scheme", "redirect", "example.com", "redirect_url", "invalid", "", ""},
		{"ftp", "redirect", "ftp://example.com", "redirect_url", "invalid", "", ""},
		{"nginx variable", "redirect", "https://example.com$request_uri", "redirect_url", "invalid", "", ""},
		{"directive injection", "redirect", "https://example.com; return 200", "redirect_url", "invalid", "", ""},
		{"quote", "redirect", `https://example.com/"x`, "redirect_url", "invalid", "", ""},
		{"credentials", "redirect", "https://user:pw@example.com", "redirect_url", "invalid", "", ""},
		{"bad port", "redirect", "https://example.com:99999", "redirect_url", "invalid", "", ""},
		{"too long", "redirect", "https://example.com/" + strings.Repeat("a", MaxUnknownRedirectLen), "redirect_url", "invalid", "", ""},
	} {
		u := store.UnknownHosts{Mode: c.mode, RedirectURL: c.url}
		err := UnknownHosts(&u)
		var ve Errors
		switch {
		case c.code == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.code == "" && (u.Mode != c.wantMode || u.RedirectURL != c.want):
			t.Errorf("%s: normalized to %+v", c.name, u)
		case c.code != "" && (!errors.As(err, &ve) || ve[0].Code != c.code || ve[0].Field != c.field):
			t.Errorf("%s: got %v, want %s %s", c.name, err, c.field, c.code)
		}
	}
}

func TestCustomPage(t *testing.T) {
	for _, c := range []struct {
		name, html, code string
	}{
		{"ok", "<!doctype html><p>{{STATUS}} {{RID}} {{HOST}}</p>", ""},
		{"max size", strings.Repeat("a", MaxCustomPageBytes), ""},
		{"empty", "", "required"},
		{"blank", " \n\t", "required"},
		{"too large", strings.Repeat("a", MaxCustomPageBytes+1), "too_large"},
		{"bad utf8", "<p>\xff</p>", "invalid"},
		{"nul", "<p>\x00</p>", "invalid"},
	} {
		err := CustomPage("html", c.html)
		var ve Errors
		switch {
		case c.code == "" && err != nil:
			t.Errorf("%s: unexpected error %v", c.name, err)
		case c.code != "" && (!errors.As(err, &ve) || ve[0].Code != c.code || ve[0].Field != "html"):
			t.Errorf("%s: got %v, want code %s", c.name, err, c.code)
		}
	}
}
