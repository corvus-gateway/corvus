package compiler

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

type streamView struct {
	store.Stream
	Upstream        string // forward host address (see upstreamAddr)
	UpstreamResolve bool   // Upstream is a name: resolved at run time
	CertDir         string // TLS termination ("" = plain TCP)
	SkipTCP         bool   // TLS requested but the certificate files don't exist yet
}

// StreamConflict reports why st cannot listen on its port: the port is used by Corvus
// itself, or another enabled stream already listens on it with the same protocol.
// "" = no conflict. Disabled streams don't listen and never conflict.
func StreamConflict(p Paths, others []store.Stream, st *store.Stream) string {
	if what, ok := p.ReservedPorts()[st.IncomingPort]; ok {
		return fmt.Sprintf("port %d is used by Corvus (%s)", st.IncomingPort, what)
	}
	if !st.Enabled {
		return ""
	}
	for _, o := range others {
		if o.ID == st.ID || !o.Enabled || o.IncomingPort != st.IncomingPort {
			continue
		}
		if (o.TCP && st.TCP) || (o.UDP && st.UDP) {
			return fmt.Sprintf("port %d is already used by stream #%d", st.IncomingPort, o.ID)
		}
	}
	return ""
}

// renderStreams writes streams/<id>.conf for every enabled stream and returns how many
// were written (0 = no stream {} block).
func (c *Compiler) renderStreams(ctx context.Context, s *store.Store, dir string, static map[string]string, hostGW string) (int, error) {
	streams, err := s.ListStreams(ctx)
	if err != nil {
		return 0, err
	}
	n := 0
	for _, st := range streams {
		if !st.Enabled {
			continue
		}
		sc := st
		// Defense in depth: never render a stream that would not pass validation.
		if err := validate.Stream(&sc); err != nil {
			return 0, fmt.Errorf("stream %d: %w", st.ID, err)
		}
		if why := StreamConflict(c.P, streams, &sc); why != "" {
			return 0, fmt.Errorf("stream %d: %s", st.ID, why)
		}
		if n == 0 {
			if err := os.MkdirAll(filepath.Join(dir, "streams"), 0o755); err != nil {
				return 0, err
			}
		}
		v := streamView{Stream: sc, Upstream: upstreamAddr(sc.ForwardHost, static, hostGW)}
		v.UpstreamResolve = !isIP(v.Upstream)
		if sc.CertificateID != nil {
			// Never fall back to plain text: the TCP listener waits for the certificate.
			v.CertDir = c.certDir(sc.CertificateID)
			v.SkipTCP = v.CertDir == ""
		}
		if err := writeTemplate(filepath.Join(dir, "streams", fmt.Sprintf("%d.conf", st.ID)), "stream.conf.tmpl", v); err != nil {
			return 0, err
		}
		n++
	}
	return n, nil
}
