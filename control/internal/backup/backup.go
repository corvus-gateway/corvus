// Package backup creates, validates, stages and applies Corvus backups.
//
// A backup is a .tar.gz whose layout is the data directory layout, so restoring by hand
// is "extract into an empty data directory before the first start":
//
//	manifest.json   format version, Corvus version, creation time, DB engine, file list with sha256
//	corvus.db       consistent SQLite snapshot (VACUUM INTO); absent with PostgreSQL
//	README.txt      PostgreSQL only: the database must be backed up with pg_dump
//	secrets/…       data key, session/queue secrets, Redis password
//	certs/…         certificates and keys
//	rules/…         installed managed-rule bundle (when present)
//
// Regenerable or bulky data (nginx/ configuration versions, history/ traffic statistics
// and request logs, geoip/, run/, lists/ IP list range files) is never included. The
// entries of IP lists fed from a URL live only in lists/ (internal/ipset): after a
// restore they are downloaded again at once (internal/feeds); until then those lists
// are used as empty. Lists edited in the console are in the database.
//
// Restoring through the console is staged: the upload is validated and extracted to
// <data>/restore-upload, confirming renames it to <data>/restore-pending, and the next
// start of corvus-control (ApplyPending, before the database is opened) swaps it in,
// keeping the replaced files in <data>/restore-backup-<timestamp>.
package backup

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	// FormatVersion is the archive format written by this build (and the only one accepted).
	FormatVersion = 1
	ManifestName  = "manifest.json"
	ReadmeName    = "README.txt"
	DBName        = "corvus.db"

	UploadDir  = "restore-upload"  // validated, waiting for confirmation
	PendingDir = "restore-pending" // confirmed, applied on the next start
)

// maxEntries caps the number of archive entries (files and directories) a restore
// extracts. A backup holds the database, a few secrets, a handful of files per
// certificate and the managed-rule bundle: a few thousand entries at most.
var maxEntries = 50000

// stateDirs are the data subdirectories a backup carries (and a restore replaces).
var stateDirs = []string{"secrets", "certs", "rules"}

// regenerable is replaced along with a restore: it is derived from the restored database.
var regenerable = []string{"nginx"}

// FileEntry describes one regular file in the archive.
type FileEntry struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

// Manifest is manifest.json.
type Manifest struct {
	Format    int         `json:"format"`
	Version   string      `json:"version"`
	CreatedAt time.Time   `json:"created_at"`
	DBEngine  string      `json:"db_engine"`
	Files     []FileEntry `json:"files"`
}

