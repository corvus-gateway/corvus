// Package auth implements administrator authentication for the Corvus control plane.
package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net"
	"sort"
	"sync"
	"time"
	"unicode/utf8"

	"golang.org/x/crypto/bcrypt"
)

const (
	SessionCookie = "corvus_session"
	SessionTTL    = 12 * time.Hour
	MinPassword   = 10
	bcryptCost    = 12
)

var ErrWeakPassword = errors.New("password must be at least 10 characters")

func HashPassword(pw string) (string, error) {
	if utf8.RuneCountInString(pw) < MinPassword {
		return "", ErrWeakPassword
	}
	if len(pw) > 72 { // bcrypt limit; reject instead of silently truncating
		return "", errors.New("password must be at most 72 bytes")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(pw), bcryptCost)
	return string(h), err
}

// HashSecret hashes a password or PIN whose policy is checked by the caller (sign-in
// users and login page PINs, see validate). It still refuses more than bcrypt's 72 bytes.
func HashSecret(s string) (string, error) {
	if s == "" {
		return "", errors.New("empty secret")
	}
	if len(s) > 72 {
		return "", errors.New("password must be at most 72 bytes")
	}
	h, err := bcrypt.GenerateFromPassword([]byte(s), bcryptCost)
	return string(h), err
}

func CheckPassword(hash, pw string) bool {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(pw)) == nil
}

// dummyHash makes login timing similar whether or not the user exists.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("corvus-timing-equalizer"), bcryptCost)

func CheckPasswordOrDummy(hash, pw string) bool {
	if hash == "" {
		_ = bcrypt.CompareHashAndPassword(dummyHash, []byte(pw))
		return false
	}
	return CheckPassword(hash, pw)
}

// RandomToken returns n random bytes, URL-safe base64 encoded.
func RandomToken(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

// TokenHash is what is stored in the database instead of the session token itself.
func TokenHash(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}

// SetupToken guards the one-time creation of the first administrator.
type SetupToken struct {
	mu    sync.Mutex
	token string
}

func NewSetupToken() *SetupToken { return &SetupToken{token: RandomToken(18)} }

// MinPresetSetupToken is the shortest setup token accepted from configuration.
const MinPresetSetupToken = 12

// PresetSetupToken returns a setup token with a value chosen by the operator
// (CORVUS_SETUP_TOKEN), e.g. a password an app platform shows its user.
func PresetSetupToken(v string) (*SetupToken, error) {
	if len(v) < MinPresetSetupToken {
		return nil, fmt.Errorf("setup token must be at least %d characters", MinPresetSetupToken)
	}
	return &SetupToken{token: v}, nil
}

func (s *SetupToken) Value() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.token
}

// Restore makes a consumed token t valid again (when the setup it was used for failed).
func (s *SetupToken) Restore(t string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == "" {
		s.token = t
	}
}

// Consume returns true (and invalidates the token) if t matches.
func (s *SetupToken) Consume(t string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.token == "" || subtle.ConstantTimeCompare([]byte(s.token), []byte(t)) != 1 {
		return false
	}
	s.token = ""
	return true
}

// Limiter throttles failed login attempts per key (client IP, or account).
//
// Memory is bounded: keys whose failures have all expired are swept at most once per
// window (from Fail), and the table holds at most MaxKeys keys — when it is full after
// a sweep, the keys with the oldest last failure are dropped first. An IPv6 address
// counts per /64 (one host usually has the whole prefix), so one client cannot fill
// the table, or escape the limit, by rotating addresses.
type Limiter struct {
	Max    int
	Window time.Duration
	// MaxKeys caps the number of tracked keys (default DefaultMaxKeys).
	MaxKeys int

	mu        sync.Mutex
	fails     map[string][]time.Time
	lastFail  map[string]uint64 // order of each key's last failure (ties of time.Now)
	seq       uint64
	lastSweep time.Time
}

