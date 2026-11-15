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

// deployTimeout bounds a whole deployment (render, openresty -t of up to 30 s, the
// reload check of up to 15 s) once it no longer follows the caller's context.
const deployTimeout = 3 * time.Minute

// detach keeps a deployment going when its caller goes away (a console request whose
// client disconnects): once the gateway may have switched, the database must record
// it. Cancelling halfway would roll back the transaction after the gateway took the
// new configuration, record a killed `openresty -t` as a failed version, and let the
// next deployment reuse the rolled-back id (and directory) that is live.
func detach(ctx context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.WithoutCancel(ctx), deployTimeout)
}

// liveDir returns the version directory name the "current" symlink points to ("" if none).
func (d *Deployer) liveDir() string {
	target, err := os.Readlink(d.currentLink())
	if err != nil {
		return ""
	}
	return filepath.Base(target)
}

// removeVersionDir deletes a version directory unless the gateway is using it.
func (d *Deployer) removeVersionDir(name string) {
	if name == "" || name == d.liveDir() {
		return
	}
	_ = os.RemoveAll(filepath.Join(d.versionsDir(), name))
}

// newVersion inserts v and names its directory. An id whose directory is live (a
// deployment whose transaction was rolled back after the gateway switched to it) is
// skipped instead of being overwritten.
func (d *Deployer) newVersion(ctx context.Context, tx *store.Store, v *store.ConfigVersion) error {
	live := d.liveDir()
	for i := 0; i < 3; i++ {
		v.ID = 0
		if err := tx.CreateVersion(ctx, v); err != nil {
			return err
		}
		v.Dir = fmt.Sprintf("v%06d", v.ID)
		if v.Dir != live {
			return tx.SetVersionDir(ctx, v.ID, v.Dir)
		}
		slog.Warn("version id reused: its directory is serving traffic, skipping it", "dir", v.Dir)
		if err := tx.DeleteVersion(ctx, v.ID); err != nil {
			return err
		}
	}
	return fmt.Errorf("version directory %s is in use by the gateway", live)
}

