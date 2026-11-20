// Package feeds keeps lists (IP addresses or User-Agent substrings) in sync with a
// URL: one entry per line, fetched periodically over https. A failed fetch keeps the
// last good entries; the gateway configuration is redeployed only when they change.
// IP feeds may hold millions of entries: they are merged into address ranges while
// they download and stored as a range file (internal/ipset), not in the database.
package feeds

import (
	"bufio"
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"runtime/debug"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"

	"corvus/control/internal/ipset"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// Feed size limits. IP feeds are parsed while they download (memory grows with the
// number of entries, not with the size of the download); community blocklists reach
// ~60 MB. User-Agent feeds are small lists matched by a linear scan.
const (
	MaxIPFeedBytes = 64 << 20
	MaxUAFeedBytes = 5 << 20
)

// maxLineBytes: longer lines are not list entries (counted as invalid and skipped).
const maxLineBytes = 4096

// Limits in effect (variables for tests).
var (
	ipFeedBytes = int64(MaxIPFeedBytes)
	uaFeedBytes = int64(MaxUAFeedBytes)
	ipFeedItems = validate.MaxIPFeedItems
)

// fetchTimeout bounds one download (a large list over a slow link).
const fetchTimeout = 3 * time.Minute

// failRetry is how soon a failed feed is retried (at most its refresh interval).
const failRetry = 15 * time.Minute

// Service refreshes every list with a source URL.
type Service struct {
	Store *store.Store
	// Lists stores the entries of IP feeds (range files, shared with the compiler).
	Lists *ipset.Dir
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
		if !Due(l, now) && !s.dataMissing(l, now) {
			continue
		}
		if _, err := s.Refresh(ctx, l.ID); err != nil && ctx.Err() == nil {
			slog.Warn("list feed refresh failed", "list", l.Name, "err", err)
		}
	}
}

