package validate

import (
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"corvus/control/internal/netcfg"
	"corvus/control/internal/store"
)

// Limits for security rules.
const (
	MaxRules         = 200
	MaxGroups        = 10
	MaxConditions    = 10
	MaxValueLen      = 1024
	MaxValues        = 1000
	MaxRegexLen      = 512
	MaxIPListItems   = 50000
	MaxUAListItems   = 10000 // User-Agent lists are matched by a linear scan
	MaxUAEntryLen    = 256
	MinUAEntryLen    = 3 // shorter substrings match far too much
	MinListRefreshS  = 300
	MaxListRefreshS  = 7 * 86400
	DefListRefreshS  = 3600
	MaxListURLLen    = 2048
	maxRuleNameRunes = 100
)

var stringOps = map[string]bool{
	"eq": true, "ne": true, "contains": true, "not_contains": true, "starts_with": true, "ends_with": true,
	"matches": true, "not_matches": true, "in": true, "not_in": true,
}

var setOps = map[string]bool{"in": true, "not_in": true}

// RuleFields lists the fields a condition may test and whether exists/not_exists apply.
var RuleFields = map[string]struct{ set, str, exists bool }{
	"ip.src":          {set: true},
	"ip.country":      {set: true},
	"http.method":     {set: true},
	"http.scheme":     {set: true},
	"http.host":       {str: true},
	"http.path":       {str: true},
	"http.query":      {str: true},
	"http.user_agent": {str: true, exists: true},
	// User-Agent contains (case-insensitive) any entry of a User-Agent list.
	"http.user_agent_list": {set: true},
	"http.referer":         {str: true, exists: true},
	"http.header":          {str: true, exists: true},
	"http.body":            {str: true},
}

// bodyOps: request bodies are matched as normalised text (see gateway corvus/body.lua).
var bodyOps = map[string]bool{"contains": true, "not_contains": true, "matches": true, "not_matches": true}

var (
	headerNameRe = regexp.MustCompile(`^[A-Za-z0-9-]{1,64}$`)
	countryRe    = regexp.MustCompile(`^[A-Z]{2}$`)
	methodRe     = regexp.MustCompile(`^[A-Z]{1,16}$`)
)

// DisplayName validates a free-form display name (any language, no control characters).
func DisplayName(s string, max int) bool {
	if s == "" || utf8.RuneCountInString(s) > max || !utf8.ValidString(s) {
		return false
	}
	for _, r := range s {
		if unicode.IsControl(r) {
			return false
		}
	}
	return true
}

// SecurityRule validates and normalises a rule. listKind returns the kind of a list id
// ("" when it does not exist; nil skips the check); hostExists reports whether a proxy
// host id exists.
func SecurityRule(r *store.SecurityRule, listKind func(int64) string, hostExists func(int64) bool) error {
	var errs Errors
	r.Name = strings.TrimSpace(r.Name)
	r.Description = strings.TrimSpace(r.Description)
	if !DisplayName(r.Name, maxRuleNameRunes) {
		errs.add("name", "invalid", "name must be 1-100 characters")
	}
	if utf8.RuneCountInString(r.Description) > 500 || strings.ContainsAny(r.Description, "\x00") {
		errs.add("description", "invalid", "description must be at most 500 characters")
	}
	switch r.Action {
	case "block":
		switch r.StatusCode {
		case 0:
			r.StatusCode = 403
		case 401, 403, 404, 410, 429, 444:
		default:
			errs.add("status_code", "invalid", "status code must be 401, 403, 404, 410, 429 or 444")
		}
		r.SkipBasicAuth = false
	case "allow":
		r.StatusCode = 403
	case "log", "challenge":
		r.StatusCode, r.SkipBasicAuth = 403, false
	default:
		errs.add("action", "invalid", "action must be block, allow, log or challenge")
	}
	challengeSettings(r, &errs)
	switch r.Scope {
	case "", "all":
		r.Scope, r.HostIDs = "all", []int64{}
	case "hosts":
		if len(r.HostIDs) == 0 {
			errs.add("host_ids", "required", "select at least one proxy host")
		}
		seen := map[int64]bool{}
		ids := []int64{}
		for _, h := range r.HostIDs {
			if hostExists != nil && !hostExists(h) {
				errs.add("host_ids", "not_found", "proxy host %d does not exist", h)
			} else if !seen[h] {
				seen[h] = true
				ids = append(ids, h)
			}
		}
		r.HostIDs = ids
	default:
		errs.add("scope", "invalid", "scope must be all or hosts")
	}
	groups := r.Expression.Groups
	if len(groups) == 0 {
		errs.add("expression", "required", "add at least one condition")
	}
	if len(groups) > MaxGroups {
		errs.add("expression", "too_many", "at most %d OR groups", MaxGroups)
	}
	for gi := range groups {
		if len(groups[gi]) == 0 {
			errs.add("expression", "invalid", "empty condition group")
		}
		if len(groups[gi]) > MaxConditions {
			errs.add("expression", "too_many", "at most %d conditions per group", MaxConditions)
		}
		for ci := range groups[gi] {
			if err := condition(&groups[gi][ci], listKind); err != nil {
				errs.add(fmt.Sprintf("expression.groups.%d.%d", gi, ci), "invalid", "%s", err.Error())
			}
		}
	}
	return errs.OrNil()
}

