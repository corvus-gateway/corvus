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
// and request logs, geoip/, run/) is never included.
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
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, invalid("corrupt archive: %v", err)
		}
		name, err := cleanName(h.Name)
		if err != nil {
			return nil, err
		}
		if name == "" {
			continue
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

// ApplyPending applies a confirmed restore (<data>/restore-pending) before the database
// is opened. The replaced files are moved to <data>/restore-backup-<ts>/. On failure every
// move is undone, the staged restore is renamed to restore-failed-<ts> (so it is not
// retried on every start) and the error is returned; the old state stays in place.
// It returns the rollback directory ("" when nothing was pending).
func ApplyPending(dataDir string, log Logger, now time.Time) (string, error) {
	pend := filepath.Join(dataDir, PendingDir)
	if !dirExists(pend) {
		os.RemoveAll(filepath.Join(dataDir, UploadDir+".tmp"))
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
	rb := filepath.Join(dataDir, "restore-backup-"+ts)
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

	type move struct{ from, to string }
	var done []move
	undo := func() {
		for i := len(done) - 1; i >= 0; i-- {
			if err := os.Rename(done[i].to, done[i].from); err != nil {
				log.Error("restore rollback: could not move back", "from", done[i].to, "to", done[i].from, "err", err)
			}
		}
	}
	mv := func(from, to string) error {
		if err := os.Rename(from, to); err != nil {
			return err
		}
		done = append(done, move{from, to})
		return nil
	}
	for _, n := range aside {
		cur := filepath.Join(dataDir, n)
		if !exists(cur) {
			continue
		}
		if err := mv(cur, filepath.Join(rb, n)); err != nil {
			undo()
			return fail(err)
		}
	}
	entries, err := os.ReadDir(pend)
	if err != nil {
		undo()
		return fail(err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, e := range entries {
		n := e.Name()
		if n == ManifestName || n == ReadmeName {
			continue
		}
		if err := mv(filepath.Join(pend, n), filepath.Join(dataDir, n)); err != nil {
			undo()
			return fail(err)
		}
	}
	// Keep the applied manifest with the rollback copy for reference.
	_ = os.Rename(filepath.Join(pend, ManifestName), filepath.Join(rb, "applied-"+ManifestName))
	if err := os.RemoveAll(pend); err != nil {
		log.Error("could not remove the restore staging directory", "dir", pend, "err", err)
	}
	s := m.Summarize()
	log.Info("RESTORED FROM BACKUP", "backup_version", m.Version, "backup_created_at", m.CreatedAt, "db_engine", m.DBEngine,
		"files", s.Files, "previous_state", rb)
	return rb, nil
}
