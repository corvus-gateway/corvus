package signin

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"time"
)

// Serve answers the gateway on a Unix socket at path until ctx ends. Only the gateway may
// connect (same rule as the access-log socket): owner-only when Corvus runs as its own
// user, root + group nobody (the nginx workers) in the legacy root mode.
func (s *Service) Serve(ctx context.Context, path string) error {
	_ = os.Remove(path)
	l, err := net.Listen("unix", path)
	if err != nil {
		return err
	}
	if os.Getuid() != 0 {
		_ = os.Chmod(path, 0o600)
	} else if err := os.Chown(path, 0, 65534); err == nil {
		_ = os.Chmod(path, 0o660)
	}
	srv := &http.Server{Handler: s.Handler(), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 10 * time.Second,
		WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}
	go func() {
		<-ctx.Done()
		sctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}()
	go func() {
		if err := srv.Serve(l); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("sign-in socket", "err", err)
		}
	}()
	return nil
}