// Apply runs mutate (may be nil) and deploys the result atomically. It runs to the end
// even if ctx is cancelled (see detach).
func (d *Deployer) Apply(ctx context.Context, by *int64, message string, mutate func(tx *store.Store) error) (*store.ConfigVersion, error) {
	ctx, cancel := detach(ctx)
	defer cancel()
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
		if err := d.newVersion(ctx, tx, v); err != nil {
			return err
		}
		dir := filepath.Join(d.versionsDir(), v.Dir)
		d.removeVersionDir(v.Dir) // a rolled-back attempt may have left the same id behind
		if err := d.Compiler.Render(ctx, tx, dir); err != nil {
			d.removeVersionDir(v.Dir)
			return err
		}
		out, err := d.Runner.Test(ctx, filepath.ToSlash(filepath.Join(dir, "nginx.conf")))
		if err != nil {
			d.removeVersionDir(v.Dir)
			testErr = &TestError{Output: out}
			return testErr
		}
		if err := d.activate(v.Dir); err != nil {
			d.removeVersionDir(v.Dir)
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
// Like Apply it runs to the end even if ctx is cancelled.
func (d *Deployer) Rollback(ctx context.Context, by *int64, targetID int64) (*store.ConfigVersion, error) {
	return d.rollback(ctx, by, targetID, fmt.Sprintf("rollback to #%d", targetID), false, "")
}

// ErrLiveChanged: the configuration the gateway failed with is no longer the live one (a
// deployment from the console meanwhile), so an automatic fallback is abandoned.
var ErrLiveChanged = errors.New("the active configuration changed meanwhile; automatic fallback abandoned")

// rollback re-activates targetID. automatic marks it as made by the control plane (see
// store.ConfigVersion.Automatic). With expectLive set it fails with ErrLiveChanged
// unless that version directory is still live.
func (d *Deployer) rollback(ctx context.Context, by *int64, targetID int64, msg string, automatic bool, expectLive string) (*store.ConfigVersion, error) {
	ctx, cancel := detach(ctx)
	defer cancel()
	d.mu.Lock()
	defer d.mu.Unlock()
	if expectLive != "" && d.liveDir() != expectLive {
		return nil, ErrLiveChanged
	}
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
	if err := d.activate(target.Dir); err != nil {
		// activate pointed the link back to the version still serving
		return nil, d.notAccepted(ctx, err, store.ConfigVersion{CreatedBy: by, Message: msg, RollbackOf: &targetID, ConfigRev: target.ConfigRev, Automatic: automatic})
	}
	v := &store.ConfigVersion{CreatedBy: by, Message: msg, RollbackOf: &targetID, Dir: target.Dir, ConfigRev: target.ConfigRev, Automatic: automatic}
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
//   - active version is automatic  → (a fallback or safe mode after the stored settings
//     could not be used) deploy the stored settings again: what made them fail (a port
//     held by another program) may be gone. If they are rejected the automatic version
//     stays; if the gateway does not start with them it falls back again.
//   - otherwise                   → re-render from the database so configuration
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
	if statErr == nil && v.RollbackOf != nil && !v.Automatic {
		slog.Info("keeping rolled-back configuration", "version", v.ID)
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.swapLink(v.Dir)
	}
	// Nothing changed since the active version was rendered (same settings, same
	// templates): keep it. Restarting does not create versions (and prune old ones).
	if statErr == nil && !v.Automatic && d.rendersSame(ctx, v.Dir) {
		d.mu.Lock()
		defer d.mu.Unlock()
		return d.swapLink(v.Dir)
	}
	msg := "startup re-render"
	if v.Automatic {
		slog.Info("an automatic fallback is active: trying the stored settings again", "version", v.ID)
		msg = "startup: stored settings tried again after an automatic fallback"
	}
	if _, err := d.Apply(ctx, nil, msg, nil); err != nil {
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

// Fallback makes an earlier configuration current when the active one cannot be used:
// EnsureCurrent failed at startup (for example a restored backup whose settings are
// rejected), or the gateway exits right after starting with it (failed: the version
// directory it failed with, for example because a port is taken, which `openresty -t`
// does not notice). It re-activates the newest earlier version whose files still pass
// the test, recorded as an automatic rollback, and otherwise the safe configuration
// (SafeMode); safe-mode versions are no candidates. The stored settings are not
// changed, so the console shows them as not deployed until the operator fixes and
// saves them, and the next start tries them again (EnsureCurrent).
//
// With failed set, the fallback is abandoned (ErrLiveChanged) once another version
// directory is live: an operator deployed meanwhile, and that deployment wins.
func (d *Deployer) Fallback(ctx context.Context, failed string, reason string) (*store.ConfigVersion, error) {
	ctx, cancel := detach(ctx)
	defer cancel()
	versions, err := d.Store.ListVersions(ctx, 100, 0)
	if err != nil {
		return nil, err
	}
	tried := map[string]bool{failed: true}
	for _, v := range versions {
		// ConfigRev < 0: safe mode, or a rollback to it (not an earlier configuration)
		if v.Status == "failed" || v.Dir == "" || IsSafeMode(&v) || tried[v.Dir] {
			continue
		}
		tried[v.Dir] = true
		target := v.ID
		if v.RollbackOf != nil {
			target = *v.RollbackOf
		}
		fv, err := d.rollback(ctx, nil, target, fmt.Sprintf("fallback to #%d (%s)", target, reason), true, failed)
		if err == nil {
			slog.Warn("fell back to an earlier configuration", "version", fv.ID, "of", target, "reason", reason)
			return fv, nil
		}
		if errors.Is(err, ErrLiveChanged) {
			return nil, err
		}
		slog.Warn("earlier configuration not usable", "version", target, "err", err)
	}
	return d.SafeMode(ctx, failed, reason)
}

// IsSafeMode reports whether v is a safe-mode version, or a rollback to one: its
// configuration revision never matches the stored settings (see SafeMode).
func IsSafeMode(v *store.ConfigVersion) bool { return v != nil && v.ConfigRev < 0 }

// SafeMode deploys the configuration of an empty installation (the default server
// only, no hosts or streams), recorded as a version that does not match the stored
// settings. It is the last resort that lets the gateway start after everything else
// was rejected; the console stays reachable to fix the settings. It is automatic (tried
// again at the next start, see EnsureCurrent) and, like Fallback, abandoned with
// ErrLiveChanged when failed is set and another version directory is live.
func (d *Deployer) SafeMode(ctx context.Context, failed string, reason string) (*store.ConfigVersion, error) {
	ctx, cancel := detach(ctx)
	defer cancel()
	d.mu.Lock()
	defer d.mu.Unlock()
	if failed != "" && d.liveDir() != failed {
		return nil, ErrLiveChanged
	}
	if err := os.MkdirAll(d.versionsDir(), 0o755); err != nil {
		return nil, err
	}
	tmp, err := os.MkdirTemp(d.Dir, "safe-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(tmp)
	empty, err := store.Open(filepath.Join(tmp, "empty.db"))
	if err != nil {
		return nil, err
	}
	defer empty.Close()

	// ConfigRev -1 never matches the stored settings: they show as not deployed.
	v := &store.ConfigVersion{Message: "safe mode: default server only (" + reason + ")", ConfigRev: -1, Automatic: true}
	if err := d.newVersion(ctx, d.Store, v); err != nil {
		return nil, err
	}
	fail := func(out string, err error) (*store.ConfigVersion, error) {
		d.removeVersionDir(v.Dir)
		if out == "" {
			out = err.Error()
		}
		_ = d.Store.FinishVersion(ctx, v.ID, false, out)
		return nil, err
	}
	dir := filepath.Join(d.versionsDir(), v.Dir)
	if err := d.Compiler.Render(ctx, empty, dir); err != nil {
		return fail("", err)
	}
	out, err := d.Runner.Test(ctx, filepath.ToSlash(filepath.Join(dir, "nginx.conf")))
	if err != nil {
		return fail(out, &TestError{VersionID: v.ID, Output: out})
	}
	if err := d.activate(v.Dir); err != nil {
		return fail("", err)
	}
	if err := d.Store.FinishVersion(ctx, v.ID, true, out); err != nil {
		return nil, err
	}
	v.Status, v.TestOutput = "active", out
	slog.Warn("SAFE MODE: the gateway runs the default server only; fix the settings in the console and save", "version", v.ID, "reason", reason)
	return v, nil
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
			d.removeVersionDir(e.Name())
		}
	}
	// History rows: as many deployed versions as there are directories (older ones
	// cannot be rolled back to anyway) and as many failed attempts.
	if n, err := d.Store.PruneVersions(ctx, keep, keep); err != nil {
		slog.Warn("pruning configuration version history", "err", err)
	} else if n > 0 {
		slog.Info("pruned configuration version history", "rows", n)
	}
}

// Dirty reports whether stored settings changed since the active version was rendered.
func (d *Deployer) Dirty(ctx context.Context) (bool, error) {
	rev, activeRev, hasActive, err := d.Store.DeployState(ctx)
	if err != nil {
		return false, err
	}
	return !hasActive || activeRev != rev, nil
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