// Challenge limits: proof-of-work difficulty in leading zero bits (each bit doubles
// the browser's work: 16 ≈ 65k hashes, 24 ≈ 17M) and the pass cookie lifetime.
const (
	MinChallengeDifficulty = 12
	MaxChallengeDifficulty = 24
	MinChallengeTTL        = 300
	MaxChallengeTTL        = 7 * 86400
)

// challengeSettings validates the challenge fields of a challenge rule and resets them
// to the defaults for other actions.
func challengeSettings(r *store.SecurityRule, errs *Errors) {
	if r.Action != "challenge" {
		r.ChallengeMode, r.ChallengeDifficulty = store.DefaultChallengeMode, store.DefaultChallengeDifficulty
		r.ChallengeTTL, r.ChallengeBind = store.DefaultChallengeTTL, store.DefaultChallengeBind
		return
	}
	switch r.ChallengeMode {
	case "":
		r.ChallengeMode = store.DefaultChallengeMode
	case "js", "cookie":
	default:
		errs.add("challenge_mode", "invalid", "challenge mode must be js or cookie")
	}
	if r.ChallengeDifficulty == 0 || r.ChallengeMode == "cookie" {
		r.ChallengeDifficulty = store.DefaultChallengeDifficulty // not used in cookie mode
	}
	if r.ChallengeDifficulty < MinChallengeDifficulty || r.ChallengeDifficulty > MaxChallengeDifficulty {
		errs.add("challenge_difficulty", "invalid", "difficulty must be %d-%d bits", MinChallengeDifficulty, MaxChallengeDifficulty)
	}
	if r.ChallengeTTL == 0 {
		r.ChallengeTTL = store.DefaultChallengeTTL
	}
	if r.ChallengeTTL < MinChallengeTTL || r.ChallengeTTL > MaxChallengeTTL {
		errs.add("challenge_ttl_s", "invalid", "challenge lifetime must be %d-%d seconds (5 minutes to 7 days)", MinChallengeTTL, MaxChallengeTTL)
	}
	switch r.ChallengeBind {
	case "":
		r.ChallengeBind = store.DefaultChallengeBind
	case "ip", "ip_ua":
	default:
		errs.add("challenge_bind", "invalid", "challenge binding must be ip or ip_ua")
	}
}

