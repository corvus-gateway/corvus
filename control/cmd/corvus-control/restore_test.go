package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/backup"
)

func writeFile(t *testing.T, p, data string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
}

// An interrupted restore that cannot be rolled back stops startup; a staged restore that
// is rejected does not (the previous state is used).
func TestApplyStagedRestore(t *testing.T) {
	d := t.TempDir()
	if err := applyStagedRestore(d, time.Now()); err != nil {
		t.Fatalf("nothing staged: %v", err)
	}

	// rejected staged restore (no manifest is not even a restore; a bad manifest is)
	writeFile(t, filepath.Join(d, backup.PendingDir, backup.ManifestName), "not json")
	if err := applyStagedRestore(d, time.Now()); err != nil {
		t.Fatalf("rejected restore must not stop startup: %v", err)
	}

	rb := filepath.Join(d, "restore-backup-20261104-120000")
	writeFile(t, filepath.Join(rb, backup.JournalName),
		`{"moves":[{"from":"corvus.db","to":"restore-backup-20261104-120000/corvus.db"}],"complete":false}`)
	writeFile(t, filepath.Join(rb, "corvus.db"), "OLD")
	writeFile(t, filepath.Join(d, "corvus.db"), "NEW")
	err := applyStagedRestore(d, time.Now())
	if err == nil || !strings.Contains(err.Error(), "refusing to start") {
		t.Fatalf("unrecoverable interrupted restore: %v", err)
	}
}
