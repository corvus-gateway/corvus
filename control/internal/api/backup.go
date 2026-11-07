package api

import (
	"errors"
	"io"
	"io/fs"
	"log/slog"
	"net/http"
	"path/filepath"
	"sync"
	"time"

	"corvus/control/internal/backup"
	"corvus/control/internal/store"
)

// ------------------------------------------------------------------ backup & restore (admin session only)

const (
	maxBackupUpload  = 200 << 20 // compressed upload
	maxBackupExtract = 2 << 30   // uncompressed content
	restartCommand   = "docker compose restart"
)

var backupMu sync.Mutex // one backup/restore operation at a time

func (s *Server) dataDir() string { return filepath.FromSlash(s.Paths.DataDir) }

// extendDeadlines lifts the admin server's read/write timeouts for large transfers.
func extendDeadlines(w http.ResponseWriter, d time.Duration) {
	rc := http.NewResponseController(w)
	_ = rc.SetReadDeadline(time.Now().Add(d))
	_ = rc.SetWriteDeadline(time.Now().Add(d))
}

func (s *Server) downloadBackup(w http.ResponseWriter, r *http.Request) {
	if s.dataDir() == "" {
		writeErr(w, http.StatusServiceUnavailable, "unavailable", "data directory not configured")
		return
	}
	backupMu.Lock()
	defer backupMu.Unlock()
	now := time.Now()
	engine := s.Store.Dialect()
	opts := backup.CreateOptions{DataDir: s.dataDir(), Version: s.Version, Engine: engine, Now: now}
	if engine == store.SQLite {
		opts.Snapshot = s.Store.SnapshotTo
	}
	name := backup.FileName(s.Version, now)
	// Audited before the snapshot, so the backup contains its own download record.
	s.audit(r, "backup.download", name, map[string]any{"db_engine": engine})
	extendDeadlines(w, 30*time.Minute)
	h := w.Header()
	h.Set("Content-Type", "application/gzip")
	h.Set("Content-Disposition", `attachment; filename="`+name+`"`)
	h.Set("Cache-Control", "no-store")
	m, err := backup.Create(r.Context(), w, opts)
	if err != nil {
		// Headers (and possibly data) are already sent; the client sees a truncated archive.
		slog.Error("backup download failed", "err", err)
		return
	}
	slog.Info("backup downloaded", "file", name, "files", len(m.Files), "user", userFrom(r).Email)
}

type restoreStatus struct {
	State          string          `json:"state"` // "", "uploaded", "pending"
	Summary        *backup.Summary `json:"summary,omitempty"`
	CurrentEngine  string          `json:"current_engine"`
	CurrentVersion string          `json:"current_version"`
	RestartCommand string          `json:"restart_command,omitempty"`
}

func (s *Server) restoreStatus() restoreStatus {
	st := restoreStatus{CurrentEngine: s.Store.Dialect(), CurrentVersion: s.Version}
	state, m := backup.Status(s.dataDir())
	st.State = state
	if m != nil {
		sum := m.Summarize()
		st.Summary = &sum
	}
	if state == "pending" {
		st.RestartCommand = restartCommand
	}
	return st
}

func (s *Server) getRestore(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, 200, s.restoreStatus())
}

// uploadRestore validates a backup (multipart field "file") and stages it for confirmation.
func (s *Server) uploadRestore(w http.ResponseWriter, r *http.Request) {
	if s.dataDir() == "" {
		writeErr(w, http.StatusServiceUnavailable, "unavailable", "data directory not configured")
		return
	}
	extendDeadlines(w, 30*time.Minute)
	r.Body = http.MaxBytesReader(w, r.Body, maxBackupUpload+1<<20)
	mr, err := r.MultipartReader()
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "expected a multipart/form-data upload with a \"file\" field")
		return
	}
	var part *capReader
	for {
		p, err := mr.NextPart()
		if err != nil {
			writeErr(w, http.StatusBadRequest, "bad_request", "missing \"file\" field")
			return
		}
		if p.FormName() == "file" {
			part = &capReader{r: p, n: maxBackupUpload}
			break
		}
	}
	backupMu.Lock()
	defer backupMu.Unlock()
	m, err := backup.Stage(part, s.dataDir(), maxBackupExtract)
	if err != nil {
		var mbe *http.MaxBytesError
		switch {
		case part.over || errors.As(err, &mbe):
			writeErr(w, http.StatusRequestEntityTooLarge, "too_large", "the backup is larger than 200 MB")
		case errors.Is(err, backup.ErrInvalid):
			s.audit(r, "backup.restore_rejected", "", map[string]any{"reason": err.Error()})
			writeErr(w, http.StatusUnprocessableEntity, "invalid_backup", err.Error())
		default:
			fail(w, err)
		}
		return
	}
	if m.DBEngine != s.Store.Dialect() {
		_ = backup.DiscardUpload(s.dataDir())
		writeErr(w, http.StatusUnprocessableEntity, "invalid_backup",
			"the backup was made with the "+m.DBEngine+" database engine, but this installation uses "+s.Store.Dialect())
		return
	}
	s.audit(r, "backup.restore_upload", m.Version, map[string]any{"created_at": m.CreatedAt, "files": len(m.Files)})
	writeJSON(w, 200, s.restoreStatus())
}

// confirmRestore marks the uploaded backup to be applied on the next start.
func (s *Server) confirmRestore(w http.ResponseWriter, r *http.Request) {
	backupMu.Lock()
	defer backupMu.Unlock()
	m, err := backup.Confirm(s.dataDir())
	if err != nil {
		switch {
		case errors.Is(err, fs.ErrNotExist):
			writeErr(w, http.StatusNotFound, "not_found", "no uploaded backup to confirm")
		case errors.Is(err, backup.ErrInvalid):
			writeErr(w, http.StatusUnprocessableEntity, "invalid_backup", err.Error())
		default:
			fail(w, err)
		}
		return
	}
	s.audit(r, "backup.restore_confirm", m.Version, map[string]any{"created_at": m.CreatedAt})
	slog.Warn("restore from backup staged; it is applied on the next restart", "backup_version", m.Version, "backup_created_at", m.CreatedAt)
	writeJSON(w, 200, s.restoreStatus())
}

func (s *Server) discardRestore(w http.ResponseWriter, r *http.Request) {
	backupMu.Lock()
	defer backupMu.Unlock()
	if err := backup.Discard(s.dataDir()); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "backup.restore_discard", "", nil)
	w.WriteHeader(http.StatusNoContent)
}

// capReader fails once more than n bytes were read and remembers that it did.
type capReader struct {
	r    io.Reader
	n    int64
	over bool
}

func (c *capReader) Read(p []byte) (int, error) {
	if c.n <= 0 {
		c.over = true
		return 0, errors.New("upload too large")
	}
	if int64(len(p)) > c.n {
		p = p[:c.n+1] // read one byte past the limit to detect oversize uploads
	}
	n, err := c.r.Read(p)
	c.n -= int64(n)
	if c.n < 0 {
		c.over = true
		return 0, errors.New("upload too large")
	}
	return n, err
}
