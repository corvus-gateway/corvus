package auth

import (
	"fmt"
	"testing"
	"time"
)

// Keys whose failures expired are swept, not only when the same key is seen again.
func TestLimiterSweep(t *testing.T) {
	l := NewLimiter(3, 50*time.Millisecond)
	for i := 0; i < 100; i++ {
		l.Fail(fmt.Sprintf("acct:user%d@example.com", i))
	}
	if n := l.Len(); n != 100 {
		t.Fatalf("keys = %d", n)
	}
	time.Sleep(60 * time.Millisecond)
	l.Fail("fresh") // a Fail after a window sweeps the expired keys
	if n := l.Len(); n != 1 {
		t.Fatalf("keys after the window = %d, want 1", n)
	}
	time.Sleep(60 * time.Millisecond)
	l.Sweep()
	if n := l.Len(); n != 0 {
		t.Fatalf("keys after Sweep = %d", n)
	}
}

// The table never exceeds MaxKeys; the keys failed longest ago go first, so a client
// that is still being limited stays limited.
func TestLimiterCap(t *testing.T) {
	l := NewLimiter(2, time.Hour)
	l.MaxKeys = 100
	l.Fail("attacker")
	l.Fail("attacker")
	for i := 0; i < 1000; i++ {
		l.Fail(fmt.Sprintf("spray%d", i))
		if i%10 == 0 {
			l.Fail("attacker") // still active
		}
		if n := l.Len(); n > 100 {
			t.Fatalf("keys = %d after %d", n, i)
		}
	}
	if b, _ := l.Blocked("attacker"); !b {
		t.Fatal("the active key was evicted")
	}
	if b, _ := l.Blocked("spray0"); b {
		t.Fatal("old keys must have been dropped")
	}
}

// Only the newest Max failures per key are kept, and the wait ends when the count
// drops below Max.
func TestLimiterPerKeyBounded(t *testing.T) {
	l := NewLimiter(3, time.Hour)
	for i := 0; i < 50; i++ {
		l.Fail("k")
	}
	l.mu.Lock()
	n := len(l.fails["k"])
	l.mu.Unlock()
	if n != 3 {
		t.Fatalf("stored failures = %d", n)
	}
	if b, wait := l.Blocked("k"); !b || wait <= 59*time.Minute {
		t.Fatalf("blocked=%v wait=%v", b, wait)
	}
}

// IPv6 addresses count per /64.
func TestLimiterIPv6Prefix(t *testing.T) {
	l := NewLimiter(3, time.Hour)
	l.Fail("2001:db8:1:2::1")
	l.Fail("2001:db8:1:2::2")
	l.Fail("2001:db8:1:2:ffff::3")
	if b, _ := l.Blocked("2001:db8:1:2::99"); !b {
		t.Fatal("addresses of one /64 must share the limit")
	}
	if b, _ := l.Blocked("2001:db8:1:3::1"); b {
		t.Fatal("another /64 is limited separately")
	}
	l.Fail("192.0.2.1")
	l.Fail("192.0.2.1")
	l.Fail("192.0.2.1")
	if b, _ := l.Blocked("192.0.2.2"); b {
		t.Fatal("IPv4 addresses are limited one by one")
	}
	l.Reset("2001:db8:1:2::5")
	if b, _ := l.Blocked("2001:db8:1:2::1"); b {
		t.Fatal("Reset applies to the /64")
	}
	if limiterKey("acct:x@example.com") != "acct:x@example.com" {
		t.Fatal("non-IP keys are unchanged")
	}
}
