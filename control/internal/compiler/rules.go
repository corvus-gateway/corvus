package compiler

import (
	"context"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"path/filepath"
	"strconv"
	"strings"

	"corvus/control/internal/ipset"
	"corvus/control/internal/netcfg"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// rules.json — read by gateway/lua/corvus/rules.lua at (re)load.
type rulesFile struct {
	Rules     []ruleJSON          `json:"rules"`
	Hosts     map[string]hostJSON `json:"hosts"`
	Geo       string              `json:"geo"`
	BlockPage string              `json:"block_page"`
	ErrorPage string              `json:"error_page"` // upstream errors 502/503/504
	// Page for the "challenge" rule action (built-in only).
	ChallengePage string `json:"challenge_page"`
	// Login page of hosts with a login page (built-in only, corvus/login.lua).
	LoginPage string `json:"login_page"`
	// Request-body signatures from the active rule bundle (empty: built-in list in Lua).
	BodySignatures []sigJSON `json:"body_signatures,omitempty"`
}

type sigJSON struct {
	ID      string `json:"id"`
	Pattern string `json:"pattern"`
	Options string `json:"options"`
}

type ruleJSON struct {
	ID       int64        `json:"id"`
	Action   string       `json:"action"`
	Status   int          `json:"status"`
	SkipAuth bool         `json:"skip_auth"`
	Groups   [][]condJSON `json:"groups"`
	// Only for action "challenge" (gateway corvus/challenge.lua).
	Challenge *challengeJSON `json:"challenge,omitempty"`
}

type challengeJSON struct {
	Mode       string `json:"mode"`       // js | cookie
	Difficulty int    `json:"difficulty"` // leading zero bits (js)
	TTL        int    `json:"ttl"`        // pass cookie lifetime, seconds
	Bind       string `json:"bind"`       // ip | ip_ua
}

type condJSON struct {
	F  string   `json:"f"`
	N  string   `json:"n,omitempty"`
	Op string   `json:"op"`
	V  []string `json:"v"`
	CI bool     `json:"ci,omitempty"`
	// ip.src: IP lists, as range files mapped by the gateway (corvus/iplist.lua). The
	// condition holds for an address in V or in any of them.
	L []listFileJSON `json:"l,omitempty"`
}

// listFileJSON refers to a range file (internal/ipset) by path and SHA-256: the gateway
// refuses the configuration if the file is missing or does not match.
type listFileJSON struct {
	Path   string `json:"p"`
	SHA256 string `json:"h"`
}

type hostJSON struct {
	Rules    []int  `json:"rules"` // 1-based indexes into Rules, in evaluation order
	CF       bool   `json:"cf"`
	Exploits bool   `json:"exploits"`
	Body     string `json:"body"`     // off | log | block
	BodyMax  int    `json:"body_max"` // bytes
	// "Under attack" challenge settings (proxy hosts whose mode is not off). On: always
	// active; otherwise active while corvus-control has pushed an auto activation for
	// the host (corvus/underattack.lua).
	UnderAttack *underAttackJSON `json:"under_attack,omitempty"`
	// Login page (#294): only for hosts that have it on. Never carries the PIN hash: the
	// gateway asks corvus-control to check credentials (internal/signin).
	Login *loginJSON `json:"login,omitempty"`
}

type loginJSON struct {
	Mode     string   `json:"mode"`      // pin | users
	Hours    int      `json:"hours"`     // session lifetime
	Exclude  []string `json:"exclude"`   // path prefixes that need no sign-in
	PassUser bool     `json:"pass_user"` // send X-Corvus-User (users mode)
}

type underAttackJSON struct {
	On bool `json:"on"`
	challengeJSON
}

// GeoFile is where the country database lives (written by internal/geoip).
func (p Paths) GeoFile() string {
	return filepath.ToSlash(filepath.Join(p.DataDir, "geoip", "country.bin"))
}

// redirectRuleKey is the rules.json key of a redirection host (proxy hosts use their id).
func redirectRuleKey(id int64) string { return "r" + strconv.FormatInt(id, 10) }

func (c *Compiler) rulePolicy(ctx context.Context, s *store.Store, hosts []store.ProxyHost, redirects []store.RedirectionHost) (*rulesFile, error) {
	rules, err := s.ListSecurityRules(ctx)
	if err != nil {
		return nil, err
	}
	lists, err := s.ListIPLists(ctx)
	if err != nil {
		return nil, err
	}
	listByID := map[int64][]string{}
	listKind := map[int64]string{}
	ipLists := &listFiles{c: c, lists: map[int64]store.IPList{}, refs: map[int64]listFileJSON{}}
	for _, l := range lists {
		lc := l
		if err := validate.IPList(&lc); err != nil {
			return nil, fmt.Errorf("list %d: %w", l.ID, err)
		}
		listByID[l.ID], listKind[l.ID] = lc.Items, lc.Kind
		ipLists.lists[l.ID] = lc
	}
	hostIDs := map[int64]bool{}
	for _, h := range hosts {
		hostIDs[h.ID] = true
	}
	out := &rulesFile{Rules: []ruleJSON{}, Hosts: map[string]hostJSON{}, Geo: c.P.GeoFile(),
		BlockPage:     filepath.ToSlash(filepath.Join(c.P.HTMLDir, "blocked.html")),
		ErrorPage:     filepath.ToSlash(filepath.Join(c.P.HTMLDir, "error.html")),
		ChallengePage: filepath.ToSlash(filepath.Join(c.P.HTMLDir, "challenge.html")),
		LoginPage:     filepath.ToSlash(filepath.Join(c.P.HTMLDir, "login.html"))}
	if c.Bundle != nil {
		if b := c.Bundle(); b != nil {
			for _, sg := range b.Signatures {
				out.BodySignatures = append(out.BodySignatures, sigJSON{ID: sg.ID, Pattern: sg.Pattern, Options: sg.Options})
			}
		}
	}
	type scoped struct {
		all   bool
		hosts map[int64]bool
	}
	var scopes []scoped
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		// A host-scoped rule whose hosts were all deleted applies nowhere: skip it
		// rather than failing every deployment.
		if r.Scope == "hosts" {
			var live []int64
			for _, h := range r.HostIDs {
				if hostIDs[h] {
					live = append(live, h)
				}
			}
			if len(live) == 0 {
				continue
			}
			r.HostIDs = live
		}
		rc := r
		// Defense in depth: never render a rule that would not pass validation.
		if err := validate.SecurityRule(&rc, func(id int64) string { return listKind[id] }, func(id int64) bool { return hostIDs[id] }); err != nil {
			return nil, fmt.Errorf("security rule %d: %w", r.ID, err)
		}
		rj := ruleJSON{ID: rc.ID, Action: rc.Action, Status: rc.StatusCode, SkipAuth: rc.SkipBasicAuth}
		if rc.Action == "challenge" {
			rj.Challenge = &challengeJSON{Mode: rc.ChallengeMode, Difficulty: rc.ChallengeDifficulty, TTL: rc.ChallengeTTL, Bind: rc.ChallengeBind}
		}
		for _, g := range rc.Expression.Groups {
			var cg []condJSON
			for _, cond := range g {
				cj := condJSON{F: cond.Field, N: cond.Name, Op: cond.Operator, V: cond.Value, CI: cond.CI}
				if cond.Field == "ip.src" {
					nets, files, err := ipLists.encode(cond.Value)
					if err != nil {
						return nil, fmt.Errorf("security rule %d: %w", r.ID, err)
					}
					cj.V, cj.L = nets, files
				}
				if cond.Field == "http.user_agent_list" {
					cj.V = uaEntries(cond.Value, listByID)
				}
				cg = append(cg, cj)
			}
			rj.Groups = append(rj.Groups, cg)
		}
		out.Rules = append(out.Rules, rj)
		sc := scoped{all: rc.Scope == "all", hosts: map[int64]bool{}}
		for _, h := range rc.HostIDs {
			sc.hosts[h] = true
		}
		scopes = append(scopes, sc)
	}
	for _, h := range hosts {
		if !h.Enabled {
			continue
		}
		hj := hostJSON{Rules: []int{}, CF: h.ClientIP == "cloudflare", Exploits: h.BlockExploits, Body: h.BodyInspection, BodyMax: h.BodyMaxKB * 1024}
		if h.UnderAttack == "on" || h.UnderAttack == "auto" {
			uc := h.UnderAttackConfig
			validate.UnderAttackDefaults(&uc)
			hj.UnderAttack = &underAttackJSON{On: h.UnderAttack == "on",
				challengeJSON: challengeJSON{Mode: uc.Mode, Difficulty: uc.Difficulty, TTL: uc.TTL, Bind: uc.Bind}}
		}
		if h.Login.Enabled {
			hj.Login = hostLoginJSON(h.Login)
		}
		for i, sc := range scopes {
			if sc.all || sc.hosts[h.ID] {
				hj.Rules = append(hj.Rules, i+1)
			}
		}
		out.Hosts[strconv.FormatInt(h.ID, 10)] = hj
	}
	// Redirection hosts get bans, rules that apply to all hosts and exploit blocking;
	// host-scoped rules target proxy hosts only.
	for _, h := range redirects {
		if !h.Enabled {
			continue
		}
		hj := hostJSON{Rules: []int{}, Exploits: h.BlockExploits, Body: "off", BodyMax: 1 << 20}
		for i, sc := range scopes {
			if sc.all {
				hj.Rules = append(hj.Rules, i+1)
			}
		}
		out.Hosts[redirectRuleKey(h.ID)] = hj
	}
	return out, nil
}

