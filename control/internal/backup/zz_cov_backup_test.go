package backup

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type failWriter struct{ after int }

func (f *failWriter) Write(p []byte) (int, error) {
	if f.after <= 0 {
		return 0, errors.New("disk full")
	}
	f.after -= len(p)
	return len(p), nil
}

func TestCovFileNameSanitises(t *testing.T) {
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	if got := FileName("", at); got != "corvus-backup-dev-20260102-020405.tar.gz" {
		t.Fatalf("empty version: %s", got)
	}
	if got := FileName("1.2.3 rc/1", at); got != "corvus-backup-1.2.3_rc_1-20260102-020405.tar.gz" {
		t.Fatalf("special characters: %s", got)
	}
}

func TestCovCreateErrors(t *testing.T) {
	ctx := context.Background()
	d := fakeData(t)
	boom := errors.New("vacuum failed")
	_, err := Create(ctx, &bytes.Buffer{}, CreateOptions{DataDir: d, Snapshot: func(context.Context, string) error { return boom }})
	if !errors.Is(err, boom) || !strings.Contains(err.Error(), "database snapshot") {
		t.Fatalf("snapshot error: %v", err)
	}
	// the temporary snapshot directory is cleaned up
	ents, _ := os.ReadDir(d)
	for _, e := range ents {
		if strings.HasPrefix(e.Name(), ".backup-") {
			t.Fatalf("left over %s", e.Name())
		}
	}
	if _, err := Create(ctx, &bytes.Buffer{}, CreateOptions{DataDir: filepath.Join(d, "missing"), Snapshot: snapshot("x")}); err == nil {
		t.Fatal("missing data dir must fail")
	}
	cctx, cancel := context.WithCancel(ctx)
	cancel()
	if _, err := Create(cctx, &bytes.Buffer{}, CreateOptions{DataDir: d}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
	for _, n := range []int{0, 100} {
		if _, err := Create(ctx, &failWriter{after: n}, CreateOptions{DataDir: d, Snapshot: snapshot(strings.Repeat("S", 1<<16))}); err == nil {
			t.Errorf("writer failing after %d bytes: no error", n)
		}
	}
	// no Now: the current time is used
	var buf bytes.Buffer
	m, err := Create(ctx, &buf, CreateOptions{DataDir: d})
	if err != nil || time.Since(m.CreatedAt) > time.Minute {
		t.Fatalf("default time: %+v %v", m, err)
	}
	// a snapshot file that vanishes before it is archived
	_, err = Create(ctx, &bytes.Buffer{}, CreateOptions{DataDir: d, Snapshot: func(context.Context, string) error { return nil }})
	if err == nil {
		t.Fatal("missing snapshot file must fail")
	}
}

func TestCovDiscardUploadKeepsPending(t *testing.T) {
	src := fakeData(t)
	d := t.TempDir()
	if _, err := Stage(bytes.NewReader(create(t, src)), d, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(d); err != nil {
		t.Fatal(err)
	}
	if _, err := Stage(bytes.NewReader(create(t, src)), d, 1<<20); err != nil {
		t.Fatal(err)
	}
	_ = os.MkdirAll(filepath.Join(d, UploadDir+".tmp"), 0o700)
	if err := DiscardUpload(d); err != nil {
		t.Fatal(err)
	}
	if st, _ := Status(d); st != "pending" {
		t.Fatalf("the confirmed restore must stay: %q", st)
	}
	for _, p := range []string{UploadDir, UploadDir + ".tmp"} {
		if _, err := os.Stat(filepath.Join(d, p)); !os.IsNotExist(err) {
			t.Errorf("%s not removed", p)
		}
	}
	if _, err := Confirm(d); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("confirm without upload: %v", err)
	}
}

func TestCovConfirmRejectsTamperedUpload(t *testing.T) {
	src := fakeData(t)
	d := t.TempDir()
	if _, err := Stage(bytes.NewReader(create(t, src)), d, 1<<20); err != nil {
		t.Fatal(err)
	}
	// modified after the upload was validated
	if err := os.WriteFile(filepath.Join(d, UploadDir, "secrets", "data_key"), []byte("EVIL"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(d); err == nil {
		t.Fatal("tampered upload must not be confirmed")
	}
	if st, _ := Status(d); st != "uploaded" {
		t.Fatalf("status = %q", st)
	}
	// an extra file is rejected too
	_ = os.WriteFile(filepath.Join(d, UploadDir, "secrets", "data_key"), []byte("KEY"), 0o600)
	_ = os.WriteFile(filepath.Join(d, UploadDir, "secrets", "extra"), []byte("x"), 0o600)
	if _, err := Confirm(d); err == nil {
		t.Fatal("upload with an unlisted file must not be confirmed")
	}
	_ = os.Remove(filepath.Join(d, UploadDir, "secrets", "extra"))
	_ = os.Remove(filepath.Join(d, UploadDir, ManifestName))
	if _, err := Confirm(d); err == nil {
		t.Fatal("upload without manifest must not be confirmed")
	}
}

func TestCovStageOverLimit(t *testing.T) {
	src := fakeData(t)
	d := t.TempDir()
	if _, err := Stage(bytes.NewReader(create(t, src)), d, 10); err == nil {
		t.Fatal("an archive over the size limit must be rejected")
	}
	if st, _ := Status(d); st != "" {
		t.Fatalf("nothing staged: %q", st)
	}
	if _, err := os.Stat(filepath.Join(d, UploadDir+".tmp")); !os.IsNotExist(err) {
		t.Fatal("temporary directory must be removed")
	}
}
