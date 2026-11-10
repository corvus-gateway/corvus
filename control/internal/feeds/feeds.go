// Package feeds keeps lists (IP addresses or User-Agent substrings) in sync with a
// URL: one entry per line, fetched periodically over https. A failed fetch keeps the
// last good entries; the gateway configuration is redeployed only when they change.
package feeds

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"corvus/control/internal/netcfg"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// MaxFeedBytes is the largest feed accepted.
const MaxFeedBytes = 5 << 20

// failRetry is how soon a failed feed is retried (at most its refresh interval).
const failRetry = 15 * time.Minute

// Service refreshes every list with a source URL.
type Service struct {
	Store *store.Store
	// Apply runs mutate in a transaction and redeploys the gateway (deploy.Deployer.Apply).
	Apply func(ctx context.Context, message string, mutate func(tx *store.Store) error) error
	// Client fetches feeds; nil uses SafeClient (public https targets only).
	Client *http.Client
	Now    func() time.Time

	mu       sync.Mutex // one refresh at a time
	kickOnce sync.Once
	kick     chan int64
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now().UTC().Truncate(time.Second)
	}
	return time.Now().UTC().Truncate(time.Second)
}

func (s *Service) client() *http.Client {
	if s.Client != nil {
		return s.Client
	}
	return SafeClient()
}

func (s *Service) kicks() chan int64 {
	s.kickOnce.Do(func() { s.kick = make(chan int64, 16) })
	return s.kick
}

// Kick asks Run to refresh list id soon (after it was created or its URL changed).
func (s *Service) Kick(id int64) {
	select {
	case s.kicks() <- id:
	default:
	}
}

// Run refreshes due feeds every minute until ctx ends.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	first := time.After(20 * time.Second)
	for {
		select {
		case <-ctx.Done():
			return
		case id := <-s.kicks():
			if _, err := s.Refresh(ctx, id); err != nil && ctx.Err() == nil {
				slog.Warn("list feed refresh failed", "list", id, "err", err)
			}
		case <-first:
			s.refreshDue(ctx)
		case <-t.C:
			s.refreshDue(ctx)
		}
	}
}

func (s *Service) refreshDue(ctx context.Context) {
	lists, err := s.Store.ListIPLists(ctx)
	if err != nil {
		return
	}
	now := s.now()
	for _, l := range lists {
		if !Due(l, now) {
			continue
		}
		if _, err := s.Refresh(ctx, l.ID); err != nil && ctx.Err() == nil {
			slog.Warn("list feed refresh failed", "list", l.Name, "err", err)
		}
	}
}

// Due reports whether a feed list should be fetched now.
func Due(l store.IPList, now time.Time) bool {
	if l.SourceURL == "" {
		return false
	}
	if l.CheckedAt == nil {
		return true
	}
	every := time.Duration(l.RefreshS) * time.Second
	if every < time.Duration(validate.MinListRefreshS)*time.Second {
		every = time.Duration(validate.DefListRefreshS) * time.Second
	}
	if l.FetchError != "" && every > failRetry {
		every = failRetry
	}
	return now.Sub(*l.CheckedAt) >= every
}

// ErrNoFeed is returned by Refresh for a list without a source URL.
var ErrNoFeed = errors.New("this list has no source URL")

// Refresh fetches list id now. It returns the updated list; a fetch error is also
// recorded in the list (FetchError) while the previous entries stay in effect.
func (s *Service) Refresh(ctx context.Context, id int64) (*store.IPList, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	l, err := s.Store.IPListByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if l.SourceURL == "" {
		return l, ErrNoFeed
	}
	src := l.SourceURL
	fctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	items, warn, ferr := Fetch(fctx, s.client(), src, l.Kind)
	cancel()
	now := s.now()
	if ferr != nil {
		if err := s.Store.SetIPListFetch(ctx, id, store.ListFetch{CheckedAt: now, Error: ferr.Error()}); err != nil {
			return nil, err
		}
		l, _ = s.Store.IPListByID(ctx, id)
		return l, ferr
	}
	res := store.ListFetch{Items: items, CheckedAt: now, FetchedAt: &now, Warning: warn}
	if sameSet(items, l.Items) {
		res.Items = nil // unchanged: record the check, no redeploy
		err = s.Store.SetIPListFetch(ctx, id, res)
	} else {
		err = s.Apply(ctx, fmt.Sprintf("list feed updated: %s (%d entries)", l.Name, len(items)), func(tx *store.Store) error {
			cur, err := tx.IPListByID(ctx, id)
			if err != nil {
				return err
			}
			if cur.SourceURL != src || cur.Kind != l.Kind {
				return fmt.Errorf("the list was changed during the fetch")
			}
			return tx.SetIPListFetch(ctx, id, res)
		})
		if err != nil {
			// e.g. the new configuration failed the gateway test: keep the old entries.
			_ = s.Store.SetIPListFetch(ctx, id, store.ListFetch{CheckedAt: now, Error: "apply: " + err.Error()})
		}
	}
	if err != nil {
		l, _ = s.Store.IPListByID(ctx, id)
		return l, err
	}
	return s.Store.IPListByID(ctx, id)
}

func sameSet(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	x := append([]string(nil), a...)
	y := append([]string(nil), b...)
	sort.Strings(x)
	sort.Strings(y)
	for i := range x {
		if x[i] != y[i] {
			return false
		}
	}
	return true
}

// ------------------------------------------------------------------ fetching