// hostLoginJSON is a host's login page for the gateway (defaults filled in).
func hostLoginJSON(l store.HostLogin) *loginJSON {
	hours := l.SessionHours
	if hours <= 0 {
		hours = validate.DefaultSessionHours
	}
	ex := append([]string{}, l.ExcludePaths...)
	return &loginJSON{Mode: l.Mode, Hours: hours, Exclude: ex, PassUser: l.Mode == store.LoginUsers && l.PassUser}
}

// uaEntries expands User-Agent list references into lower-cased, de-duplicated
// substrings (matched case-insensitively by the gateway, see corvus/ualist.lua).
func uaEntries(values []string, lists map[int64][]string) []string {
	out := []string{}
	seen := map[string]bool{}
	for _, v := range values {
		id, ok := store.ListRef(v)
		if !ok {
			continue
		}
		for _, e := range lists[id] {
			e = strings.ToLower(e)
			if !seen[e] {
				seen[e] = true
				out = append(out, e)
			}
		}
	}
	return out
}

// listFiles turns the IP lists referenced by rules into range files (internal/ipset),
// once per list and rendering.
type listFiles struct {
	c     *Compiler
	lists map[int64]store.IPList // validated
	refs  map[int64]listFileJSON
}

// ListDir is the directory of the range files (<DataDir>/lists).
func (p Paths) ListDir() string { return filepath.Join(filepath.FromSlash(p.DataDir), "lists") }

