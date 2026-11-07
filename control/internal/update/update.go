// Package update checks GitHub for a newer Corvus release, so the console can show
// that an update is available. Only the public "latest release" endpoint is called
// (twice a day); nothing about the installation is sent. CORVUS_UPDATE_CHECK=false
// turns it off.
package update

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
)

// LatestURL is the GitHub API endpoint of the latest (non-pre-release) release.
const LatestURL = "https://api.github.com/repos/corvus-gateway/corvus/releases/latest"

// releasePrefix: release notes links outside it are not passed to the console.
const releasePrefix = "https://github.com/corvus-gateway/corvus/releases/"

// Release is a newer release than the running one.
type Release struct {
	Version string `json:"version"` // e.g. "1.2.1"
	URL     string `json:"url"`     // release notes
}

// Checker polls for the latest release.
type Checker struct {
	Current  string // running version, e.g. "1.2.0"
	URL      string // default LatestURL
	Client   *http.Client
	Interval time.Duration // default 12 h

	newer atomic.Pointer[Release]
}

// Available returns the newer release, or nil.
func (c *Checker) Available() *Release {
	if c == nil {
		return nil
	}
	return c.newer.Load()
}

// Run checks a minute after start and then every Interval until ctx ends.
func (c *Checker) Run(ctx context.Context) {
	if _, ok := parse(c.Current); !ok {
		return // development build: nothing to compare with
	}
	every := c.Interval
	if every <= 0 {
		every = 12 * time.Hour
	}
	wait := time.Minute
	for {
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		if err := c.Check(ctx); err != nil {
			slog.Debug("update check failed", "err", err)
		}
		wait = every
	}
}

// Check asks for the latest release once.
func (c *Checker) Check(ctx context.Context) error {
	url := c.URL
	if url == "" {
		url = LatestURL
	}
	cl := c.Client
	if cl == nil {
		cl = &http.Client{Timeout: 20 * time.Second}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "corvus-awpg")
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &statusError{resp.StatusCode}
	}
	var rel struct {
		Tag        string `json:"tag_name"`
		URL        string `json:"html_url"`
		Draft      bool   `json:"draft"`
		Prerelease bool   `json:"prerelease"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&rel); err != nil {
		return err
	}
	latest, ok := parse(rel.Tag)
	cur, _ := parse(c.Current)
	if !ok || rel.Draft || rel.Prerelease || !less(cur, latest) {
		c.newer.Store(nil)
		return nil
	}
	link := rel.URL
	if !strings.HasPrefix(link, releasePrefix) {
		link = releasePrefix + "latest"
	}
	c.newer.Store(&Release{Version: strings.TrimPrefix(rel.Tag, "v"), URL: link})
	return nil
}

type statusError struct{ code int }

func (e *statusError) Error() string { return "GitHub answered " + strconv.Itoa(e.code) }

// parse reads "v1.2.3" or "1.2.3"; pre-releases ("1.2.0-rc.1") and other forms are
// not compared.
func parse(v string) ([3]int, bool) {
	var out [3]int
	parts := strings.Split(strings.TrimPrefix(strings.TrimSpace(v), "v"), ".")
	if len(parts) != 3 {
		return out, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return out, false
		}
		out[i] = n
	}
	return out, true
}

func less(a, b [3]int) bool {
	for i := range a {
		if a[i] != b[i] {
			return a[i] < b[i]
		}
	}
	return false
}
