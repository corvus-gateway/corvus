package backup

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"
)

func write(t *testing.T, p, body string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(p), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

// fakeData builds a data directory with state and data that must not be backed up.
func fakeData(t *testing.T) string {
	d := t.TempDir()
	write(t, filepath.Join(d, "corvus.db"), "LIVE-DB")
	write(t, filepath.Join(d, "corvus.db-wal"), "WAL")
	write(t, filepath.Join(d, "secrets", "data_key"), "KEY")
	write(t, filepath.Join(d, "secrets", "waitroom_hmac"), "HMAC")
	write(t, filepath.Join(d, "certs", "default", "fullchain.pem"), "CERT")
	write(t, filepath.Join(d, "certs", "7", "privkey.pem"), "PRIV")
	write(t, filepath.Join(d, "certs", "7", "privkey.pem.tmp"), "HALF")
	write(t, filepath.Join(d, "rules", "bundle.json"), "{}")
	write(t, filepath.Join(d, "nginx", "versions", "v1", "nginx.conf"), "conf")
	write(t, filepath.Join(d, "history", "traffic.db"), "hist")
	write(t, filepath.Join(d, "history", "logs", "2026-01-01.log.gz"), "log")
	write(t, filepath.Join(d, "geoip", "country.bin"), "geo")
	write(t, filepath.Join(d, "run", "corvus.pid"), "1")
	return d
}

func snapshot(body string) func(context.Context, string) error {
	return func(_ context.Context, dst string) error { return os.WriteFile(dst, []byte(body), 0o600) }
}

func create(t *testing.T, d string) []byte {
	t.Helper()
	var buf bytes.Buffer
	if _, err := Create(context.Background(), &buf, CreateOptions{DataDir: d, Version: "1.2.3", Engine: "sqlite", Snapshot: snapshot("SNAPSHOT"),
		Now: time.Date(2026, 11, 4, 10, 0, 0, 0, time.UTC)}); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func readTar(t *testing.T, b []byte) map[string][]byte {
	t.Helper()
	gz, err := gzip.NewReader(bytes.NewReader(b))
	if err != nil {
		t.Fatal(err)
	}
	tr := tar.NewReader(gz)
	out := map[string][]byte{}
	for {
		h, err := tr.Next()
		if err == io.EOF {
			return out
		}
		if err != nil {
			t.Fatal(err)
		}
		if h.Typeflag == tar.TypeReg {
			out[h.Name], _ = io.ReadAll(tr)
		}
	}
}

func TestCreateContents(t *testing.T) {
	d := fakeData(t)
	files := readTar(t, create(t, d))
	var names []string
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	want := []string{"certs/7/privkey.pem", "certs/default/fullchain.pem", "corvus.db", "manifest.json", "rules/bundle.json", "secrets/data_key", "secrets/waitroom_hmac"}
	if strings.Join(names, ",") != strings.Join(want, ",") {
		t.Fatalf("archive files:\n got %v\nwant %v", names, want)
	}
	if string(files["corvus.db"]) != "SNAPSHOT" {
		t.Fatalf("database must come from the snapshot, got %q", files["corvus.db"])
	}
	var m Manifest
	if err := json.Unmarshal(files["manifest.json"], &m); err != nil {
		t.Fatal(err)
	}
	if m.Format != FormatVersion || m.Version != "1.2.3" || m.DBEngine != "sqlite" || m.CreatedAt.IsZero() || len(m.Files) != len(want)-1 {
		t.Fatalf("manifest %+v", m)
	}
	for _, f := range m.Files {
		sum := sha256.Sum256(files[f.Path])
		if f.SHA256 != hex.EncodeToString(sum[:]) || f.Size != int64(len(files[f.Path])) {
			t.Fatalf("manifest entry %+v does not match the archived file", f)
		}
	}
	// No temporary snapshot is left behind.
	if left, _ := filepath.Glob(filepath.Join(d, ".backup-*")); len(left) != 0 {
		t.Fatalf("temporary files left: %v", left)
	}
}

func TestCreatePostgresHasReadme(t *testing.T) {
	d := fakeData(t)
	var buf bytes.Buffer
	m, err := Create(context.Background(), &buf, CreateOptions{DataDir: d, Version: "1", Engine: "postgres"})
	if err != nil {
		t.Fatal(err)
	}
	files := readTar(t, buf.Bytes())
	if _, ok := files["corvus.db"]; ok {
		t.Fatal("PostgreSQL backup must not contain corvus.db")
	}
	if !strings.Contains(string(files["README.txt"]), "pg_dump") || m.Summarize().HasDB {
		t.Fatalf("README missing: %q", files["README.txt"])
	}
}

func TestFileName(t *testing.T) {
	got := FileName("1.0.2", time.Date(2026, 11, 4, 9, 8, 7, 0, time.UTC))
	if got != "corvus-backup-1.0.2-20261104-090807.tar.gz" {
		t.Fatal(got)
	}
}

// archive builds a tar.gz from raw entries (plus an optional matching manifest).
type entry struct {
	name, body, link string
	typ              byte
}

func build(t *testing.T, entries []entry, manifest *Manifest) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for _, e := range entries {
		typ := e.typ
		if typ == 0 {
			typ = tar.TypeReg
		}
		h := &tar.Header{Name: e.name, Typeflag: typ, Mode: 0o600, Linkname: e.link}
		if typ == tar.TypeReg {
			h.Size = int64(len(e.body))
		}
		if err := tw.WriteHeader(h); err != nil {
			t.Fatal(err)
		}
		if typ == tar.TypeReg {
			tw.Write([]byte(e.body))
		}
	}
	if manifest != nil {
		b, _ := json.Marshal(manifest)
		tw.WriteHeader(&tar.Header{Name: ManifestName, Typeflag: tar.TypeReg, Mode: 0o600, Size: int64(len(b))})
		tw.Write(b)
	}
	tw.Close()
	gz.Close()
	return buf.Bytes()
}

func manifestFor(entries []entry) *Manifest {
	m := &Manifest{Format: FormatVersion, Version: "1", DBEngine: "sqlite", CreatedAt: time.Now()}
	for _, e := range entries {
		if e.typ == 0 || e.typ == tar.TypeReg {
			sum := sha256.Sum256([]byte(e.body))
			m.Files = append(m.Files, FileEntry{Path: e.name, Size: int64(len(e.body)), SHA256: hex.EncodeToString(sum[:])})
		}
	}
	return m
}

func TestStageValid(t *testing.T) {
	src := fakeData(t)
	d := t.TempDir()
	m, err := Stage(bytes.NewReader(create(t, src)), d, 1<<20)
	if err != nil {
		t.Fatal(err)
	}
	s := m.Summarize()
	if !s.HasDB || s.Secrets != 2 || s.Certs != 2 || s.Files != 6 {
		t.Fatalf("summary %+v", s)
	}
	if b, _ := os.ReadFile(filepath.Join(d, UploadDir, "secrets", "data_key")); string(b) != "KEY" {
		t.Fatal("staged file missing")
	}
	if st, _ := Status(d); st != "uploaded" {
		t.Fatalf("status %q", st)
	}
	if _, err := Confirm(d); err != nil {
		t.Fatal(err)
	}
	if st, _ := Status(d); st != "pending" {
		t.Fatalf("status after confirm %q", st)
	}
	if err := Discard(d); err != nil {
		t.Fatal(err)
	}
	if st, _ := Status(d); st != "" {
		t.Fatalf("status after discard %q", st)
	}
}

func TestStageRejects(t *testing.T) {
	ok := []entry{{name: "secrets/data_key", body: "KEY"}, {name: "corvus.db", body: "DB"}}
	cases := map[string][]byte{
		"traversal":       build(t, append(ok, entry{name: "secrets/../../evil", body: "x"}), manifestFor(ok)),
		"dotdot top":      build(t, append(ok, entry{name: "../evil", body: "x"}), manifestFor(ok)),
		"absolute":        build(t, append(ok, entry{name: "/etc/passwd", body: "x"}), manifestFor(ok)),
		"windows path":    build(t, append(ok, entry{name: "certs\\..\\x", body: "x"}), manifestFor(ok)),
		"symlink":         build(t, append(ok, entry{name: "certs/link", typ: tar.TypeSymlink, link: "/etc/shadow"}), manifestFor(ok)),
		"hardlink":        build(t, append(ok, entry{name: "certs/link", typ: tar.TypeLink, link: "secrets/data_key"}), manifestFor(ok)),
		"unknown entry":   build(t, append(ok, entry{name: "nginx/nginx.conf", body: "x"}), manifestFor(append(ok, entry{name: "nginx/nginx.conf", body: "x"}))),
		"unknown top":     build(t, append(ok, entry{name: "evil.sh", body: "x"}), manifestFor(append(ok, entry{name: "evil.sh", body: "x"}))),
		"bad checksum":    build(t, []entry{{name: "secrets/data_key", body: "KEY"}, {name: "corvus.db", body: "TAMPERED"}}, manifestFor(ok)),
		"unlisted file":   build(t, append(ok, entry{name: "certs/1/x.pem", body: "x"}), manifestFor(ok)),
		"missing file":    build(t, ok[:1], manifestFor(ok)),
		"no manifest":     build(t, ok, nil),
		"no data key":     build(t, ok[1:], manifestFor(ok[1:])),
		"bad format":      build(t, ok, &Manifest{Format: 99, Files: manifestFor(ok).Files}),
		"duplicate entry": build(t, append(ok, ok[0]), manifestFor(ok)),
		"not gzip":        []byte("PK\x03\x04 this is a zip"),
	}
	for name, archive := range cases {
		t.Run(name, func(t *testing.T) {
			d := t.TempDir()
			_, err := Stage(bytes.NewReader(archive), d, 1<<20)
			if !errors.Is(err, ErrInvalid) {
				t.Fatalf("want ErrInvalid, got %v", err)
			}
			if _, err := os.Stat(filepath.Join(d, UploadDir)); err == nil {
				t.Fatal("rejected archive must not be staged")
			}
			if left, _ := os.ReadDir(d); len(left) != 0 {
				t.Fatalf("leftovers: %v", left)
			}
			if _, err := os.Stat(filepath.Join(filepath.Dir(d), "evil")); err == nil {
				t.Fatal("file written outside the staging directory")
			}
		})
	}
	// Size limit on the uncompressed content.
	big := []entry{{name: "secrets/data_key", body: "KEY"}, {name: "corvus.db", body: strings.Repeat("x", 4096)}}
	if _, err := Stage(bytes.NewReader(build(t, big, manifestFor(big))), t.TempDir(), 1024); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversize: %v", err)
	}
}