func (c *Compiler) listDir() *ipset.Dir {
	if c.Lists != nil {
		return c.Lists
	}
	return &ipset.Dir{Path: c.P.ListDir()}
}

// encode splits the values of an ip.src condition into networks written in the rule
// (see encodeNets) and the range files of the lists it references.
func (lf *listFiles) encode(values []string) ([]string, []listFileJSON, error) {
	var inline []string
	var files []listFileJSON
	seen := map[int64]bool{}
	for _, v := range values {
		id, ok := store.ListRef(v)
		if !ok {
			inline = append(inline, v)
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ref, err := lf.file(id)
		if err != nil {
			return nil, nil, err
		}
		files = append(files, ref)
	}
	nets, err := encodeNets(inline)
	return nets, files, err
}

// file returns the range file of IP list id: the stored file of an IP feed, or one
// written from the entries of a list kept in the database. A feed whose file is missing
// or damaged (for example after restoring a backup, which does not carry them) is used
// as an empty list, as before its first download; the feeds service downloads it again
// at once and redeploys.
func (lf *listFiles) file(id int64) (listFileJSON, error) {
	if ref, ok := lf.refs[id]; ok {
		return ref, nil
	}
	l, ok := lf.lists[id]
	if !ok {
		return listFileJSON{}, fmt.Errorf("IP list %d does not exist", id)
	}
	dir := lf.c.listDir()
	var set *ipset.Set
	hash := ""
	if l.DataHash != "" {
		if _, err := dir.Check(l.DataHash); err == nil {
			hash = l.DataHash
			dir.Touch(hash) // in use: a concurrent cleanup must spare it
		} else {
			slog.Warn("IP list entries unavailable, the list is empty until it is downloaded again", "list", l.Name, "err", err)
			set = &ipset.Set{}
		}
	} else {
		var err error
		if set, err = ipset.FromEntries(l.Items); err != nil {
			return listFileJSON{}, fmt.Errorf("IP list %d: %w", id, err)
		}
	}
	if hash == "" {
		info, err := dir.Write(set)
		if err != nil {
			return listFileJSON{}, fmt.Errorf("IP list %d: %w", id, err)
		}
		hash = info.Hash
	}
	ref := listFileJSON{Path: filepath.ToSlash(dir.File(hash)), SHA256: hash}
	lf.refs[id] = ref
	return ref, nil
}

// encodeNets encodes networks entered in a condition as "<hex network bytes>/<prefix>"
// (IPv4: 8 hex digits, IPv6: 32) for the Lua matcher.
func encodeNets(raw []string) ([]string, error) {
	cidrs, err := netcfg.NormalizeCIDRs(raw)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		_, n, err := net.ParseCIDR(c)
		if err != nil {
			return nil, err
		}
		ones, _ := n.Mask.Size()
		ip := n.IP
		if v4 := ip.To4(); v4 != nil {
			ip = v4
		}
		out = append(out, hex.EncodeToString(ip)+"/"+strconv.Itoa(ones))
	}
	return out, nil
}
