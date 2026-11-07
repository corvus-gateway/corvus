package compiler

import "strings"

// topLevelDirective reports whether an advanced configuration sets directive name at its
// own level (not inside a nested block such as a location): only those conflict with the
// directive Corvus would add at the same level. Quotes and comments are skipped.
func topLevelDirective(cfg, name string) bool {
	depth, start := 0, true // start: at the beginning of a statement
	for i := 0; i < len(cfg); i++ {
		c := cfg[i]
		switch {
		case c == '#':
			for i < len(cfg) && cfg[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			for i++; i < len(cfg) && cfg[i] != c; i++ {
				if cfg[i] == '\\' {
					i++
				}
			}
			start = false
		case c == '{':
			depth++
			start = true
		case c == '}':
			depth--
			start = true
		case c == ';':
			start = true
		case c == ' ' || c == '\t' || c == '\n' || c == '\r':
		default:
			if start && depth == 0 && strings.HasPrefix(cfg[i:], name) {
				if j := i + len(name); j == len(cfg) || strings.ContainsRune(" \t\n\r;", rune(cfg[j])) {
					return true
				}
			}
			start = false
		}
	}
	return false
}
