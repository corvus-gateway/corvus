// Package deploy applies configuration changes safely:
//
//	change (in a DB transaction) → render new version dir → openresty -t
//	→ atomically repoint the "current" symlink → reload → commit
//
// If any step fails the transaction is rolled back, the live configuration is
// untouched and a failed version is recorded with the test output.
package deploy

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/compiler"
	"corvus/control/internal/gateway"
	"corvus/control/internal/store"
)

// TestError is returned when the generated configuration is rejected.
type TestError struct {
	VersionID int64
	Output    string
}

func (e *TestError) Error() string { return "configuration rejected: " + e.Output }

// ReloadError: the configuration passed the test but the gateway kept serving the
// previous one after the reload (gateway.NotAcceptedError). The attempt is recorded as
// a failed version (VersionID, 0 if recording failed).
type ReloadError struct {
	VersionID int64
	Err       *gateway.NotAcceptedError
}

func (e *ReloadError) Error() string { return e.Err.Error() }
func (e *ReloadError) Unwrap() error { return e.Err }

// notAccepted records a reload the gateway did not take as a failed version and
// returns it as a ReloadError; other errors are returned unchanged.
func (d *Deployer) notAccepted(ctx context.Context, err error, v store.ConfigVersion) error {
	var na *gateway.NotAcceptedError
	if !errors.As(err, &na) {
		return err
	}
	v.Status, v.TestOutput = "failed", na.Error()
	re := &ReloadError{Err: na}
	if e := d.Store.CreateVersion(ctx, &v); e == nil {
		re.VersionID = v.ID
	}
	return re
}

type Deployer struct {
	Store    *store.Store
	Compiler *compiler.Compiler
	Runner   gateway.Runner
	Dir      string // e.g. /data/nginx  (versions/ and current live here)
	Keep     int    // number of recent version directories to keep

	mu sync.Mutex
}

func (d *Deployer) versionsDir() string { return filepath.Join(d.Dir, "versions") }

// versionDirName matches the directory names Apply gives versions ("v%06d").
var versionDirName = regexp.MustCompile(`^v[0-9]{6,19}$`)

// ErrBadVersionDir is returned for a stored version directory name that Apply
// could not have produced (for example from an edited database).
var ErrBadVersionDir = errors.New("invalid version directory")

// VersionPath returns the directory of a version, refusing names other than the
// ones Apply generates and any path that would resolve outside versions/.
func (d *Deployer) VersionPath(name string) (string, error) {
	if !versionDirName.MatchString(name) {
		return "", ErrBadVersionDir
	}
	base := filepath.Clean(d.versionsDir())
	p := filepath.Join(base, name)
	if rel, err := filepath.Rel(base, p); err != nil || rel != name {
		return "", ErrBadVersionDir
	}
	return p, nil
}
func (d *Deployer) currentLink() string { return filepath.Join(d.Dir, "current") }

// CurrentConf is the nginx.conf path the gateway is started with.
func (d *Deployer) CurrentConf() string {
	return filepath.ToSlash(filepath.Join(d.currentLink(), "nginx.conf"))
}

// Apply runs mutate (may be nil) and deploys the result atomically.
func (d *Deployer) Apply(ctx context.Context, by *int64, message string, mutate func(tx *store.Store) error) (*store.ConfigVersion, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if err := os.MkdirAll(d.versionsDir(), 0o755); err != nil {
		return nil, err
	}

	var version *store.ConfigVersion
	var testErr *TestError
	err := d.Store.WithTx(ctx, func(tx *store.Store) error {
		rev, err := tx.ConfigRev(ctx)
		if err != nil {
			return err
		}
		if mutate != nil {
			if err := mutate(tx); err != nil {
				return err
			}
			if rev, err = tx.BumpConfigRev(ctx); err != nil {
				return err
			}
		}
		v := &store.ConfigVersion{CreatedBy: by, Message: message, ConfigRev: rev}
		if err := tx.CreateVersion(ctx, v); err != nil {
			return err
		}
		v.Dir = fmt.Sprintf("v%06d", v.ID)
		if err := tx.SetVersionDir(ctx, v.ID, v.Dir); err != nil {
			return err
		}
		dir := filepath.Join(d.versionsDir(), v.Dir)
		_ = os.RemoveAll(dir) // a rolled-back attempt may have left the same id behind
		if err := d.Compiler.Render(ctx, tx, dir); err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
		out, err := d.Runner.Test(ctx, filepath.ToSlash(filepath.Join(dir, "nginx.conf")))
		if err != nil {
			_ = os.RemoveAll(dir)
			testErr = &TestError{Output: out}
			return testErr
		}
		if err := d.activate(v.Dir); err != nil {
			_ = os.RemoveAll(dir)
			return err
		}
		if err := tx.FinishVersion(ctx, v.ID, true, out); err != nil {
			return err
		}
		v.Status, v.TestOutput = "active", out
		version = v
		return nil
	})
	if err != nil {
		if testErr != nil {
			// Record the rejected attempt outside the rolled-back transaction.
			rev, _ := d.Store.ConfigRev(ctx)
			fv := &store.ConfigVersion{CreatedBy: by, Message: message, Status: "failed", TestOutput: testErr.Output, ConfigRev: rev}
			if e := d.Store.CreateVersion(ctx, fv); e == nil {
				testErr.VersionID = fv.ID
			}
		}
		if testErr == nil {
			// the reload did not take effect (e.g. a port in use): record it like a rejected test
			rev, _ := d.Store.ConfigRev(ctx)
			err = d.notAccepted(ctx, err, store.ConfigVersion{CreatedBy: by, Message: message, ConfigRev: rev})
		}
		return nil, err
	}
	d.prune(ctx)
	return version, nil
}