func condition(c *store.Condition, listKind func(int64) string) error {
	f, ok := RuleFields[c.Field]
	if !ok {
		return fmt.Errorf("unknown field %q", c.Field)
	}
	if c.Field == "http.header" {
		c.Name = strings.ToLower(strings.TrimSpace(c.Name))
		if !headerNameRe.MatchString(c.Name) {
			return fmt.Errorf("a valid header name is required")
		}
	} else {
		c.Name = ""
	}
	switch {
	case (c.Operator == "exists" || c.Operator == "not_exists") && f.exists:
		c.Value = []string{}
		return nil
	case f.set && !setOps[c.Operator], f.str && !stringOps[c.Operator], c.Field == "http.body" && !bodyOps[c.Operator]:
		return fmt.Errorf("operator %q is not valid for %s", c.Operator, c.Field)
	}
	vals := make([]string, 0, len(c.Value))
	for _, v := range c.Value {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if len(v) > MaxValueLen || strings.ContainsAny(v, "\x00\r\n") {
			return fmt.Errorf("values must be single-line and at most %d bytes", MaxValueLen)
		}
		vals = append(vals, v)
	}
	if len(vals) == 0 {
		return fmt.Errorf("a value is required")
	}
	if len(vals) > MaxValues {
		return fmt.Errorf("at most %d values", MaxValues)
	}
	if !setOps[c.Operator] && len(vals) != 1 {
		return fmt.Errorf("operator %q takes exactly one value", c.Operator)
	}
	switch c.Field {
	case "ip.src":
		var cidrs []string
		for _, v := range vals {
			if id, ok := store.ListRef(v); ok {
				if err := checkListRef(id, store.ListKindIP, listKind); err != nil {
					return err
				}
				continue
			}
			cidrs = append(cidrs, v)
		}
		if _, err := netcfg.NormalizeCIDRs(cidrs); err != nil {
			return err
		}
	case "http.user_agent_list":
		for _, v := range vals {
			id, ok := store.ListRef(v)
			if !ok {
				return fmt.Errorf("select a User-Agent list")
			}
			if err := checkListRef(id, store.ListKindUserAgent, listKind); err != nil {
				return err
			}
		}
	case "ip.country":
		for i, v := range vals {
			vals[i] = strings.ToUpper(v)
			if !countryRe.MatchString(vals[i]) {
				return fmt.Errorf("invalid country code %q", v)
			}
		}
	case "http.method":
		for i, v := range vals {
			vals[i] = strings.ToUpper(v)
			if !methodRe.MatchString(vals[i]) {
				return fmt.Errorf("invalid method %q", v)
			}
		}
	case "http.scheme":
		for i, v := range vals {
			vals[i] = strings.ToLower(v)
			if vals[i] != "http" && vals[i] != "https" {
				return fmt.Errorf("scheme must be http or https")
			}
		}
	}
	if c.Operator == "matches" || c.Operator == "not_matches" {
		if err := RuleRegex(vals[0]); err != nil {
			return err
		}
	}
	c.Value = vals
	return nil
}

func checkListRef(id int64, want string, listKind func(int64) string) error {
	if listKind == nil {
		return nil
	}
	switch k := listKind(id); k {
	case "":
		return fmt.Errorf("list %d does not exist", id)
	case want:
		return nil
	default:
		if want == store.ListKindIP {
			return fmt.Errorf("list %d is not an IP list", id)
		}
		return fmt.Errorf("list %d is not a User-Agent list", id)
	}
}

// RuleRegex accepts patterns that are valid RE2 (no backreferences or lookaround), which
// PCRE also understands. Backtracking at runtime is additionally capped by
// lua_regex_match_limit in the gateway.
func RuleRegex(p string) error {
	if len(p) > MaxRegexLen {
		return fmt.Errorf("regular expression must be at most %d characters", MaxRegexLen)
	}
	// \C (single byte) and (*VERB) control verbs are PCRE-only; RE2 rejects the rest.
	if strings.Contains(p, `\C`) || strings.Contains(p, "(*") {
		return fmt.Errorf("unsupported regular expression construct")
	}
	if _, err := regexp.Compile(p); err != nil {
		return fmt.Errorf("invalid regular expression: %v", err)
	}
	return nil
}

