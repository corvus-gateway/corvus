package validate

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

// MaxNotes is the longest note (in characters) on a host or stream.
const MaxNotes = 1000

// notes normalizes a free-text note in place: Windows line endings become "\n" and
// surrounding whitespace is trimmed. Control characters other than newline and tab are
// refused. Notes are only shown in the console and never reach the gateway configuration.
func notes(errs *Errors, v *string) {
	s := strings.TrimSpace(strings.ReplaceAll(*v, "\r\n", "\n"))
	*v = s
	if !utf8.ValidString(s) {
		errs.add("notes", "invalid", "notes must be valid UTF-8")
		return
	}
	if n := utf8.RuneCountInString(s); n > MaxNotes {
		errs.add("notes", "too_long", "notes can be at most %d characters (got %d)", MaxNotes, n)
	}
	for _, r := range s {
		if r != '\n' && r != '\t' && unicode.IsControl(r) {
			errs.add("notes", "invalid", "notes cannot contain control characters")
			return
		}
	}
}
