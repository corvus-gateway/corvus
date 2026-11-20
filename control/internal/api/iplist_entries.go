package api

import (
	"errors"
	"net/http"
	"net/netip"
	"strconv"
	"strings"

	"corvus/control/internal/ipset"
	"corvus/control/internal/store"
)

// Bounded views of an IP list for the console: lists fed from a URL may hold millions of
// entries, kept in a range file (internal/ipset) rather than in the database, so the
// console never loads them whole.

const (
	defPreview = 100
	maxPreview = 1000
)

func (s *Server) listDir() *ipset.Dir {
	if s.Deployer != nil && s.Deployer.Compiler != nil && s.Deployer.Compiler.Lists != nil {
		return s.Deployer.Compiler.Lists
	}
	return &ipset.Dir{Path: s.Paths.ListDir()}
}

// markMissing flags file-backed lists whose range file is missing or damaged (they are
// used as empty until downloaded again, see internal/feeds).
func (s *Server) markMissing(lists []store.IPList) {
	dir := s.listDir()
	for i := range lists {
		if lists[i].DataHash != "" {
			if _, err := dir.Check(lists[i].DataHash); err != nil {
				lists[i].DataMissing = true
			}
		}
	}
}

type listEntries struct {
	Entries   []string `json:"entries"`
	Total     int      `json:"total"`  // entries of the list
	Ranges    int      `json:"ranges"` // merged ranges (file-backed lists; entries are shown as ranges)
	Truncated bool     `json:"truncated"`
	Missing   bool     `json:"missing,omitempty"`
}

// ipListEntries returns the first entries of a list (?limit=, at most 1000). For a
// file-backed list they are its merged ranges (an address, a CIDR or "first-last").
func (s *Server) ipListEntries(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	limit := defPreview
	if v := r.URL.Query().Get("limit"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 {
			writeErr(w, http.StatusBadRequest, "bad_request", "invalid limit")
			return
		}
		limit = min(n, maxPreview)
	}
	l, err := s.Store.IPListByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	out := listEntries{Total: l.ItemCount, Ranges: l.RangeCount, Entries: []string{}}
	if l.DataHash == "" {
		out.Entries = l.Items[:min(limit, len(l.Items))]
		out.Truncated = len(l.Items) > limit
		writeJSON(w, 200, out)
		return
	}
	f, err := s.listDir().Open(l.DataHash)
	if err != nil {
		out.Missing = true
		writeJSON(w, 200, out)
		return
	}
	defer f.Close()
	if out.Entries, err = f.Preview(limit); err != nil {
		fail(w, err)
		return
	}
	out.Truncated = f.Header().N4+f.Header().N6 > limit
	writeJSON(w, 200, out)
}

// ipListCheck reports whether ?ip= is in an IP list, as the gateway would match it.
func (s *Server) ipListCheck(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	addr, err := netip.ParseAddr(strings.TrimSpace(r.URL.Query().Get("ip")))
	if err != nil || addr.Zone() != "" {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid IP address")
		return
	}
	addr = addr.Unmap() // the gateway sees IPv4 clients as IPv4 addresses
	l, err := s.Store.IPListByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if l.Kind != store.ListKindIP {
		writeErr(w, http.StatusConflict, "conflict", "not an IP address list")
		return
	}
	res := map[string]any{"ip": addr.String(), "match": false}
	var match bool
	var rng string
	if l.DataHash == "" {
		set, err := ipset.FromEntries(l.Items)
		if err != nil {
			fail(w, err)
			return
		}
		match, rng = set.Contains(addr)
	} else {
		f, err := s.listDir().Open(l.DataHash)
		if errors.Is(err, ipset.ErrMissing) {
			res["missing"] = true
			writeJSON(w, 200, res)
			return
		}
		if err != nil {
			fail(w, err)
			return
		}
		defer f.Close()
		if match, rng, err = f.Contains(addr); err != nil {
			fail(w, err)
			return
		}
	}
	res["match"] = match
	if match {
		res["range"] = rng
	}
	writeJSON(w, 200, res)
}