// Summary is what the console shows after validating an upload.
type Summary struct {
	Version   string    `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	DBEngine  string    `json:"db_engine"`
	HasDB     bool      `json:"has_db"`
	Files     int       `json:"files"`
	Bytes     int64     `json:"bytes"`
	Secrets   int       `json:"secrets"`
	Certs     int       `json:"certs"` // certificate directories
}

// Summarize derives the console summary from a manifest.
func (m *Manifest) Summarize() Summary {
	s := Summary{Version: m.Version, CreatedAt: m.CreatedAt, DBEngine: m.DBEngine, Files: len(m.Files)}
	certDirs := map[string]bool{}
	for _, f := range m.Files {
		s.Bytes += f.Size
		parts := strings.Split(f.Path, "/")
		switch {
		case f.Path == DBName:
			s.HasDB = true
		case parts[0] == "secrets":
			s.Secrets++
		case parts[0] == "certs" && len(parts) > 2:
			certDirs[parts[1]] = true
		}
	}
	s.Certs = len(certDirs)
	return s
}

// CreateOptions configures Create.
type CreateOptions struct {
	DataDir string
	Version string
	Engine  string // "sqlite" or "postgres"
	// Snapshot writes a consistent copy of the SQLite database to dst; nil skips the
	// database (PostgreSQL) and adds README.txt instead.
	Snapshot func(ctx context.Context, dst string) error
	Now      time.Time
}

// FileName is the download name of a backup.
func FileName(version string, t time.Time) string {
	v := strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '.' || r == '-' || r == '_' {
			return r
		}
		return '_'
	}, version)
	if v == "" {
		v = "dev"
	}
	return fmt.Sprintf("corvus-backup-%s-%s.tar.gz", v, t.UTC().Format("20060102-150405"))
}

const pgReadme = `This Corvus backup was made from an installation that stores its settings in PostgreSQL
(CORVUS_DB_URL). The database is NOT part of this archive: back it up separately, e.g.

    pg_dump --format=custom --file=corvus.dump "$CORVUS_DB_URL"

and restore it with pg_restore. Keep the database dump together with this archive: the
database's encrypted fields can only be read with secrets/data_key from this archive.
`

// Create writes a backup archive to w. Files are hashed while they are written; the
// manifest is the last entry.
func Create(ctx context.Context, w io.Writer, o CreateOptions) (*Manifest, error) {
	if o.Now.IsZero() {
		o.Now = time.Now()
	}
	m := &Manifest{Format: FormatVersion, Version: o.Version, CreatedAt: o.Now.UTC(), DBEngine: o.Engine}
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)

	if o.Snapshot != nil {
		tmpDir, err := os.MkdirTemp(o.DataDir, ".backup-")
		if err != nil {
			return nil, err
		}
		defer os.RemoveAll(tmpDir)
		snap := filepath.Join(tmpDir, DBName)
		if err := o.Snapshot(ctx, snap); err != nil {
			return nil, fmt.Errorf("database snapshot: %w", err)
		}
		if err := addFile(tw, m, snap, DBName, 0o600, o.Now); err != nil {
			return nil, err
		}
	} else {
		if err := addBytes(tw, m, ReadmeName, []byte(pgReadme), o.Now); err != nil {
			return nil, err
		}
	}

	for _, d := range stateDirs {
		root := filepath.Join(o.DataDir, d)
		if _, err := os.Lstat(root); errors.Is(err, fs.ErrNotExist) {
			continue
		}
		err := filepath.WalkDir(root, func(p string, de fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			rel, err := filepath.Rel(o.DataDir, p)
			if err != nil {
				return err
			}
			name := filepath.ToSlash(rel)
			switch {
			case de.IsDir():
				return tw.WriteHeader(&tar.Header{Typeflag: tar.TypeDir, Name: name + "/", Mode: 0o700, ModTime: o.Now})
			case de.Type().IsRegular():
				if strings.HasSuffix(de.Name(), ".tmp") {
					return nil // half-written file of an atomic rename
				}
				return addFile(tw, m, p, name, 0o600, o.Now)
			default:
				return nil // symlinks, sockets…: not state
			}
		})
		if err != nil {
			return nil, err
		}
	}

	mb, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: ManifestName, Mode: 0o600, Size: int64(len(mb)), ModTime: o.Now}); err != nil {
		return nil, err
	}
	if _, err := tw.Write(mb); err != nil {
		return nil, err
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	return m, gz.Close()
}

func addBytes(tw *tar.Writer, m *Manifest, name string, b []byte, t time.Time) error {
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: 0o600, Size: int64(len(b)), ModTime: t}); err != nil {
		return err
	}
	if _, err := tw.Write(b); err != nil {
		return err
	}
	sum := sha256.Sum256(b)
	m.Files = append(m.Files, FileEntry{Path: name, Size: int64(len(b)), SHA256: hex.EncodeToString(sum[:])})
	return nil
}

func addFile(tw *tar.Writer, m *Manifest, src, name string, mode int64, t time.Time) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return err
	}
	if err := tw.WriteHeader(&tar.Header{Typeflag: tar.TypeReg, Name: name, Mode: mode, Size: st.Size(), ModTime: t}); err != nil {
		return err
	}
	h := sha256.New()
	// Exactly the stat'ed size: a file that shrank fails, one that grew is cut (and hashed as written).
	if _, err := io.CopyN(io.MultiWriter(tw, h), f, st.Size()); err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	m.Files = append(m.Files, FileEntry{Path: name, Size: st.Size(), SHA256: hex.EncodeToString(h.Sum(nil))})
	return nil
}

// ------------------------------------------------------------------ validation & staging

// ErrInvalid wraps every reason an uploaded archive is rejected.
var ErrInvalid = errors.New("invalid backup")

func invalid(format string, args ...any) error {
	return fmt.Errorf("%w: %s", ErrInvalid, fmt.Sprintf(format, args...))
}

// cleanName checks an archive entry name and returns it without a trailing slash.
func cleanName(name string) (string, error) {
	n := strings.TrimSuffix(strings.TrimPrefix(name, "./"), "/")
	if n == "" || n == "." {
		return "", nil
	}
	if strings.Contains(n, "\\") || strings.HasPrefix(n, "/") || filepath.IsAbs(n) || strings.ContainsRune(n, 0) || strings.Contains(n, ":") {
		return "", invalid("unsafe path %q", name)
	}
	if path.Clean(n) != n {
		return "", invalid("unsafe path %q", name)
	}
	for _, p := range strings.Split(n, "/") {
		if p == ".." || p == "." || p == "" {
			return "", invalid("unsafe path %q", name)
		}
	}
	top := strings.SplitN(n, "/", 2)[0]
	nested := strings.Contains(n, "/")
	switch {
	case !nested && (top == ManifestName || top == ReadmeName || top == DBName):
	case contains(stateDirs, top):
	default:
		return "", invalid("unexpected entry %q", name)
	}
	return n, nil
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// Extract validates the archive read from r and extracts it into dir (which must not
// exist). On any error dir is removed. maxBytes caps the total uncompressed size.
func Extract(r io.Reader, dir string, maxBytes int64) (*Manifest, error) {
	if err := os.Mkdir(dir, 0o700); err != nil {
		return nil, err
	}
	m, err := extract(r, dir, maxBytes)
	if err != nil {
		os.RemoveAll(dir)
		return nil, err
	}
	return m, nil
}

// maxManifestBytes caps the manifest of an archive being restored.
const maxManifestBytes = 16 << 20

func extract(r io.Reader, dir string, maxBytes int64) (*Manifest, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, invalid("not a gzip file")
	}
	defer gz.Close()
	tr := tar.NewReader(gz)
	got := map[string]FileEntry{}
	var manifest []byte
	var total int64
	entries := 0
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalid("corrupt archive: %v", err)
		}
		if entries++; entries > maxEntries {
			return nil, invalid("archive has more than %d entries", maxEntries)
		}
		name, err := cleanName(h.Name)
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
		}
		// cleanName already rejects ".." segments, absolute paths and links; no entry
		// name (state files are named after hosts and ids) contains ".." at all
		if strings.Contains(name, "..") {
			return nil, invalid("unsafe path %q", h.Name)
		}
		dst := filepath.Join(dir, filepath.FromSlash(name))
		switch h.Typeflag {
		case tar.TypeDir:
			if !contains(stateDirs, strings.SplitN(name, "/", 2)[0]) {
				return nil, invalid("unexpected directory %q", h.Name)
			}
			if err := os.MkdirAll(dst, 0o700); err != nil {
				return nil, err
			}
		case tar.TypeReg:
			if contains(stateDirs, name) {
				return nil, invalid("unexpected entry %q", h.Name)
			}
			if _, dup := got[name]; dup || (name == ManifestName && manifest != nil) {
				return nil, invalid("duplicate entry %q", h.Name)
			}
			if h.Size < 0 || total+h.Size > maxBytes {
				return nil, invalid("archive is larger than %d MB uncompressed", maxBytes>>20)
			}
			total += h.Size
			if name == ManifestName {
				// the manifest is read into memory: a few lines per file, never large
				if h.Size > maxManifestBytes {
					return nil, invalid("the manifest is larger than %d MB", maxManifestBytes>>20)
				}
				if manifest, err = io.ReadAll(io.LimitReader(tr, h.Size)); err != nil {
					return nil, invalid("corrupt archive: %v", err)
				}
				continue
			}
			if err := os.MkdirAll(filepath.Dir(dst), 0o700); err != nil {
				return nil, err
			}
			f, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
			if err != nil {
				return nil, err
			}
			hs := sha256.New()
			n, err := io.Copy(io.MultiWriter(f, hs), io.LimitReader(tr, h.Size))
			if cerr := f.Close(); err == nil {
				err = cerr
			}
			if err != nil || n != h.Size {
				return nil, invalid("corrupt archive entry %q", h.Name)
			}
			got[name] = FileEntry{Path: name, Size: n, SHA256: hex.EncodeToString(hs.Sum(nil))}
		default:
			return nil, invalid("entry %q is not a regular file or directory (links are not allowed)", h.Name)
		}
	}
	if manifest == nil {
		return nil, invalid("manifest.json is missing")
	}
	var m Manifest
	if err := json.Unmarshal(manifest, &m); err != nil {
		return nil, invalid("manifest.json: %v", err)
	}
	if m.Format != FormatVersion {
		return nil, invalid("unsupported backup format %d (this version reads format %d)", m.Format, FormatVersion)
	}
	if err := verify(&m, got); err != nil {
		return nil, err
	}
	// Keep the manifest with the staged files (ApplyPending re-checks them).
	if err := os.WriteFile(filepath.Join(dir, ManifestName), manifest, 0o600); err != nil {
		return nil, err
	}
	return &m, nil
}

// verify compares the manifest's file list with the files actually present.
func verify(m *Manifest, got map[string]FileEntry) error {
	seen := map[string]bool{}
	for _, f := range m.Files {
		if seen[f.Path] {
			return invalid("manifest lists %q twice", f.Path)
		}
		seen[f.Path] = true
		g, ok := got[f.Path]
		if !ok {
			return invalid("%q is listed in the manifest but missing", f.Path)
		}
		if g.Size != f.Size || !strings.EqualFold(g.SHA256, f.SHA256) {
			return invalid("checksum mismatch for %q", f.Path)
		}
	}
	for p := range got {
		if !seen[p] {
			return invalid("%q is not listed in the manifest", p)
		}
	}
	if _, ok := got["secrets/data_key"]; !ok {
		return invalid("secrets/data_key is missing")
	}
	return nil
}

// verifyDir re-hashes the files of an extracted backup against its manifest.
func verifyDir(dir string) (*Manifest, error) {
	b, err := os.ReadFile(filepath.Join(dir, ManifestName))
	if err != nil {
		return nil, invalid("manifest.json: %v", err)
	}
	var m Manifest
	if err := json.Unmarshal(b, &m); err != nil {
		return nil, invalid("manifest.json: %v", err)
	}
	if m.Format != FormatVersion {
		return nil, invalid("unsupported backup format %d", m.Format)
	}
	got := map[string]FileEntry{}
	err = filepath.WalkDir(dir, func(p string, de fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, p)
		name := filepath.ToSlash(rel)
		if de.IsDir() || name == ManifestName {
			return nil
		}
		if !de.Type().IsRegular() {
			return invalid("%q is not a regular file", name)
		}
		f, err := os.Open(p)
		if err != nil {
			return err
		}
		defer f.Close()
		h := sha256.New()
		n, err := io.Copy(h, f)
		if err != nil {
			return err
		}
		got[name] = FileEntry{Path: name, Size: n, SHA256: hex.EncodeToString(h.Sum(nil))}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return &m, verify(&m, got)
}

// Stage validates an uploaded archive and extracts it to <data>/restore-upload,
// replacing an earlier unconfirmed upload.
func Stage(r io.Reader, dataDir string, maxBytes int64) (*Manifest, error) {
	up := filepath.Join(dataDir, UploadDir)
	tmp := up + ".tmp"
	os.RemoveAll(tmp)
	m, err := Extract(r, tmp, maxBytes)
	if err != nil {
		return nil, err
	}
	if err := os.RemoveAll(up); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	if err := os.Rename(tmp, up); err != nil {
		os.RemoveAll(tmp)
		return nil, err
	}
	return m, nil
}

// Confirm marks the validated upload to be applied on the next start.
func Confirm(dataDir string) (*Manifest, error) {
	up := filepath.Join(dataDir, UploadDir)
	if !dirExists(up) {
		return nil, fs.ErrNotExist
	}
	m, err := verifyDir(up)
	if err != nil {
		return nil, err
	}
	pend := filepath.Join(dataDir, PendingDir)
	if err := os.RemoveAll(pend); err != nil {
		return nil, err
	}
	if err := os.Rename(up, pend); err != nil {
		return nil, err
	}
	return m, nil
}

// Discard removes an unconfirmed upload and a confirmed, not yet applied restore.
// DiscardUpload removes an uploaded backup that was not confirmed yet; a restore already
// confirmed (pending) is kept.
func DiscardUpload(dataDir string) error {
	for _, d := range []string{UploadDir, UploadDir + ".tmp"} {
		if err := os.RemoveAll(filepath.Join(dataDir, d)); err != nil {
			return err
		}
	}
	return nil
}

func Discard(dataDir string) error {
	for _, d := range []string{UploadDir, UploadDir + ".tmp", PendingDir} {
		if err := os.RemoveAll(filepath.Join(dataDir, d)); err != nil {
			return err
		}
	}
	return nil
}

// Status reports the staged restore: state is "", "uploaded" (waiting for confirmation)
// or "pending" (applied on the next start).
func Status(dataDir string) (state string, m *Manifest) {
	for _, s := range []struct{ dir, state string }{{PendingDir, "pending"}, {UploadDir, "uploaded"}} {
		b, err := os.ReadFile(filepath.Join(dataDir, s.dir, ManifestName))
		if err != nil {
			continue
		}
		var mm Manifest
		if json.Unmarshal(b, &mm) == nil {
			return s.state, &mm
		}
	}
	return "", nil
}

func dirExists(p string) bool {
	st, err := os.Stat(p)
	return err == nil && st.IsDir()
}

func exists(p string) bool {
	_, err := os.Lstat(p)
	return err == nil
}

// ------------------------------------------------------------------ apply on startup

// Logger is the subset of slog used by ApplyPending.
type Logger interface {
	Info(msg string, args ...any)
	Error(msg string, args ...any)
}

// JournalName is the move journal ApplyPending keeps in the rollback directory.
const JournalName = "restore-journal.json"

// keepRestoreDirs is how long restore-backup-* and restore-failed-* directories are
// kept (the newest of each kind is always kept).
const keepRestoreDirs = 30 * 24 * time.Hour

// journal records the moves of a restore before they are made, so a restore that is
// interrupted (power cut, crash) is rolled back on the next start instead of leaving
// the old database moved aside and Corvus starting empty.
type journal struct {
	Moves    []journalMove `json:"moves"`
	Complete bool          `json:"complete"`
}

type journalMove struct {
	From string `json:"from"` // relative to the data directory
	To   string `json:"to"`
}

// save writes the journal durably (temporary file, fsync, rename, directory fsync).
func (j *journal) save(dir string) error {
	b, err := json.Marshal(j)
	if err != nil {
		return err
	}
	tmp := filepath.Join(dir, JournalName+".tmp")
	f, err := os.OpenFile(tmp, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o600)
	if err != nil {
		return err
	}
	_, err = f.Write(b)
	if err == nil {
		err = f.Sync()
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp, filepath.Join(dir, JournalName))
	}
	if err != nil {
		os.Remove(tmp)
		return err
	}
	syncDir(dir)
	return nil
}

// syncDir makes renames in dir durable (best effort: not supported everywhere).
func syncDir(dir string) {
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync()
		d.Close()
	}
}

// rollback moves the journaled files back, newest move first. A move that never
// happened (its target is missing) is skipped. It returns how many moves could not be
// undone.
func (j *journal) rollback(dataDir string, log Logger) int {
	failed := 0
	for i := len(j.Moves) - 1; i >= 0; i-- {
		from, to := filepath.Join(dataDir, j.Moves[i].From), filepath.Join(dataDir, j.Moves[i].To)
		if !exists(to) {
			continue // not moved yet
		}
		if exists(from) {
			log.Error("restore rollback: both copies exist, leaving them as they are", "path", from, "moved", to)
			failed++
			continue
		}
		if err := os.Rename(to, from); err != nil {
			log.Error("restore rollback: could not move back", "from", to, "to", from, "err", err)
			failed++
		}
	}
	syncDir(dataDir)
	return failed
}

// testCrash, when set, simulates a power cut before move number n: ApplyPending stops
// on the spot, as the process would.
var testCrash func(n int) bool

var errCrash = errors.New("simulated crash")

// InterruptedRestoreError: a restore interrupted before it completed could not be rolled
// back (files could not be moved back, or its journal is unreadable). The data directory
// mixes old and restored files, possibly without a database or data key: Corvus must not
// start on it (it would create an empty database and a new data key). The journal is
// kept for the next try.
type InterruptedRestoreError struct{ Err error }

func (e *InterruptedRestoreError) Error() string {
	return "an interrupted restore from backup could not be rolled back: " + e.Err.Error()
}
func (e *InterruptedRestoreError) Unwrap() error { return e.Err }

// recoverInterrupted rolls back restores interrupted before they completed. It returns
// an *InterruptedRestoreError when a rollback could not be finished (the journal is
// kept for the next try).
func recoverInterrupted(dataDir string, log Logger) error {
	dirs, _ := filepath.Glob(filepath.Join(dataDir, "restore-backup-*"))
	sort.Strings(dirs)
	var errs []error
	for i := len(dirs) - 1; i >= 0; i-- {
		rb := dirs[i]
		raw, err := os.ReadFile(filepath.Join(rb, JournalName))
		if err != nil {
			continue // completed by an older version, or nothing journaled yet
		}
		var j journal
		if err := json.Unmarshal(raw, &j); err != nil {
			log.Error("restore journal is unreadable; not rolled back", "dir", rb, "err", err)
			errs = append(errs, err)
			continue
		}
		if j.Complete {
			continue
		}
		log.Error("a restore from backup was interrupted; rolling it back", "dir", rb, "moves", len(j.Moves))
		if n := j.rollback(dataDir, log); n > 0 {
			errs = append(errs, fmt.Errorf("rolling back the interrupted restore in %s: %d files could not be moved back", rb, n))
			continue
		}
		os.Remove(filepath.Join(rb, JournalName))
		if err := os.Remove(rb); err != nil { // only when empty
			log.Error("restore rollback: the rollback directory is not empty", "dir", rb, "err", err)
		}
		log.Info("interrupted restore rolled back; the previous state is in place")
	}
	if len(errs) > 0 {
		return &InterruptedRestoreError{Err: errors.Join(errs...)}
	}
	return nil
}

// cleanRestoreDirs removes restore-backup-* and restore-failed-* directories older than
// keepRestoreDirs, keeping the newest of each kind and any with an unfinished journal.
func cleanRestoreDirs(dataDir string, log Logger, now time.Time) {
	for _, kind := range []string{"restore-backup-", "restore-failed-"} {
		dirs, _ := filepath.Glob(filepath.Join(dataDir, kind+"*"))
		sort.Strings(dirs) // the timestamp sorts chronologically
		for i := 0; i < len(dirs)-1; i++ {
			d := dirs[i]
			st, err := os.Stat(d)
			if err != nil || !st.IsDir() {
				continue
			}
			t, err := time.Parse("20060102-150405", strings.TrimPrefix(filepath.Base(d), kind))
			if err != nil {
				t = st.ModTime()
			}
			if now.Sub(t) < keepRestoreDirs {
				continue
			}
			if raw, err := os.ReadFile(filepath.Join(d, JournalName)); err == nil {
				var j journal
				if json.Unmarshal(raw, &j) != nil || !j.Complete {
					continue
				}
			}
			if err := os.RemoveAll(d); err != nil {
				log.Error("could not remove an old restore directory", "dir", d, "err", err)
			} else {
				log.Info("old restore directory removed", "dir", filepath.Base(d))
			}
		}
	}
}

// ApplyPending applies a confirmed restore (<data>/restore-pending) before the database
// is opened. The replaced files are moved to <data>/restore-backup-<ts>/. On failure every
// move is undone, the staged restore is renamed to restore-failed-<ts> (so it is not
// retried on every start) and the error is returned; the old state stays in place.
// It returns the rollback directory ("" when nothing was pending).
//
// Every move is journaled (restore-backup-<ts>/restore-journal.json, synced) before it
// is made, and the journal is marked complete at the end: a restore interrupted by a
// power cut is rolled back on the next start and then applied again. Restore
// directories older than 30 days are removed (the newest of each kind is kept).
//
// An interrupted restore that cannot be rolled back is reported as an
// *InterruptedRestoreError: the caller must not start on that data directory.
func ApplyPending(dataDir string, log Logger, now time.Time) (string, error) {
	if err := recoverInterrupted(dataDir, log); err != nil {
		// files of the interrupted restore are still mixed in: do not apply on top
		log.Error("an interrupted restore could not be rolled back completely; check the data directory", "err", err)
		return "", err
	}
	cleanRestoreDirs(dataDir, log, now)
	pend := filepath.Join(dataDir, PendingDir)
	if !dirExists(pend) {
		os.RemoveAll(filepath.Join(dataDir, UploadDir+".tmp"))
		return "", nil
	}
	if !exists(filepath.Join(pend, ManifestName)) {
		// left behind by a restore that completed just before its staging directory was
		// removed (the manifest is moved out last, see below)
		if err := os.RemoveAll(pend); err != nil {
			log.Error("could not remove the restore staging directory", "dir", pend, "err", err)
		}
		return "", nil
	}
	ts := now.UTC().Format("20060102-150405")
	fail := func(err error) (string, error) {
		failed := filepath.Join(dataDir, "restore-failed-"+ts)
		if rerr := os.Rename(pend, failed); rerr != nil {
			log.Error("could not set the failed restore aside", "err", rerr)
		}
		log.Error("restore from backup FAILED; the previous state was kept", "err", err, "staged_files", failed)
		return "", err
	}
	m, err := verifyDir(pend)
	if err != nil {
		return fail(err)
	}
	rbName := "restore-backup-" + ts
	rb := filepath.Join(dataDir, rbName)
	if err := os.Mkdir(rb, 0o700); err != nil {
		return fail(err)
	}

	// Current files that the restore replaces: the database (with its WAL/SHM, which must
	// never be applied to another database), the state directories and regenerable config.
	var aside []string
	hasDB := exists(filepath.Join(pend, DBName))
	if hasDB {
		aside = append(aside, DBName, DBName+"-wal", DBName+"-shm", DBName+"-journal")
	}
	aside = append(append(aside, stateDirs...), regenerable...)

	j := &journal{}
	if err := j.save(rb); err != nil {
		os.Remove(rb)
		return fail(err)
	}
	undo := func() {
		if j.rollback(dataDir, log) == 0 {
			os.Remove(filepath.Join(rb, JournalName))
			os.Remove(rb)
		}
	}
	// mv journals a move durably before making it (paths relative to dataDir).
	mv := func(from, to string) error {
		if testCrash != nil && testCrash(len(j.Moves)) {
			return errCrash
		}
		j.Moves = append(j.Moves, journalMove{From: from, To: to})
		if err := j.save(rb); err != nil {
			j.Moves = j.Moves[:len(j.Moves)-1]
			return err
		}
		return os.Rename(filepath.Join(dataDir, from), filepath.Join(dataDir, to))
	}
	apply := func() error {
		for _, n := range aside {
			if !exists(filepath.Join(dataDir, n)) {
				continue
			}
			if err := mv(n, filepath.Join(rbName, n)); err != nil {
				return err
			}
		}
		entries, err := os.ReadDir(pend)
		if err != nil {
			return err
		}
		sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
		for _, e := range entries {
			n := e.Name()
			if n == ManifestName || n == ReadmeName {
				continue
			}
			if err := mv(filepath.Join(PendingDir, n), n); err != nil {
				return err
			}
		}
		// Keep the applied manifest with the rollback copy for reference. Moved last: a
		// staging directory without a manifest is one whose restore completed.
		return mv(filepath.Join(PendingDir, ManifestName), filepath.Join(rbName, "applied-"+ManifestName))
	}
	if err := apply(); err != nil {
		if errors.Is(err, errCrash) {
			return "", err // the process "died" here (tests)
		}
		undo()
		return fail(err)
	}
	syncDir(dataDir)
	j.Complete = true
	if err := j.save(rb); err != nil {
		// without a journal the restore counts as complete too; never roll back a
		// finished restore on the next start
		if rerr := os.Remove(filepath.Join(rb, JournalName)); rerr != nil {
			log.Error("could not mark the restore as complete; remove the journal before the next start", "journal", filepath.Join(rb, JournalName), "err", errors.Join(err, rerr))
		}
	}
	if err := os.RemoveAll(pend); err != nil {
		log.Error("could not remove the restore staging directory", "dir", pend, "err", err)
	}
	s := m.Summarize()
	log.Info("RESTORED FROM BACKUP", "backup_version", m.Version, "backup_created_at", m.CreatedAt, "db_engine", m.DBEngine,
		"files", s.Files, "previous_state", rb)
	return rb, nil
}