// Rollback re-activates the generated configuration of an earlier version.
// The database is not changed, so the next Apply renders the current settings again.
func (d *Deployer) Rollback(ctx context.Context, by *int64, targetID int64) (*store.ConfigVersion, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	target, err := d.Store.VersionByID(ctx, targetID)
	if err != nil {
		return nil, err
	}
	if target.Status == "failed" || target.Dir == "" {
		return nil, errors.New("this version was never deployed")
	}
	dir, err := d.VersionPath(target.Dir)
	if err != nil {
		return nil, err
	}
	if _, err := os.Stat(dir); err != nil {
		return nil, errors.New("configuration files of this version were pruned")
	}
	out, err := d.Runner.Test(ctx, filepath.ToSlash(filepath.Join(dir, "nginx.conf")))
	if err != nil {
		return nil, &TestError{Output: out}
	}
	msg := fmt.Sprintf("rollback to #%d", targetID)
	if err := d.activate(target.Dir); err != nil {
		// activate pointed the link back to the version still serving
		return nil, d.notAccepted(ctx, err, store.ConfigVersion{CreatedBy: by, Message: msg, RollbackOf: &targetID, ConfigRev: target.ConfigRev})
	}
	v := &store.ConfigVersion{CreatedBy: by, Message: msg, RollbackOf: &targetID, Dir: target.Dir, ConfigRev: target.ConfigRev}
	if err := d.Store.CreateVersion(ctx, v); err != nil {
		return nil, err
	}
	if err := d.Store.FinishVersion(ctx, v.ID, true, out); err != nil {
		return nil, err
	}
	v.Status, v.TestOutput = "active", out
	return v, nil
}

// EnsureCurrent prepares the "current" symlink at startup, before the gateway starts.
//   - no active version            → initial deployment
//   - active version is a rollback → keep it (the operator chose that configuration)
//   - otherwise                    → re-render from the database so configuration
//     templates always match this binary (upgrades); fall back to the existing
//     version if the fresh render is rejected.
func (d *Deployer) EnsureCurrent(ctx context.Context) error {
	v, err := d.Store.ActiveVersion(ctx)
	if errors.Is(err, store.ErrNotFound) {
		_, err = d.Apply(ctx, nil, "initial deployment", nil)
		return err
	}
	if err != nil {
		return err
	}
	vdir, statErr := d.VersionPath(v.Dir)
	if statErr == nil {
		_, statErr = os.Stat(filepath.Join(vdir, "nginx.conf"))
	}
	if statErr == nil && v.RollbackOf != nil {
		slog.Info("keeping rolled-back configuration", "version", v.ID)
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.swapLink(v.Dir)
	}
	// Nothing changed since the active version was rendered (same settings, same
	// templates): keep it. Restarting does not create versions (and prune old ones).
	if statErr == nil && d.rendersSame(ctx, v.Dir) {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.swapLink(v.Dir)
	}
	if _, err := d.Apply(ctx, nil, "startup re-render", nil); err != nil {
		if statErr != nil {
			return err
		}
		slog.Warn("startup re-render rejected, keeping previous version", "version", v.ID, "err", err)
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.swapLink(v.Dir)
	}
	return nil
}

// waitActiveTimeout bounds the wait for the gateway to report a reloaded version. Apply
// waits inside the store transaction (one SQLite connection: every other write waits
// too), so it stays short; a timeout is only logged, the deployment still counts.
const waitActiveTimeout = 15 * time.Second