// DefaultMaxKeys bounds a Limiter's table: about 100k keys of a few hundred bytes.
const DefaultMaxKeys = 100_000

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, MaxKeys: DefaultMaxKeys, fails: map[string][]time.Time{}, lastFail: map[string]uint64{}}
}

// limiterKey counts an IPv6 address per /64; other keys are used as they are.
func limiterKey(key string) string {
	ip := net.ParseIP(key)
	if ip == nil || ip.To4() != nil {
		return key
	}
	return (&net.IPNet{IP: ip.Mask(net.CIDRMask(64, 128)), Mask: net.CIDRMask(64, 128)}).String()
}

// Sweep removes the keys whose failures have all expired.
func (l *Limiter) Sweep() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.sweep(time.Now())
}

func (l *Limiter) sweep(now time.Time) {
	l.lastSweep = now
	for k, list := range l.fails {
		if len(list) == 0 || now.Sub(list[len(list)-1]) > l.Window {
			l.forget(k)
		}
	}
}

// makeRoom keeps the table below MaxKeys before a new key is added: expired keys go
// first, then (only when that is not enough) the tenth of keys failed longest ago.
func (l *Limiter) makeRoom(now time.Time) {
	limit := l.MaxKeys
	if limit <= 0 {
		limit = DefaultMaxKeys
	}
	if len(l.fails) < limit {
		return
	}
	l.sweep(now)
	if len(l.fails) < limit {
		return
	}
	// Ordered by when each key last failed: the failure sequence number, not the time,
	// since time.Now can return the same value for many calls (coarse clocks).
	type entry struct {
		key  string
		last uint64
	}
	all := make([]entry, 0, len(l.fails))
	for k := range l.fails {
		all = append(all, entry{k, l.lastFail[k]})
	}
	sort.Slice(all, func(i, j int) bool { return all[i].last < all[j].last })
	drop := len(all) - limit + 1 + limit/10
	for i := 0; i < drop && i < len(all); i++ {
		l.forget(all[i].key)
	}
}

// forget drops a key from the table.
func (l *Limiter) forget(key string) {
	delete(l.fails, key)
	delete(l.lastFail, key)
}

// Len reports how many keys are tracked.
func (l *Limiter) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return len(l.fails)
}

func (l *Limiter) prune(key string, now time.Time) []time.Time {
	list := l.fails[key]
	i := 0
	for ; i < len(list) && now.Sub(list[i]) > l.Window; i++ {
	}
	list = list[i:]
	if len(list) == 0 {
		l.forget(key)
	} else {
		l.fails[key] = list
	}
	return list
}

// Blocked reports whether key has too many recent failures and when it may retry.
func (l *Limiter) Blocked(key string) (bool, time.Duration) {
	key = limiterKey(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	list := l.prune(key, now)
	if len(list) < l.Max {
		return false, 0
	}
	return true, l.Window - now.Sub(list[0])
}

func (l *Limiter) Fail(key string) {
	key = limiterKey(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fail(key, time.Now())
}

// Allow records an attempt for key unless key is blocked, in one step (Blocked followed
// by Fail lets concurrent requests through between the two). It reports whether the
// attempt may go ahead.
func (l *Limiter) Allow(key string) bool {
	key = limiterKey(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.prune(key, now)) >= l.Max {
		return false
	}
	l.fail(key, now)
	return true
}

func (l *Limiter) fail(key string, now time.Time) {
	if now.Sub(l.lastSweep) > l.Window {
		l.sweep(now)
	}
	list := l.prune(key, now)
	if len(list) == 0 {
		l.makeRoom(now)
	}
	if l.Max > 0 && len(list) >= l.Max {
		// only the newest Max failures matter; older ones are not kept
		list = append(list[:0:0], list[len(list)-l.Max+1:]...)
	}
	l.fails[key] = append(list, now)
	if l.lastFail == nil {
		l.lastFail = map[string]uint64{}
	}
	l.seq++
	l.lastFail[key] = l.seq
}

func (l *Limiter) Reset(key string) {
	key = limiterKey(key)
	l.mu.Lock()
	defer l.mu.Unlock()
	l.forget(key)
}
