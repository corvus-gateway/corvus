// Package sigs manages the rule bundle: request-body signatures and virtual-patch
// templates. A default bundle is compiled in; newer bundles are accepted only when
// signed with the Corvus bundle key (Ed25519) and their serial is higher (no downgrade).
package sigs

import (
	"bytes"
	"context"
	"crypto/ed25519"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/hints"
	"corvus/control/internal/store"
)

//go:embed default.json
var defaultBundle []byte

// PublicKey verifies bundle signatures (private key kept offline; see tools/corvus-bundle).
var PublicKey = "un7tFdaUU5/U547lHGyKsekX/+Q1iPnBGMOzLcYmMio="

type Signature struct {
	ID          string `json:"id"`
	Pattern     string `json:"pattern"`
	Options     string `json:"options"`
	Description string `json:"description"`
}

type Template struct {
	ID             string              `json:"id"`
	CVEs           []string            `json:"cves"`
	Severity       string              `json:"severity"`
	Products       string              `json:"products"`
	Title          map[string]string   `json:"title"`
	Description    map[string]string   `json:"description"`
	FalsePositives map[string]string   `json:"false_positives"`
	Groups         [][]store.Condition `json:"groups"`
}

// NeedsExempt reports whether the template has an "{exempt}" IP placeholder.
func (t Template) NeedsExempt() bool {
	for _, g := range t.Groups {
		for _, c := range g {
			for _, v := range c.Value {
				if v == "{exempt}" {
					return true
				}
			}
		}
	}
	return false
}

type Bundle struct {
	Serial     int64       `json:"serial"`
	Version    string      `json:"version"`
	Published  time.Time   `json:"published"`
	Signatures []Signature `json:"signatures"`
	Templates  []Template  `json:"templates"`
}

var idRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,63}$`)
var optRe = regexp.MustCompile(`^[jomix]{0,6}$`)

func parse(data []byte) (*Bundle, error) {
	var b Bundle
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(&b); err != nil {
		return nil, fmt.Errorf("invalid bundle: %w", err)
	}
	if b.Serial <= 0 || b.Version == "" {
		return nil, errors.New("bundle has no serial/version")
	}
	seen := map[string]bool{}
	for _, s := range b.Signatures {
		if !idRe.MatchString(s.ID) || seen["s:"+s.ID] || s.Pattern == "" || len(s.Pattern) > 2048 || !optRe.MatchString(s.Options) {
			return nil, fmt.Errorf("invalid signature %q", s.ID)
		}
		seen["s:"+s.ID] = true
	}
	for _, t := range b.Templates {
		if !idRe.MatchString(t.ID) || seen["t:"+t.ID] || len(t.Groups) == 0 || t.Title["en"] == "" {
			return nil, fmt.Errorf("invalid template %q", t.ID)
		}
		seen["t:"+t.ID] = true
	}
	return &b, nil
}

// Verify checks data against the detached base64 signature.
func Verify(data, sig []byte) error {
	pub, _ := base64.StdEncoding.DecodeString(PublicKey)
	raw, err := base64.StdEncoding.DecodeString(strings.TrimSpace(string(sig)))
	if err != nil || len(pub) != ed25519.PublicKeySize || !ed25519.Verify(pub, data, raw) {
		return errors.New("bundle signature is not valid")
	}
	return nil
}

// Settings (meta "managed_rules").
type Settings struct {
	URL        string `json:"url"`         // bundle URL; "<url>.sig" holds the signature
	AutoUpdate bool   `json:"auto_update"` // check daily
}

type Status struct {
	Serial      int64      `json:"serial"`
	Version     string     `json:"version"`
	Source      string     `json:"source"` // builtin | installed
	Signatures  int        `json:"signatures"`
	Templates   int        `json:"templates"`
	InstalledAt *time.Time `json:"installed_at,omitempty"`
	CheckedAt   *time.Time `json:"checked_at,omitempty"`
	Error       string     `json:"error,omitempty"`
	Settings    Settings   `json:"settings"`

	ErrorHint string `json:"error_hint,omitempty"` // classifies Error (internal/hints)
}

type Service struct {
	Dir       string
	Store     *store.Store
	OnUpdated func(ctx context.Context)
	Client    *http.Client

	mu        sync.RWMutex
	active    *Bundle
	source    string
	installed *time.Time
	checked   *time.Time
	lastErr   string
}

func (s *Service) path() string { return filepath.Join(s.Dir, "bundle.json") }

// Init loads the built-in bundle and, if newer and validly signed, the installed one.
func (s *Service) Init() error {
	b, err := parse(defaultBundle)
	if err != nil {
		return fmt.Errorf("built-in rule bundle: %w", err)
	}
	s.active, s.source = b, "builtin"
	data, err1 := os.ReadFile(s.path())
	sig, err2 := os.ReadFile(s.path() + ".sig")
	if err1 == nil && err2 == nil {
		if Verify(data, sig) == nil {
			if ib, err := parse(data); err == nil && ib.Serial > b.Serial {
				s.active, s.source = ib, "installed"
				if st, err := os.Stat(s.path()); err == nil {
					t := st.ModTime().UTC()
					s.installed = &t
				}
			}
		} else {
			slog.Warn("installed rule bundle has an invalid signature; using the built-in bundle")
		}
	}
	return nil
}

func (s *Service) Bundle() *Bundle {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.active
}

func (s *Service) Settings(ctx context.Context) Settings {
	var set Settings
	if v, _ := s.Store.GetMeta(ctx, "managed_rules"); v != "" {
		_ = json.Unmarshal([]byte(v), &set)
	}
	if set.URL == "" {
		set.URL = os.Getenv("CORVUS_RULES_URL")
	}
	return set
}

func (s *Service) Status(ctx context.Context) Status {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return Status{Serial: s.active.Serial, Version: s.active.Version, Source: s.source, Signatures: len(s.active.Signatures),
		Templates: len(s.active.Templates), InstalledAt: s.installed, CheckedAt: s.checked, Error: s.lastErr, ErrorHint: hints.Net(s.lastErr), Settings: s.Settings(ctx)}
}

// Install verifies and activates a bundle (from the update URL or an upload).
func (s *Service) Install(ctx context.Context, data, sig []byte) (Status, error) {
	err := s.install(data, sig)
	s.mu.Lock()
	now := time.Now().UTC()
	s.checked = &now
	if err != nil {
		s.lastErr = err.Error()
	} else {
		s.lastErr = ""
	}
	s.mu.Unlock()
	if err == nil && s.OnUpdated != nil {
		s.OnUpdated(ctx)
	}
	return s.Status(ctx), err
}

var ErrNotNewer = errors.New("bundle is not newer than the active one")

func (s *Service) install(data, sig []byte) error {
	if err := Verify(data, sig); err != nil {
		return err
	}
	b, err := parse(data)
	if err != nil {
		return err
	}
	s.mu.RLock()
	cur := s.active.Serial
	s.mu.RUnlock()
	if b.Serial <= cur {
		return ErrNotNewer
	}
	if err := os.MkdirAll(s.Dir, 0o750); err != nil {
		return err
	}
	for name, content := range map[string][]byte{"bundle.json": data, "bundle.json.sig": sig} {
		tmp := filepath.Join(s.Dir, name+".tmp")
		if err := os.WriteFile(tmp, content, 0o640); err != nil {
			return err
		}
		if err := os.Rename(tmp, filepath.Join(s.Dir, name)); err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	s.mu.Lock()
	s.active, s.source, s.installed = b, "installed", &now
	s.mu.Unlock()
	slog.Info("rule bundle installed", "version", b.Version, "serial", b.Serial)
	return nil
}

// Check downloads <url> and <url>.sig and installs them when newer.
func (s *Service) Check(ctx context.Context) (Status, error) {
	set := s.Settings(ctx)
	if set.URL == "" {
		return s.Status(ctx), errors.New("no update URL configured")
	}
	if !strings.HasPrefix(set.URL, "https://") {
		return s.Status(ctx), errors.New("update URL must use https")
	}
	data, err := s.get(ctx, set.URL)
	if err == nil {
		var sig []byte
		if sig, err = s.get(ctx, set.URL+".sig"); err == nil {
			st, ierr := s.Install(ctx, data, sig)
			if errors.Is(ierr, ErrNotNewer) {
				s.mu.Lock()
				s.lastErr = ""
				s.mu.Unlock()
				return s.Status(ctx), nil
			}
			return st, ierr
		}
	}
	s.mu.Lock()
	now := time.Now().UTC()
	s.checked, s.lastErr = &now, err.Error()
	s.mu.Unlock()
	return s.Status(ctx), err
}

func (s *Service) get(ctx context.Context, url string) ([]byte, error) {
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: time.Minute}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "corvus-awpg")
	resp, err := c.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("%s: HTTP %d", url, resp.StatusCode)
	}
	return io.ReadAll(io.LimitReader(resp.Body, 8<<20))
}

// Run checks for updates daily when auto-update is on.
func (s *Service) Run(ctx context.Context) {
	t := time.NewTicker(24 * time.Hour)
	defer t.Stop()
	check := func() {
		if set := s.Settings(ctx); set.AutoUpdate && set.URL != "" {
			if _, err := s.Check(ctx); err != nil {
				slog.Warn("rule bundle update check failed", "err", err)
			}
		}
	}
	check()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			check()
		}
	}
}

// Rule builds a security rule from the template. "{exempt}" placeholders become the
// given IP list (exemptListID > 0) or the condition is dropped.
func (t Template) Rule(action string, statusCode int, scope string, hostIDs []int64, exemptListID int64) store.SecurityRule {
	var groups [][]store.Condition
	for _, g := range t.Groups {
		var out []store.Condition
		for _, c := range g {
			if len(c.Value) == 1 && c.Value[0] == "{exempt}" {
				if exemptListID <= 0 {
					continue
				}
				c.Value = []string{fmt.Sprintf("list:%d", exemptListID)}
			} else {
				c.Value = append([]string(nil), c.Value...)
			}
			out = append(out, c)
		}
		if len(out) > 0 {
			groups = append(groups, out)
		}
	}
	name := t.Title["en"]
	if len(t.CVEs) > 0 {
		name = "[" + t.CVEs[0] + "] " + name
	}
	if len([]rune(name)) > 100 {
		name = string([]rune(name)[:100])
	}
	desc := t.Description["en"]
	if len([]rune(desc)) > 480 {
		desc = string([]rune(desc)[:480])
	}
	return store.SecurityRule{Name: name, Description: desc, Enabled: true, Action: action, StatusCode: statusCode,
		Scope: scope, HostIDs: hostIDs, Expression: store.Expression{Groups: groups}}
}
