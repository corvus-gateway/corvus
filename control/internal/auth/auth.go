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

// Limiter throttles failed login attempts per key (client IP).
type Limiter struct {
	Max    int
	Window time.Duration

	mu    sync.Mutex
	fails map[string][]time.Time
}

func NewLimiter(max int, window time.Duration) *Limiter {
	return &Limiter{Max: max, Window: window, fails: map[string][]time.Time{}}
}

func (l *Limiter) prune(key string, now time.Time) []time.Time {
	list := l.fails[key]
	i := 0
	for ; i < len(list) && now.Sub(list[i]) > l.Window; i++ {
	}
	list = list[i:]
	if len(list) == 0 {
		delete(l.fails, key)
	} else {
		l.fails[key] = list
	}
	return list
}

// Blocked reports whether key has too many recent failures and when it may retry.
func (l *Limiter) Blocked(key string) (bool, time.Duration) {
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
	l.mu.Lock()
	defer l.mu.Unlock()
	l.fails[key] = append(l.prune(key, time.Now()), time.Now())
}

func (l *Limiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fails, key)
}
