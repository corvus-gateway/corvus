package validate

import (
	"errors"
	"strings"
	"testing"
)

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
