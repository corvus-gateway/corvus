package validate

import (
	"fmt"
	"regexp"
	"strings"

	"corvus/control/internal/store"
)

// MaxLocations is the number of custom locations a proxy host may have.
const MaxLocations = 50

// nestedLocationRe finds a location block inside a location snippet (not supported:
// the snippet is inserted into a generated location).
var nestedLocationRe = regexp.MustCompile(`(?m)(?:^|[;{}])\s*location\b`)

// locations normalizes and validates a proxy host's custom locations in place.
func locations(errs *Errors, h *store.ProxyHost) {
	if h.Locations == nil {
		h.Locations = []store.Location{}
	}
	if len(h.Locations) > MaxLocations {
		errs.add("locations", "too_many", "at most %d locations per host", MaxLocations)
		return
	}
	seen := map[string]bool{}
	for i := range h.Locations {
		l := &h.Locations[i]
		prefix := fmt.Sprintf("locations[%d].", i)
		location(errs, l, prefix)
		if seen[l.Path] {
			errs.add(prefix+"path", "duplicate", "path %s is used by another location", l.Path)
		}
		seen[l.Path] = true
		if l.Path == "/" && HasRootLocation(h.AdvancedConfig) {
			errs.add(prefix+"path", "duplicate", "the advanced configuration already defines location /")
		}
	}
}

// Location normalizes and validates a single custom location in place.
func Location(l *store.Location) error {
	var errs Errors
	location(&errs, l, "")
	return errs.OrNil()
}

func location(errs *Errors, l *store.Location, prefix string) {
	f := func(name string) string { return prefix + name }

	l.Path = strings.TrimSpace(l.Path)
	switch {
	case !pathRe.MatchString(l.Path) || strings.Contains(l.Path, "//") || strings.Contains(l.Path, "/../") || strings.HasSuffix(l.Path, "/.."):
		errs.add(f("path"), "invalid", "path must start with / and contain only letters, digits and ._~/-")
	case strings.HasPrefix(l.Path, "/__corvus") || strings.HasPrefix(l.Path, "/.well-known/acme-challenge"):
		errs.add(f("path"), "reserved", "%s is reserved", l.Path)
	}

	l.ForwardScheme = strings.ToLower(strings.TrimSpace(l.ForwardScheme))
	if l.ForwardScheme == "" {
		l.ForwardScheme = "http"
	}
	if l.ForwardScheme != "http" && l.ForwardScheme != "https" {
		errs.add(f("forward_scheme"), "invalid", "scheme must be http or https")
	}
	l.ForwardHost = forwardHost(errs, f("forward_host"), l.ForwardHost)
	if l.ForwardPort < 1 || l.ForwardPort > 65535 {
		errs.add(f("forward_port"), "invalid", "port must be 1-65535")
	}

	l.ForwardPath = strings.TrimSpace(l.ForwardPath)
	if l.ForwardPath != "" {
		if !pathRe.MatchString(l.ForwardPath) || strings.Contains(l.ForwardPath, "/../") || strings.HasSuffix(l.ForwardPath, "/..") {
			errs.add(f("forward_path"), "invalid", "forward path must start with / and contain only letters, digits and ._~/-")
		} else if l.Path == "/" {
			errs.add(f("forward_path"), "unsupported", "location / cannot have a forward path")
		}
	}

	if err := AdvancedConfig(l.AdvancedConfig); err != nil {
		errs.add(f("advanced_config"), "invalid", "%s", err.Error())
	} else if nestedLocationRe.MatchString(l.AdvancedConfig) {
		errs.add(f("advanced_config"), "invalid", "location blocks are not allowed inside a location")
	}
}
