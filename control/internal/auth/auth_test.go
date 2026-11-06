package auth

import (
	"strings"
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