// Fetch downloads and parses a feed. warning describes ignored or truncated entries.
func Fetch(ctx context.Context, c *http.Client, src, kind string) (items []string, warning string, err error) {
	// Address checks happen at connect time (SafeClient); the URL itself was validated
	// when the list was saved.
	if u, err := url.Parse(src); err != nil || u.Scheme != "https" {
		return nil, "", errors.New("only https:// URLs are allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "corvus-list-feed")
	req.Header.Set("Accept", "text/plain, */*;q=0.5")
	resp, err := c.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxFeedBytes+1))
	if err != nil {
		return nil, "", err
	}
	if len(data) > MaxFeedBytes {
		return nil, "", fmt.Errorf("feed is larger than %d MB", MaxFeedBytes>>20)
	}
	return Parse(kind, data)
}

// Parse turns a feed body into normalised list entries. Lines that are empty or start
// with # or ; are comments. IP feeds take the first word of each line (an IP or CIDR,
// optionally followed by a comment, as in most blocklists). User-Agent feeds take the
// whole line as a plain substring; backslash escapes (feeds written as nginx regexes,
// e.g. "Indy\ Library") are removed. Invalid lines are skipped and reported in warning;
// a feed without any valid entry is an error. Entries beyond the list limit are dropped.
func Parse(kind string, data []byte) (items []string, warning string, err error) {
	var raw []string
	skipped := 0
	for _, line := range strings.Split(string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		switch kind {
		case store.ListKindUserAgent:
			e := unescape(line)
			if validate.UAEntry(e) != nil {
				skipped++
				continue
			}
			raw = append(raw, e)
		default:
			if i := strings.IndexAny(line, "#;"); i >= 0 {
				line = line[:i]
			}
			f := strings.Fields(line)
			if len(f) == 0 {
				continue
			}
			if !ipOrCIDR(f[0]) {
				skipped++
				continue
			}
			raw = append(raw, f[0])
		}
	}
	max := validate.MaxIPListItems
	if kind == store.ListKindUserAgent {
		max = validate.MaxUAListItems
		items, err = validate.NormalizeUAEntries(raw)
	} else {
		items, err = netcfg.NormalizeCIDRs(raw)
	}
	if err != nil {
		return nil, "", err
	}
	broad := 0
	if kind != store.ListKindUserAgent {
		items, broad = dropBroad(items)
		skipped += broad
	}
	if len(items) == 0 {
		if skipped > 0 {
			return nil, "", fmt.Errorf("no valid entries (%d lines not understood)", skipped)
		}
		return nil, "", fmt.Errorf("the feed is empty")
	}
	var warn []string
	if skipped-broad > 0 {
		warn = append(warn, fmt.Sprintf("%d invalid lines ignored", skipped-broad))
	}
	if broad > 0 {
		warn = append(warn, fmt.Sprintf("%d ranges broader than /%d (IPv4) or /%d (IPv6) ignored", broad, MinFeedPrefixV4, MinFeedPrefixV6))
	}
	if len(items) > max {
		warn = append(warn, fmt.Sprintf("%d entries, only the first %d are used", len(items), max))
		items = items[:max]
	}
	return items, strings.Join(warn, "; "), nil
}

// Shortest prefixes accepted from a feed: a remote list must not be able to cover
// the whole address space (or a large share of it) with a single entry.
const (
	MinFeedPrefixV4 = 8
	MinFeedPrefixV6 = 16
)

// dropBroad removes normalised CIDRs shorter than the minimum feed prefix.
func dropBroad(in []string) ([]string, int) {
	out := in[:0]
	n := 0
	for _, c := range in {
		_, ipn, err := net.ParseCIDR(c)
		if err == nil {
			ones, bits := ipn.Mask.Size()
			if (bits == 32 && ones < MinFeedPrefixV4) || (bits == 128 && ones < MinFeedPrefixV6) {
				n++
				continue
			}
		}
		out = append(out, c)
	}
	if n > 0 {
		slog.Warn("list feed: ignoring very broad ranges", "count", n)
	}
	return out, n
}

func ipOrCIDR(s string) bool {
	if strings.Contains(s, "/") {
		_, _, err := net.ParseCIDR(s)
		return err == nil
	}
	return net.ParseIP(s) != nil
}

func unescape(s string) string {
	if !strings.Contains(s, `\`) {
		return s
	}
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
		}
		b.WriteByte(s[i])
	}
	return strings.TrimSpace(b.String())
}

// ------------------------------------------------------------------ SSRF guard

// SafeClient is an HTTP client that only connects to public addresses: the target is
// checked after DNS resolution, at connect time (so DNS rebinding cannot reach internal
// services), environment proxies are not used and redirects must stay on https.
func SafeClient() *http.Client {
	d := &net.Dialer{Timeout: 10 * time.Second, Control: dialGuard}
	tr := &http.Transport{
		Proxy:                 nil,
		DialContext:           d.DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 20 * time.Second,
		MaxIdleConns:          4,
		IdleConnTimeout:       30 * time.Second,
		ForceAttemptHTTP2:     true,
	}
	return &http.Client{Transport: tr, Timeout: 60 * time.Second, CheckRedirect: checkRedirect}
}

func checkRedirect(req *http.Request, via []*http.Request) error {
	if len(via) >= 5 {
		return errors.New("too many redirects")
	}
	return validate.FeedURL(req.URL.String())
}

// dialGuard runs for every connection attempt with the resolved address.
func dialGuard(network, address string, _ syscall.RawConn) error {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	ip := net.ParseIP(host)
	if ip == nil || !validate.PublicIP(ip) {
		return fmt.Errorf("refusing to connect to non-public address %s", host)
	}
	return nil
}