// activate repoints the symlink and reloads; on reload failure the old target is restored.
func (d *Deployer) activate(dirName string) error {
	prev, _ := os.Readlink(d.currentLink())
	if err := d.swapLink(dirName); err != nil {
		return err
	}
	if err := d.Runner.Reload(); err != nil {
		if prev != "" {
			_ = d.swapLink(filepath.Base(prev))
		}
		return fmt.Errorf("reload failed: %w", err)
	}
	// Reload is asynchronous; report success only once the new version serves traffic.
	if w, ok := d.Runner.(interface {
		WaitActive(ctx context.Context, name string, timeout time.Duration) error
	}); ok {
		if err := w.WaitActive(context.Background(), dirName, waitActiveTimeout); err != nil {
			var na *gateway.NotAcceptedError
			if errors.As(err, &na) {
				// nginx keeps running the old configuration: point the link back to it
				if prev != "" {
					_ = d.swapLink(filepath.Base(prev))
				}
				return err
			}
			slog.Warn("reload confirmation timed out", "version", dirName, "err", err)
		}
	}
	return nil
}

// swapLink atomically replaces the "current" symlink (rename over the old one).
func (d *Deployer) swapLink(dirName string) error {
	if _, err := d.VersionPath(dirName); err != nil {
		return err
	}
	tmp := d.currentLink() + ".tmp"
	_ = os.Remove(tmp)
	if err := os.Symlink(filepath.Join("versions", dirName), tmp); err != nil {
		return err
	}
	return os.Rename(tmp, d.currentLink())
}

func (d *Deployer) prune(ctx context.Context) {
	keep := d.Keep
	if keep <= 0 {
		keep = 20
	}
	inUse, err := d.Store.VersionDirsInUse(ctx, keep)
	if err != nil {
		return
	}
	entries, err := os.ReadDir(d.versionsDir())
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() && strings.HasPrefix(e.Name(), "v") && !inUse[e.Name()] {
			_ = os.RemoveAll(filepath.Join(d.versionsDir(), e.Name()))
		}
	}
}

// Dirty reports whether stored settings changed since the active version was rendered.
func (d *Deployer) Dirty(ctx context.Context) (bool, error) {
	rev, err := d.Store.ConfigRev(ctx)
	if err != nil {
		return false, err
	}
	v, err := d.Store.ActiveVersion(ctx)
	if errors.Is(err, store.ErrNotFound) {
		return true, nil
	}
	if err != nil {
		return false, err
	}
	return v.ConfigRev != rev, nil
}

// rendersSame reports whether rendering the stored settings now gives exactly the files
// of version directory dirName.
func (d *Deployer) rendersSame(ctx context.Context, dirName string) bool {
	tmp, err := os.MkdirTemp(d.Dir, "render-check-")
	if err != nil {
		return false
	}
	defer os.RemoveAll(tmp)
	out := filepath.Join(tmp, dirName) // same name: the gateway reports it as its version
	if err := d.Compiler.Render(ctx, d.Store, out); err != nil {
		return false
	}
	return sameTree(out, filepath.Join(d.versionsDir(), dirName))
}

// sameTree compares two directory trees file by file (names and contents).
func sameTree(a, b string) bool {
	files := map[string][]byte{}
	err := filepath.WalkDir(a, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(a, p)
		data, err := os.ReadFile(p)
		// rendered files name their own directory: compare as if rendered into b
		for _, form := range []func(string) string{filepath.ToSlash, filepath.FromSlash} {
			data = bytes.ReplaceAll(data, []byte(form(a)), []byte(form(b)))
		}
		files[rel] = data
		return err
	})
	if err != nil {
		return false
	}
	n := 0
	err = filepath.WalkDir(b, func(p string, e fs.DirEntry, err error) error {
		if err != nil || e.IsDir() {
			return err
		}
		rel, _ := filepath.Rel(b, p)
		want, ok := files[rel]
		got, rerr := os.ReadFile(p)
		if !ok || rerr != nil || !bytes.Equal(want, got) {
			return errDiffers
		}
		n++
		return nil
	})
	return err == nil && n == len(files)
}

var errDiffers = errors.New("differs")

// RollbackActive reports whether the active version is a rollback.
func (d *Deployer) RollbackActive(ctx context.Context) bool {
	v, err := d.Store.ActiveVersion(ctx)
	return err == nil && v.RollbackOf != nil
}

// Refresh redeploys after a background change (certificate issued, country database or
// rule bundle updated). While a rollback is active it only reloads the gateway, so the
// operator's rollback stays in place and renewed certificate files are still picked up.
func (d *Deployer) Refresh(ctx context.Context, message string) error {
	if d.RollbackActive(ctx) {
		slog.Warn("rollback active: not redeploying the stored settings", "reason", message)
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.Runner.Reload()
	}
	_, err := d.Apply(ctx, nil, message, nil)
	if err != nil {
		// the stored settings did not deploy: still reload the active configuration so
		// renewed certificate files are picked up (renewal does not retry once the new
		// expiry date is stored)
		d.mu.Lock()
		if rerr := d.Runner.Reload(); rerr != nil {
			slog.Error("gateway reload after failed redeploy", "err", rerr)
		}
		d.mu.Unlock()
	}
	return err
}
