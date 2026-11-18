package auth

import (
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"
)

func TestHtpasswdHashIsBcrypt(t *testing.T) {
	h, err := HtpasswdHash("password")
	if err != nil || !strings.HasPrefix(h, "$2a$06$") {
		t.Fatalf("hash = %q, %v", h, err)
	}
	if bcrypt.CompareHashAndPassword([]byte(h), []byte("password")) != nil {
		t.Fatal("hash does not verify")
	}
}

func TestLimiter(t *testing.T) {
	l := NewLimiter(3, time.Minute)
	for i := 0; i < 3; i++ {
		if b, _ := l.Blocked("ip"); b {
			t.Fatal("blocked too early")
		}
		l.Fail("ip")
	}
	if b, _ := l.Blocked("ip"); !b {
		t.Fatal("not blocked after max failures")
	}
	l.Reset("ip")
	if b, _ := l.Blocked("ip"); b {
		t.Fatal("still blocked after reset")
	}
}

// Allow checks and records in one step: concurrent callers get exactly Max attempts.
func TestLimiterAllowConcurrent(t *testing.T) {
	l := NewLimiter(7, time.Minute)
	var wg sync.WaitGroup
	var allowed atomic.Int32
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if l.Allow("user:1") {
				allowed.Add(1)
			}
		}()
	}
	wg.Wait()
	if allowed.Load() != 7 {
		t.Fatalf("allowed %d, want 7", allowed.Load())
	}
	if b, _ := l.Blocked("user:1"); !b || l.Allow("user:1") || !l.Allow("user:2") {
		t.Fatal("Allow/Blocked disagree")
	}
}

func TestPresetSetupToken(t *testing.T) {
	if _, err := PresetSetupToken("short"); err == nil {
		t.Fatal("short preset token accepted")
	}
	s, err := PresetSetupToken("chosen-by-operator")
	if err != nil || s.Value() != "chosen-by-operator" || !s.Consume("chosen-by-operator") || s.Consume("chosen-by-operator") {
		t.Fatalf("preset token: %v", err)
	}
}

func TestSetupTokenSingleUse(t *testing.T) {
	s := NewSetupToken()
	v := s.Value()
	if s.Consume("wrong") || !s.Consume(v) || s.Consume(v) {
		t.Fatal("setup token must be single use and exact")
	}
	s.Restore(v)
	if !s.Consume(v) {
		t.Fatal("restored token not accepted")
	}
}
