package validate

import (
	"fmt"
	"strings"
	"unicode/utf8"
)

// MaxCustomPageBytes caps an operator-supplied block or error page.
const MaxCustomPageBytes = 64 * 1024

// PagePlaceholders are replaced by the gateway when a block or error page is served
// (values are HTML-escaped). Unknown {{NAME}} placeholders render empty.
var PagePlaceholders = []string{"{{STATUS}}", "{{RID}}", "{{HOST}}", "{{IP}}", "{{TIME}}"}

// CustomPage checks an operator-supplied HTML page. The page is written verbatim to
// a file the gateway serves (it is never interpreted as nginx configuration), so
// only presence, size and encoding are restricted.
func CustomPage(field, html string) error {
	switch {
	case strings.TrimSpace(html) == "":
		return Errors{{Field: field, Code: "required", Message: "the page must not be empty"}}
	case len(html) > MaxCustomPageBytes:
		return Errors{{Field: field, Code: "too_large", Message: fmt.Sprintf("the page must be at most %d KB", MaxCustomPageBytes/1024)}}
	case !utf8.ValidString(html) || strings.ContainsRune(html, 0):
		return Errors{{Field: field, Code: "invalid", Message: "the page must be UTF-8 text"}}
	}
	return nil
}