func TestApplyPending(t *testing.T) {
	// Backup of installation A, restored onto installation B.
	a := fakeData(t)
	write(t, filepath.Join(a, "certs", "9", "fullchain.pem"), "A-CERT")
	archive := create(t, a)

	b := t.TempDir()
	write(t, filepath.Join(b, "corvus.db"), "B-DB")
	write(t, filepath.Join(b, "corvus.db-wal"), "B-WAL")
	write(t, filepath.Join(b, "secrets", "data_key"), "B-KEY")
	write(t, filepath.Join(b, "certs", "3", "fullchain.pem"), "B-CERT")
	write(t, filepath.Join(b, "nginx", "versions", "v5", "nginx.conf"), "B-CONF")
	write(t, filepath.Join(b, "history", "traffic.db"), "B-HIST")

	if _, err := Stage(bytes.NewReader(archive), b, 1<<20); err != nil {
		t.Fatal(err)
	}
	// Not confirmed: nothing happens on start.
	if rb, err := ApplyPending(b, discard{}, time.Now()); err != nil || rb != "" {
		t.Fatalf("unconfirmed upload applied: %q %v", rb, err)
	}
	if _, err := Confirm(b); err != nil {
		t.Fatal(err)
	}
	rb, err := ApplyPending(b, discard{}, time.Date(2026, 11, 4, 12, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(rb) != "restore-backup-20261104-120000" {
		t.Fatalf("rollback dir %q", rb)
	}
	read := func(p string) string { x, _ := os.ReadFile(p); return string(x) }
	checks := map[string]string{
		filepath.Join(b, "corvus.db"):                              "SNAPSHOT",
		filepath.Join(b, "secrets", "data_key"):                    "KEY",
		filepath.Join(b, "certs", "9", "fullchain.pem"):            "A-CERT",
		filepath.Join(b, "history", "traffic.db"):                  "B-HIST", // not part of a backup: kept
		filepath.Join(rb, "corvus.db"):                             "B-DB",
		filepath.Join(rb, "corvus.db-wal"):                         "B-WAL",
		filepath.Join(rb, "secrets", "data_key"):                   "B-KEY",
		filepath.Join(rb, "certs", "3", "fullchain.pem"):           "B-CERT",
		filepath.Join(rb, "nginx", "versions", "v5", "nginx.conf"): "B-CONF",
	}
	for p, want := range checks {
		if got := read(p); got != want {
			t.Errorf("%s = %q, want %q", p, got, want)
		}
	}
	for _, gone := range []string{"corvus.db-wal", "nginx", PendingDir, filepath.Join("certs", "3")} {
		if _, err := os.Stat(filepath.Join(b, gone)); err == nil {
			t.Errorf("%s should not exist after the restore", gone)
		}
	}
	if _, err := os.Stat(filepath.Join(rb, "applied-manifest.json")); err != nil {
		t.Error("applied manifest not kept")
	}
	// Nothing pending any more.
	if rb2, err := ApplyPending(b, discard{}, time.Now()); err != nil || rb2 != "" {
		t.Fatalf("second start: %q %v", rb2, err)
	}
}

func TestApplyPendingTamperedKeepsState(t *testing.T) {
	b := t.TempDir()
	write(t, filepath.Join(b, "corvus.db"), "B-DB")
	write(t, filepath.Join(b, "secrets", "data_key"), "B-KEY")
	if _, err := Stage(bytes.NewReader(create(t, fakeData(t))), b, 1<<20); err != nil {
		t.Fatal(err)
	}
	if _, err := Confirm(b); err != nil {
		t.Fatal(err)
	}
	write(t, filepath.Join(b, PendingDir, "secrets", "data_key"), "TAMPERED")
	if _, err := ApplyPending(b, discard{}, time.Now()); err == nil {
		t.Fatal("tampered staged restore applied")
	}
	if x, _ := os.ReadFile(filepath.Join(b, "corvus.db")); string(x) != "B-DB" {
		t.Fatal("previous database replaced")
	}
	if x, _ := os.ReadFile(filepath.Join(b, "secrets", "data_key")); string(x) != "B-KEY" {
		t.Fatal("previous secrets replaced")
	}
	if _, err := os.Stat(filepath.Join(b, PendingDir)); err == nil {
		t.Fatal("failed restore would be retried on every start")
	}
	if m, _ := filepath.Glob(filepath.Join(b, "restore-failed-*")); len(m) != 1 {
		t.Fatalf("failed restore not set aside: %v", m)
	}
}

// Manual restore: the archive layout is the data directory layout.
func TestArchiveLayoutIsDataDir(t *testing.T) {
	files := readTar(t, create(t, fakeData(t)))
	for n := range files {
		top := strings.SplitN(n, "/", 2)[0]
		if top != "corvus.db" && top != "manifest.json" && top != "secrets" && top != "certs" && top != "rules" {
			t.Fatalf("unexpected top-level entry %q", n)
		}
	}
}

type discard struct{}

func (discard) Info(string, ...any)  {}
func (discard) Error(string, ...any) {}