// dataMissing reports an IP feed whose range file is missing or damaged (backups do
// not carry them, see internal/backup): it is fetched again at once (after a failed
// attempt, every failRetry). Until then the configuration uses the list as empty.
func (s *Service) dataMissing(l store.IPList, now time.Time) bool {
	if l.SourceURL == "" || l.Kind != store.ListKindIP || l.DataHash == "" || s.Lists == nil {
		return false
	}
	if _, err := s.Lists.Check(l.DataHash); err == nil {
		return false
	}
	return l.FetchError == "" || l.CheckedAt == nil || now.Sub(*l.CheckedAt) >= failRetry
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
	fctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	feed, ferr := Fetch(fctx, s.client(), src, l.Kind)
	cancel()
	now := s.now()
	res := store.ListFetch{CheckedAt: now, FetchedAt: &now}
	unchanged := false
	switch {
	case ferr != nil:
	case feed.IP != nil:
		// IP feed: the entries go to a range file, written before the deployment that
		// uses it (a file nothing uses is removed by the deployer's cleanup).
		res.Data, unchanged, ferr = s.store(l, feed)
		feed.IP = nil
		if feed.Count > 100_000 {
			// give the memory of a large list back to the system (small devices) once done
			defer debug.FreeOSMemory()
		}
	default:
		res.Items = feed.Items
		unchanged = l.DataHash == "" && sameSet(feed.Items, l.Items)
	}
	if ferr != nil {
		if err := s.Store.SetIPListFetch(ctx, id, store.ListFetch{CheckedAt: now, Error: ferr.Error()}); err != nil {
			return nil, err
		}
		l, _ = s.Store.IPListByID(ctx, id)
		return l, ferr
	}
	res.Warning = feed.Warning
	if unchanged {
		res.Items, res.Data = nil, nil // record the check, no redeploy
		err = s.Store.SetIPListFetch(ctx, id, res)
	} else {
		err = s.Apply(ctx, fmt.Sprintf("list feed updated: %s (%d entries)", l.Name, feed.Count), func(tx *store.Store) error {
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

// store writes the ranges of an IP feed. unchanged: the list already uses this file (and
// it is intact: after it went missing the list must be deployed again).
func (s *Service) store(l *store.IPList, feed *Feed) (*store.ListData, bool, error) {
	if s.Lists == nil {
		return nil, false, errors.New("list storage unavailable")
	}
	_, cerr := s.Lists.Check(l.DataHash)
	had := l.DataHash != "" && cerr == nil
	info, err := s.Lists.WriteBuilder(feed.IP)
	if err != nil {
		return nil, false, fmt.Errorf("storing the list: %w", err)
	}
	feed.Count = info.Networks
	return &store.ListData{Hash: info.Hash, Items: info.Networks, Ranges: info.Ranges}, had && info.Hash == l.DataHash, nil
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

// Feed is a parsed feed.
type Feed struct {
	IP      *ipset.Builder // IP feeds: the networks, merged when stored (ipset.Dir.WriteBuilder)
	Items   []string       // User-Agent feeds: normalised entries
	Count   int            // entries used (IP feeds: lines, until merged: then distinct networks)
	Warning string         // ignored or truncated entries
}

// Fetch downloads and parses a feed.
func Fetch(ctx context.Context, c *http.Client, src, kind string) (*Feed, error) {
	// Address checks happen at connect time (SafeClient); the URL itself was validated
	// when the list was saved.
	if u, err := url.Parse(src); err != nil || u.Scheme != "https" {
		return nil, errors.New("only https:// URLs are allowed")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, src, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "corvus-list-feed")
	req.Header.Set("Accept", "text/plain, */*;q=0.5")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	max := ipFeedBytes
	if kind == store.ListKindUserAgent {
		max = uaFeedBytes
	}
	tooLarge := fmt.Errorf("feed is larger than %d MB", max>>20)
	if resp.ContentLength > max {
		return nil, tooLarge
	}
	body := &limitReader{r: resp.Body, left: max}
	var f *Feed
	if kind == store.ListKindUserAgent {
		var data []byte
		if data, err = io.ReadAll(body); err == nil {
			f, err = Parse(kind, data)
		}
	} else {
		f, err = ParseIP(body)
	}
	if body.over {
		return nil, tooLarge
	}
	return f, err
}

// limitReader fails once more than left bytes were read.
type limitReader struct {
	r    io.Reader
	left int64
	over bool
}

func (l *limitReader) Read(p []byte) (int, error) {
	if int64(len(p)) > l.left+1 {
		p = p[:l.left+1]
	}
	n, err := l.r.Read(p)
	l.left -= int64(n)
	if l.left < 0 {
		l.over = true
		return n, errors.New("feed too large")
	}
	return n, err
}

// Parse turns a feed body into list entries; IP feeds are handled by ParseIP. Lines that
// are empty or start with # or ; are comments. User-Agent feeds take the whole line as a
// plain substring; backslash escapes (feeds written as nginx regexes, e.g.
// "Indy\ Library") are removed. Invalid lines are skipped and reported in the warning; a
// feed without any valid entry is an error. Entries beyond the list limit are dropped.
func Parse(kind string, data []byte) (*Feed, error) {
	if kind != store.ListKindUserAgent {
		return ParseIP(bytes.NewReader(data))
	}
	var raw []string
	skipped := 0
	for _, line := range strings.Split(string(bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || line[0] == '#' || line[0] == ';' {
			continue
		}
		e := unescape(line)
		if validate.UAEntry(e) != nil {
			skipped++
			continue
		}
		raw = append(raw, e)
	}
	items, err := validate.NormalizeUAEntries(raw)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, emptyErr(skipped)
	}
	var warn []string
	if skipped > 0 {
		warn = append(warn, fmt.Sprintf("%d invalid lines ignored", skipped))
	}
	if max := validate.MaxUAListItems; len(items) > max {
		warn = append(warn, fmt.Sprintf("%d entries, only the first %d are used", len(items), max))
		items = items[:max]
	}
	return &Feed{Items: items, Count: len(items), Warning: strings.Join(warn, "; ")}, nil
}

func emptyErr(skipped int) error {
	if skipped > 0 {
		return fmt.Errorf("no valid entries (%d lines not understood)", skipped)
	}
	return fmt.Errorf("the feed is empty")
}

// ParseIP reads an IP feed line by line: the first word of a line is an IP address or a
// CIDR (optionally followed by a comment, as in most blocklists); # and ; start comments.
// Entries are collected as address ranges while they are read (the common IPv4 lines
// without allocating), so memory grows with the number of entries (8 bytes per IPv4
// entry, see ipset.Builder), not with the size of the body. Ranges broader than
// MinFeedPrefixV4/V6 and entries beyond validate.MaxIPFeedItems are ignored (and
// reported in the warning).
func ParseIP(r io.Reader) (*Feed, error) {
	b := &ipset.Builder{}
	br := bufio.NewReaderSize(r, maxLineBytes)
	skipped, broad, count, dropped := 0, 0, 0, 0
	first := true
	for {
		line, err := br.ReadSlice('\n')
		if errors.Is(err, bufio.ErrBufferFull) {
			skipped++ // too long for a list entry: skip the rest of the line
			for errors.Is(err, bufio.ErrBufferFull) {
				_, err = br.ReadSlice('\n')
			}
			line = nil
		}
		if err != nil && err != io.EOF {
			return nil, err
		}
		if first {
			line = bytes.TrimPrefix(line, []byte("\xef\xbb\xbf"))
			first = false
		}
		if i := bytes.IndexAny(line, "#;"); i >= 0 {
			line = line[:i]
		}
		if f := firstField(line); len(f) > 0 {
			p, ok, perr := ipset.ParseEntryBytes(f)
			switch {
			case perr != nil:
				skipped++
			case !ok: // an IPv4-mapped range broader than /104 (see ipset.ParseEntry)
			case (p.Addr().Is4() && p.Bits() < MinFeedPrefixV4) || (p.Addr().Is6() && p.Bits() < MinFeedPrefixV6):
				broad++
			case count >= ipFeedItems:
				dropped++
			default:
				b.Add(p)
				count++
			}
		}
		if err == io.EOF {
			break
		}
	}
	if broad > 0 {
		slog.Warn("list feed: ignoring very broad ranges", "count", broad)
	}
	if count == 0 {
		return nil, emptyErr(skipped + broad)
	}
	var warn []string
	if skipped > 0 {
		warn = append(warn, fmt.Sprintf("%d invalid lines ignored", skipped))
	}
	if broad > 0 {
		warn = append(warn, fmt.Sprintf("%d ranges broader than /%d (IPv4) or /%d (IPv6) ignored", broad, MinFeedPrefixV4, MinFeedPrefixV6))
	}
	if dropped > 0 {
		warn = append(warn, fmt.Sprintf("%d entries, only the first %d are used", count+dropped, ipFeedItems))
	}
	return &Feed{IP: b, Count: count, Warning: strings.Join(warn, "; ")}, nil
}

// firstField returns the first word of line (no copy).
func firstField(line []byte) []byte {
	i := 0
	for i < len(line) && isSpace(line[i]) {
		i++
	}
	j := i
	for j < len(line) && !isSpace(line[j]) {
		j++
	}
	return line[i:j]
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\r' || c == '\n' || c == '\v' || c == '\f'
}

// Shortest prefixes accepted from a feed: a remote list must not be able to cover
// the whole address space (or a large share of it) with a single entry.
const (
	MinFeedPrefixV4 = 8
	MinFeedPrefixV6 = 16
)

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
	return &http.Client{Transport: tr, Timeout: fetchTimeout, CheckRedirect: checkRedirect}
}

// TestClient is SafeClient for integration tests (deploy/dev, CORVUS_TEST_FEED_HOSTS):
// connections to the named hosts skip the public-address check and caPEM is trusted in
// addition to the system roots. Never used unless that variable is set.
func TestClient(hosts []string, caPEM []byte) (*http.Client, error) {
	c := SafeClient()
	tr := c.Transport.(*http.Transport)
	allowed := map[string]bool{}
	for _, h := range hosts {
		allowed[strings.ToLower(strings.TrimSpace(h))] = true
	}
	guarded := tr.DialContext
	plain := (&net.Dialer{Timeout: 10 * time.Second}).DialContext
	tr.DialContext = func(ctx context.Context, network, addr string) (net.Conn, error) {
		if h, _, err := net.SplitHostPort(addr); err == nil && allowed[strings.ToLower(h)] {
			return plain(ctx, network, addr)
		}
		return guarded(ctx, network, addr)
	}
	if len(caPEM) > 0 {
		pool, err := x509.SystemCertPool()
		if err != nil {
			pool = x509.NewCertPool()
		}
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, errors.New("no certificate in the test feed CA file")
		}
		tr.TLSClientConfig = &tls.Config{RootCAs: pool, MinVersion: tls.VersionTLS12}
	}
	return c, nil
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