// IPList validates and normalises a list (either kind) and its feed settings.
func IPList(l *store.IPList) error {
	var errs Errors
	l.Name = strings.TrimSpace(l.Name)
	l.Description = strings.TrimSpace(l.Description)
	if !DisplayName(l.Name, 64) || strings.ContainsAny(l.Name, `$"\`) {
		errs.add("name", "invalid", "name must be 1-64 characters (no $ \" \\)")
	}
	if utf8.RuneCountInString(l.Description) > 500 {
		errs.add("description", "invalid", "description must be at most 500 characters")
	}
	if l.Kind == "" {
		l.Kind = store.ListKindIP
	}
	l.SourceURL = strings.TrimSpace(l.SourceURL)
	if l.SourceURL != "" {
		if err := FeedURL(l.SourceURL); err != nil {
			errs.add("source_url", "invalid", "%s", err.Error())
		}
		if l.RefreshS == 0 {
			l.RefreshS = DefListRefreshS
		}
		if l.RefreshS < MinListRefreshS || l.RefreshS > MaxListRefreshS {
			errs.add("refresh_s", "invalid", "refresh interval must be %d seconds to 7 days", MinListRefreshS)
		}
	} else if l.RefreshS == 0 {
		l.RefreshS = DefListRefreshS
	}
	switch l.Kind {
	case store.ListKindIP:
		if len(l.Items) > MaxIPListItems {
			errs.add("items", "too_many", "at most %d entries", MaxIPListItems)
		} else if items, err := netcfg.NormalizeCIDRs(l.Items); err != nil {
			errs.add("items", "invalid", "%s", err.Error())
		} else {
			l.Items = items
		}
	case store.ListKindUserAgent:
		if len(l.Items) > MaxUAListItems {
			errs.add("items", "too_many", "at most %d entries", MaxUAListItems)
		} else if items, err := NormalizeUAEntries(l.Items); err != nil {
			errs.add("items", "invalid", "%s", err.Error())
		} else {
			l.Items = items
		}
	default:
		errs.add("kind", "invalid", "kind must be ip or user_agent")
	}
	return errs.OrNil()
}

// UAEntry validates one User-Agent list entry: a plain, case-insensitive substring
// (no wildcards or regular expressions), 3-256 bytes, printable.
func UAEntry(v string) error {
	if len(v) < MinUAEntryLen || len(v) > MaxUAEntryLen {
		return fmt.Errorf("User-Agent entry %q must be %d-%d bytes", v, MinUAEntryLen, MaxUAEntryLen)
	}
	if !utf8.ValidString(v) {
		return fmt.Errorf("User-Agent entry %q is not valid UTF-8", v)
	}
	for _, r := range v {
		if unicode.IsControl(r) {
			return fmt.Errorf("User-Agent entry %q contains control characters", v)
		}
	}
	return nil
}

// NormalizeUAEntries trims, validates and de-duplicates (case-insensitively) entries.
func NormalizeUAEntries(in []string) ([]string, error) {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range in {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if err := UAEntry(v); err != nil {
			return nil, err
		}
		k := strings.ToLower(v)
		if !seen[k] {
			seen[k] = true
			out = append(out, v)
		}
	}
	return out, nil
}

// FeedURL checks the syntax of a list feed URL: https only, a host name, no credentials.
// Where it may point is checked again when connecting (see internal/feeds).
func FeedURL(raw string) error {
	if len(raw) > MaxListURLLen {
		return fmt.Errorf("URL must be at most %d characters", MaxListURLLen)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" || u.Hostname() == "" {
		return fmt.Errorf("invalid URL")
	}
	if u.Scheme != "https" {
		return fmt.Errorf("only https:// URLs are allowed")
	}
	if u.User != nil {
		return fmt.Errorf("credentials in the URL are not allowed")
	}
	if strings.ContainsAny(raw, " \t\r\n") {
		return fmt.Errorf("invalid URL")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil && !PublicIP(ip) {
		return fmt.Errorf("the URL must point to a public address")
	}
	if h := strings.ToLower(u.Hostname()); h == "localhost" || strings.HasSuffix(h, ".localhost") {
		return fmt.Errorf("the URL must point to a public address")
	}
	return nil
}

// PublicIP reports whether ip is a globally routable unicast address (not loopback,
// private, link-local, CGNAT, unspecified, multicast or documentation/benchmark ranges).
func PublicIP(ip net.IP) bool {
	if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() ||
		ip.IsInterfaceLocalMulticast() || ip.IsMulticast() || ip.IsUnspecified() {
		return false
	}
	for _, n := range nonPublic {
		if n.Contains(ip) {
			return false
		}
	}
	return true
}

var nonPublic = func() []*net.IPNet {
	var out []*net.IPNet
	for _, c := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15",
		"198.51.100.0/24", "203.0.113.0/24", "240.0.0.0/4", "100::/64", "2001:db8::/32"} {
		_, n, _ := net.ParseCIDR(c)
		out = append(out, n)
	}
	return out
}()
