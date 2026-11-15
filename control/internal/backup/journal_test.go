package backup

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func stagedRestore(t *testing.T) string {
	t.Helper()
	a := fakeData(t)
	write(t, filepath.Join(a, "certs", "9", "fullchain.pem"), "A-CERT")
	archive := create(t, a)
	b := t.TempDir()
	write(t, filepath.Join(b, "corvus.db"), "B-DB")
	write(t, filepath.Join(b, "corvus.db-wal"), "B-WAL")
	write(t, filepath.Join(b, "secrets", "data_key"), "B-KEY")
	write(t, filepath.Join(b, "certs", "3", "fullchain.pem"), "B-CERT")
	write(t, filepath.Join(b, "nginx", "versions", "v5", "nginx.conf"), "B-CONF")
	if _, err := Stage(bytes.NewReader(archive), b, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(b); err != nil {
		t.Fatal(err)
	}
	return b
}

func readFile(p string) string { x, _ := os.ReadFile(p); return string(x) }

// A restore interrupted at any point (power cut) is rolled back on the next start —
// the previous database is never left moved aside — and then applied again.
func TestApplyPendingInterrupted(t *testing.T) {
	defer func() { testCrash = nil }()
	for crashAt := 0; crashAt < 12; crashAt++ {
		b := stagedRestore(t)
		testCrash = func(n int) bool { return n == crashAt }
		t1 := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
		if _, err := ApplyPending(b, discard{}, t1); err == nil {
			testCrash = nil
			continue // fewer moves than crashAt: the restore completed
		}
		testCrash = nil

		// next start: rolled back, then applied
		rb, err := ApplyPending(b, discard{}, t1.Add(time.Minute))
		if err != nil {
			t.Fatalf("crash at %d: %v", crashAt, err)
		}
		if filepath.Base(rb) != "restore-backup-20261104-120100" {
			t.Fatalf("crash at %d: rollback dir %q", crashAt, rb)
		}
		for p, want := range map[string]string{
			filepath.Join(b, "corvus.db"):                    "SNAPSHOT",
			filepath.Join(b, "secrets", "data_key"):          "KEY",
			filepath.Join(b, "certs", "9", "fullchain.pem"):  "A-CERT",
			filepath.Join(rb, "corvus.db"):                   "B-DB",
			filepath.Join(rb, "corvus.db-wal"):               "B-WAL",
			filepath.Join(rb, "secrets", "data_key"):         "B-KEY",
			filepath.Join(rb, "certs", "3", "fullchain.pem"): "B-CERT",
		} {
			if got := readFile(p); got != want {
				t.Errorf("crash at %d: %s = %q, want %q", crashAt, p, got, want)
			}
		}
		if _, err := os.Stat(filepath.Join(b, "restore-backup-20261104-120000")); !os.IsNotExist(err) {
			t.Errorf("crash at %d: the rolled-back directory must be removed: %v", crashAt, err)
		}
		if _, err := os.Stat(filepath.Join(b, PendingDir)); !os.IsNotExist(err) {
			t.Errorf("crash at %d: staging directory left", crashAt)
		}
	}
}

// Just the rollback: after an interruption the previous state is back in place.
func TestInterruptedRestoreRollsBack(t *testing.T) {
	defer func() { testCrash = nil }()
	b := stagedRestore(t)
	testCrash = func(n int) bool { return n == 3 } // database files and secrets moved aside
	if _, err := ApplyPending(b, discard{}, time.Now()); err == nil {
		t.Fatal("expected the simulated crash")
	}
	testCrash = nil
	if _, err := os.Stat(filepath.Join(b, "corvus.db")); !os.IsNotExist(err) {
		t.Fatal("the test must interrupt after the database was moved aside")
	}
	if err := recoverInterrupted(b, discard{}); err != nil {
		t.Fatal(err)
	}
	for p, want := range map[string]string{
		filepath.Join(b, "corvus.db"):                   "B-DB",
		filepath.Join(b, "corvus.db-wal"):               "B-WAL",
		filepath.Join(b, "secrets", "data_key"):         "B-KEY",
		filepath.Join(b, "certs", "3", "fullchain.pem"): "B-CERT",
	} {
		if got := readFile(p); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	if _, err := verifyDir(filepath.Join(b, PendingDir)); err != nil {
		t.Fatalf("staged restore not intact after the rollback: %v", err)
	}
}

// A completed restore is never rolled back, and leftovers of a staging directory whose
// restore completed are removed instead of being reported as a failed restore.
func TestCompletedRestoreIsKept(t *testing.T) {
	b := stagedRestore(t)
	rb, err := ApplyPending(b, discard{}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(b, PendingDir, ReadmeName), "leftover")
	if _, err := ApplyPending(b, discard{}, time.Now()); err != nil {
		t.Fatal(err)
	}
	if readFile(filepath.Join(b, "corvus.db")) != "SNAPSHOT" || readFile(filepath.Join(rb, "corvus.db")) != "B-DB" {
		t.Fatal("completed restore changed")
	}
	if _, err := os.Stat(filepath.Join(b, PendingDir)); !os.IsNotExist(err) {
		t.Fatal("leftover staging directory not removed")
	}
	if m, _ := filepath.Glob(filepath.Join(b, "restore-failed-*")); len(m) != 0 {
		t.Fatalf("leftover reported as failed: %v", m)
	}
}

// Restore directories older than 30 days are removed; the newest of each kind stays.
func TestCleanRestoreDirs(t *testing.T) {
	b := t.TempDir()
	now := time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC)
	for _, n := range []string{
		"restore-backup-20260101-000000", // old: removed
		"restore-backup-20260201-000000", // old: removed
		"restore-backup-20260301-000000", // old but newest of its kind: kept
		"restore-failed-20260101-000000", // old: removed
		"restore-failed-20261101-000000", // recent: kept
	} {
		write(t, filepath.Join(b, n, "x"), "x")
	}
	// old, but its restore never completed: kept for the rollback
	write(t, filepath.Join(b, "restore-backup-20260115-000000", JournalName), `{"moves":[],"complete":false}`)
	cleanRestoreDirs(b, discard{}, now)
	want := map[string]bool{
		"restore-backup-20260115-000000": true,
		"restore-backup-20260301-000000": true,
		"restore-failed-20261101-000000": true,
	}
	ents, _ := os.ReadDir(b)
	got := map[string]bool{}
	for _, e := range ents {
		got[e.Name()] = true
	}
	if len(got) != len(want) {
		t.Fatalf("left %v, want %v", got, want)
	}
	for n := range want {
		if !got[n] {
			t.Fatalf("%s removed; left %v", n, got)
		}
	}
}

// An interrupted restore that cannot be rolled back is a distinct error (the caller
// must not start), and the journal is kept for the next try.
func TestInterruptedRestoreRollbackFails(t *testing.T) {
	b := t.TempDir()
	rb := filepath.Join(b, "restore-backup-20261104-120000")
	write(t, filepath.Join(rb, JournalName),
		`{"moves":[{"from":"corvus.db","to":"restore-backup-20261104-120000/corvus.db"}],"complete":false}`)
	write(t, filepath.Join(rb, "corvus.db"), "OLD-DB")
	write(t, filepath.Join(b, "corvus.db"), "RESTORED-DB") // both copies: cannot move back
	_, err := ApplyPending(b, discard{}, time.Now())
	var ie *InterruptedRestoreError
	if !errors.As(err, &ie) {
		t.Fatalf("err = %v (%T), want *InterruptedRestoreError", err, err)
	}
	if _, err := os.Stat(filepath.Join(rb, JournalName)); err != nil {
		t.Fatalf("journal must be kept: %v", err)
	}

	// an unreadable journal: the state is unknown, also refused
	write(t, filepath.Join(rb, JournalName), "{")
	if _, err := ApplyPending(b, discard{}, time.Now()); !errors.As(err, &ie) {
		t.Fatalf("unreadable journal: %v", err)
	}

	// a staged restore that is merely rejected is not such an error
	c := stagedRestore(t)
	write(t, filepath.Join(c, PendingDir, "corvus.db"), "TAMPERED")
	if _, err := ApplyPending(c, discard{}, time.Now()); err == nil || errors.As(err, &ie) {
		t.Fatalf("rejected restore: %v", err)
	}
}
