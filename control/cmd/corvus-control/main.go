// Command corvus-control is the Corvus control plane. In the corvus container it is
// PID 1: it owns the configuration database, renders and deploys gateway
// configuration, supervises the OpenResty data plane and serves the admin UI/API.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"encoding/base64"

	"corvus/control/internal/acme"
	"corvus/control/internal/alerts"
	"corvus/control/internal/api"
	"corvus/control/internal/auth"
	"corvus/control/internal/autoban"
	"corvus/control/internal/backup"
	"corvus/control/internal/certs"
	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
	"corvus/control/internal/feeds"
	"corvus/control/internal/gateway"
	"corvus/control/internal/geoip"
	"corvus/control/internal/history"
	"corvus/control/internal/live"
	"corvus/control/internal/netcfg"
	"corvus/control/internal/sigs"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
	"corvus/control/internal/underattack"
	"corvus/control/internal/webui"
)

var version = "dev"

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

// defaultResolver picks the DNS server nginx uses for forward hosts: Docker's embedded
// DNS (127.0.0.11) when the container is on a Docker network, otherwise the first
// nameserver of resolv.conf (e.g. 127.0.0.53 with network_mode: host).
func defaultResolver(resolvConf string) string {
	b, err := os.ReadFile(resolvConf)
	if err != nil {
		return "127.0.0.11"
	}
	first := ""
	for _, line := range strings.Split(string(b), "\n") {
		f := strings.Fields(line)
		if len(f) < 2 || f[0] != "nameserver" {
			continue
		}
		ip := net.ParseIP(f[1])
		if ip == nil {
			continue
		}
		if f[1] == "127.0.0.11" {
			return f[1]
		}
		if first == "" {
			if ip.To4() == nil {
				first = "[" + ip.String() + "]"
			} else {
				first = ip.String()
			}
		}
	}
	if first == "" {
		return "127.0.0.11"
	}
	return first
}

// ipv6Settings turns CORVUS_IPV6 into the gateway's IPv6 behaviour:
//   - "auto" (default): listen on IPv6 when the system has it; the resolver keeps
//     returning IPv4 addresses only.
//   - "on": always listen on IPv6, and let the resolver return IPv6 (AAAA) addresses
//     for forward hosts (IPv6-only upstreams).
//   - "off": IPv4 only.
//
// Docker bridge networks usually have IPv6 disabled; nginx then refuses to start if it
// is told to listen on [::], so "auto" checks first.
func ipv6Settings(mode string, available func() bool) (listen, resolver bool) {
	switch strings.ToLower(strings.TrimSpace(mode)) {
	case "on", "1", "true", "yes":
		return true, true
	case "off", "0", "false", "no":
		return false, false
	}
	return available(), false
}

// ipv6Available reports whether an interface has an IPv6 address and an IPv6 socket
// can be bound.
func ipv6Available() bool {
	b, err := os.ReadFile("/proc/net/if_inet6")
	if err != nil || len(strings.TrimSpace(string(b))) == 0 {
		return false
	}
	l, err := net.Listen("tcp6", "[::]:0")
	if err != nil {
		return false
	}
	l.Close()
	return true
}

func envBool(key string, def bool) bool {
	v, err := strconv.ParseBool(env(key, strconv.FormatBool(def)))
	if err != nil {
		return def
	}
	return v
}

func main() {
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("corvus-control stopped", "err", err)
		os.Exit(1)
	}
}

// secret returns a persistent random secret stored at path (created on first use).
func secret(path string) (string, error) {
	if b, err := os.ReadFile(path); err == nil && len(strings.TrimSpace(string(b))) >= 32 {
		return strings.TrimSpace(string(b)), nil
	}
	v := auth.RandomToken(32)
	if err := os.WriteFile(path, []byte(v+"\n"), 0o600); err != nil {
		return "", err
	}
	return v, nil
}

func run() error {
	dataDir := env("CORVUS_DATA_DIR", "/data")
	withGateway := envBool("CORVUS_GATEWAY", true)
	// A restore confirmed in the console is applied now, before anything opens the data.
	// On failure the previous state stays in place and startup continues with it.
	if rb, err := backup.ApplyPending(dataDir, slog.Default(), time.Now()); err != nil {
		slog.Error("staged restore was not applied", "err", err)
	} else if rb != "" {
		slog.Warn("configuration restored from backup; the replaced files are kept for a manual rollback", "dir", rb)
	}
	// The image runs as the unprivileged "corvus" user (H9); nginx master and workers
	// share it. certs/ and secrets/ stay owner-only.
	for d, mode := range map[string]os.FileMode{"nginx": 0o755, "run": 0o755, "certs": 0o750, "secrets": 0o700} {
		p := filepath.Join(dataDir, d)
		if err := os.MkdirAll(p, mode); err != nil {
			return err
		}
		_ = os.Chmod(p, mode)
	}

	wrSecret, err := secret(filepath.Join(dataDir, "secrets", "waitroom_hmac"))
	if err != nil {
		return err
	}
	debugKey := ""
	if envBool("CORVUS_DEBUG_ENDPOINTS", false) {
		if debugKey, err = secret(filepath.Join(dataDir, "secrets", "debug_key")); err != nil {
			return err
		}
		slog.Warn("access queue debug endpoints (stats/reset) are ENABLED; key in secrets/debug_key")
	}

	paths := compiler.Paths{
		DataDir:     filepath.ToSlash(dataDir),
		LuaDir:      env("CORVUS_LUA_DIR", "/opt/corvus/lua"),
		HTMLDir:     env("CORVUS_HTML_DIR", "/opt/corvus/html"),
		MimeTypes:   env("CORVUS_MIME_TYPES", "/usr/local/openresty/nginx/conf/mime.types"),
		Resolver:    env("CORVUS_RESOLVER", defaultResolver("/etc/resolv.conf")),
		HTTPPort:    80,
		HTTPSPort:   443,
		StatusPort:  9180,
		ACMEPort:    9181,
		DefaultCert: filepath.ToSlash(filepath.Join(dataDir, "certs", "default")),
	}
	paths.IPv6, paths.ResolverIPv6 = ipv6Settings(env("CORVUS_IPV6", "auto"), ipv6Available)
	slog.Info("gateway IPv6", "listen", paths.IPv6, "resolver_aaaa", paths.ResolverIPv6)
	adminHosts := []string{"localhost", "127.0.0.1"}
	for _, h := range strings.Split(env("CORVUS_ADMIN_HOSTS", ""), ",") {
		if h = strings.TrimSpace(h); h != "" {
			adminHosts = append(adminHosts, h)
		}
	}
	if err := certs.EnsureSelfSigned(paths.DefaultCert, "corvus-default", adminHosts); err != nil {
		return fmt.Errorf("default certificate: %w", err)
	}

	// CORVUS_DB_URL=postgres://... selects PostgreSQL (multi-node / HA); default is SQLite in the data volume.
	st, err := store.Open(env("CORVUS_DB_URL", filepath.Join(dataDir, "corvus.db")))
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()
	slog.Info("configuration database", "engine", st.Dialect())

	redisScript := env("CORVUS_REDIS_SCRIPT", "/opt/corvus/redis/waitroom.lua")
	// Access-queue store. Without CORVUS_REDIS_HOST the image runs its own Redis-compatible
	// server (Valkey) on loopback, so a single container is a complete install; set
	// CORVUS_REDIS_HOST to use an external Redis instead (shared by several gateways).
	redisHost, redisPort := env("CORVUS_REDIS_HOST", ""), env("CORVUS_REDIS_PORT", "6379")
	embeddedRedis := redisHost == "" && withGateway
	// Ports Corvus listens on itself may not be taken by streams (TCP/UDP forwarding).
	adminListen := env("CORVUS_ADMIN_LISTEN", ":9443")
	if _, port, err := net.SplitHostPort(adminListen); err == nil {
		paths.AdminPort, _ = strconv.Atoi(port)
	}
	if embeddedRedis {
		paths.RedisPort, _ = strconv.Atoi(redisPort)
	}
	if redisHost == "" {
		redisHost = "127.0.0.1"
	}
	// Redis authentication (H12). The bundled Redis loads an ACL file that we generate
	// (embedded: in the data directory; separate container: a shared volume); an
	// external Redis takes CORVUS_REDIS_USER/PASSWORD.
	redisUser, redisPass := env("CORVUS_REDIS_USER", "corvus"), env("CORVUS_REDIS_PASSWORD", "")
	if redisPass == "" {
		if redisPass, err = secret(filepath.Join(dataDir, "secrets", "redis_password")); err != nil {
			return err
		}
	}
	aclPath := env("CORVUS_REDIS_ACL_FILE", "")
	if embeddedRedis && aclPath == "" {
		aclPath = filepath.Join(dataDir, "run", "redis", "users.acl")
	}
	if aclPath != "" {
		if err := live.WriteACL(aclPath, redisUser, redisPass); err != nil {
			return fmt.Errorf("write redis ACL file: %w", err)
		}
	}

	dataKeyB64, err := secret(filepath.Join(dataDir, "secrets", "data_key"))
	if err != nil {
		return err
	}
	dataKey, err := base64.RawURLEncoding.DecodeString(dataKeyB64)
	if err != nil || len(dataKey) != 32 {
		return fmt.Errorf("invalid data key in secrets/data_key")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// Access-log collector must listen before the gateway starts writing to it.
	tc := traffic.New()
	tc.MaskIPs = func() bool { n, _ := st.Network(context.Background()); return n.MaskIPs }
	if err := tc.Listen(filepath.Join(dataDir, "run", "corvus-access.sock"), ctx.Done()); err != nil {
		slog.Warn("traffic collector disabled", "err", err)
	}

	// Managed rule bundle: body signatures + virtual-patch templates (signed updates).
	rulesSvc := &sigs.Service{Dir: filepath.Join(dataDir, "rules"), Store: st}
	if err := rulesSvc.Init(); err != nil {
		return err
	}

	comp := &compiler.Compiler{P: paths, Bundle: rulesSvc.Bundle, Cloudflare: func(ctx context.Context, s *store.Store) []string {
		cf := netcfg.Cloudflare(ctx, s)
		return append(append([]string{}, cf.IPv4...), cf.IPv6...)
	}, Hosts: func() map[string]string { return compiler.StaticHosts(env("CORVUS_HOSTS_FILE", "/etc/hosts")) },
		// 127.0.0.1/localhost forward hosts mean the Docker host (as with NPM in host mode);
		// CORVUS_LOOPBACK=container keeps them pointing at the container itself.
		HostGateway: func() string {
			if env("CORVUS_LOOPBACK", "host") == "container" {
				return ""
			}
			return compiler.HostGateway(env("CORVUS_HOSTS_FILE", "/etc/hosts"), "/proc/net/dev", "/proc/net/route")
		}}
	var runner gateway.Runner = gateway.Noop{}
	var or *gateway.OpenResty
	dep := &deploy.Deployer{Store: st, Compiler: comp, Dir: filepath.Join(dataDir, "nginx"), Keep: 20}
	if withGateway {
		or = &gateway.OpenResty{
			Bin:         env("CORVUS_OPENRESTY_BIN", "/usr/local/openresty/bin/openresty"),
			CurrentConf: dep.CurrentConf(),
			StatusURL:   "http://127.0.0.1:9180/version",
			Env: []string{
				"CORVUS_WR_SECRET=" + wrSecret, "CORVUS_DEBUG_KEY=" + debugKey,
				"CORVUS_REDIS_HOST=" + redisHost, "CORVUS_REDIS_PORT=" + redisPort, "CORVUS_REDIS_SCRIPT=" + redisScript,
				"CORVUS_REDIS_USER=" + redisUser, "CORVUS_REDIS_PASSWORD=" + redisPass,
			},
		}
		runner = or
	}
	dep.Runner = runner

	// One-time: NPM-style access-list IP rules → security rules (access lists keep Basic auth).
	if n, err := st.MigrateAccessListIPRules(ctx); err != nil {
		return fmt.Errorf("migrate access list IP rules: %w", err)
	} else if n > 0 {
		slog.Info("access list IP rules converted to security rules", "rules", n)
	}

	if err := dep.EnsureCurrent(ctx); err != nil {
		return fmt.Errorf("initial deployment: %w", err)
	}
	var rd *gateway.Redis
	if embeddedRedis {
		rd = &gateway.Redis{Bin: env("CORVUS_REDIS_BIN", "/usr/bin/valkey-server"), Port: redisPort, ACLFile: aclPath,
			Dir: filepath.Join(dataDir, "run", "redis"), MaxMemory: env("CORVUS_REDIS_MAXMEMORY", "256mb")}
		if err := rd.Start(ctx); err != nil {
			return fmt.Errorf("start embedded redis: %w", err)
		}
	}
	gatewayDone := make(<-chan error)
	if or != nil {
		if err := or.Start(); err != nil {
			return fmt.Errorf("start gateway: %w", err)
		}
		gatewayDone = or.Done()
	}

	lv, err := live.New(net.JoinHostPort(redisHost, redisPort), redisUser, redisPass, redisScript)
	if err != nil {
		slog.Warn("live access queue stats disabled", "err", err)
	}

	setup := auth.NewSetupToken()
	preset := os.Getenv("CORVUS_SETUP_TOKEN")
	if preset != "" {
		if setup, err = auth.PresetSetupToken(preset); err != nil {
			return fmt.Errorf("CORVUS_SETUP_TOKEN: %w", err)
		}
	}
	if n, _ := st.CountUsers(ctx); n == 0 {
		if preset != "" {
			slog.Warn("no administrator yet — open the admin UI and enter the setup token from CORVUS_SETUP_TOKEN")
		} else {
			slog.Warn("no administrator yet — open the admin UI and enter this setup token", "setup_token", setup.Value())
		}
	}

	acmeSvc := &acme.Service{Store: st, Key: dataKey, CertDir: paths.CertDir, Directory: env("CORVUS_ACME_DIRECTORY", ""),
		DirectorySuffix: env("CORVUS_ACME_DIRECTORY_SUFFIX", ""),
		OnIssued: func(ctx context.Context) {
			if _, err := dep.Apply(ctx, nil, "certificate issued/renewed", nil); err != nil {
				slog.Error("redeploy after certificate issuance failed", "err", err)
			}
		}}
	acmeSrv := &http.Server{Addr: fmt.Sprintf("127.0.0.1:%d", paths.ACMEPort), Handler: acmeSvc.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		if err := acmeSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("ACME challenge server", "err", err)
		}
	}()
	// Per-certificate DNS secrets of older versions become saved DNS credentials.
	if _, err := acmeSvc.AdoptLegacyCredentials(ctx); err != nil {
		slog.Warn("moving per-certificate DNS credentials failed; they keep working as they are", "err", err)
	}
	go acmeSvc.RenewLoop(ctx)

	// Country database for security rules (DB-IP Lite, monthly). CORVUS_GEOIP=off disables
	// downloads; CORVUS_GEOIP_URL points at a mirror (air-gapped installs).
	geo := &geoip.Service{Dir: filepath.Join(dataDir, "geoip"), URL: env("CORVUS_GEOIP_URL", ""),
		Disabled: strings.EqualFold(env("CORVUS_GEOIP", "on"), "off"),
		OnUpdated: func(ctx context.Context) {
			if _, err := dep.Apply(ctx, nil, "country database updated", nil); err != nil {
				slog.Error("redeploy after country database update failed", "err", err)
			}
		}}
	go geo.Run(ctx)

	// Traffic history: statistics/events survive restarts; request logs are archived
	// per day (compressed) with retention by age and total size.
	hist := &history.Service{Dir: filepath.Join(dataDir, "history"), Store: st, Collector: tc}
	if err := hist.Open(ctx); err != nil {
		slog.Warn("traffic history disabled", "err", err)
		hist = nil
	} else {
		hist.Restore(ctx, time.Now())
		tc.Sink = hist.Sink
		go hist.Run(ctx)
	}

	rulesSvc.OnUpdated = func(ctx context.Context) {
		if _, err := dep.Apply(ctx, nil, "rule bundle updated", nil); err != nil {
			slog.Error("redeploy after rule bundle update failed", "err", err)
		}
	}
	go rulesSvc.Run(ctx)

	// Automatic IP bans (fail2ban-style) pushed to the gateway's shared memory.
	ban := &autoban.Service{Store: st, Traffic: tc}
	if withGateway {
		ban.PushURL = "http://127.0.0.1:9180/bans"
		ban.HitsURL = "http://127.0.0.1:9180/bad-behaviour"
	}
	go ban.Run(ctx)

	// "Under attack" mode, automatic activations pushed to the gateway's shared memory.
	ua := &underattack.Service{Store: st, Traffic: tc}
	if withGateway {
		ua.PushURL = "http://127.0.0.1:9180/under-attack"
	}
	go ua.Run(ctx)

	// Lists fed from a URL (e.g. Tor exit nodes, bad-bot User-Agents).
	listFeeds := &feeds.Service{Store: st, Apply: func(ctx context.Context, msg string, mutate func(tx *store.Store) error) error {
		_, err := dep.Apply(ctx, nil, msg, mutate)
		return err
	}}
	go listFeeds.Run(ctx)

	tlsOn := envBool("CORVUS_ADMIN_TLS", true)
	adminProxies, err := netcfg.NormalizeTrustedCIDRs(strings.Split(env("CORVUS_ADMIN_TRUSTED_PROXIES", ""), ","))
	if err != nil {
		return fmt.Errorf("CORVUS_ADMIN_TRUSTED_PROXIES: %w", err)
	}
	srv := &api.Server{Store: st, Deployer: dep, Paths: paths, Live: lv, Setup: setup,
		Limiter: auth.NewLimiter(10, 10*time.Minute), UI: webui.Handler(), SecureCookie: tlsOn, Version: version, Proxies: api.NewConsoleProxies(adminProxies),
		AcctLimiter: auth.NewLimiter(10, 15*time.Minute),
		Traffic:     tc, ACME: acmeSvc, NPMImportDir: env("CORVUS_IMPORT_DIR", "/import"), DataKey: dataKey, MFA: auth.NewMFAChallenges(), GeoIP: geo, AutoBan: ban, UnderAttack: ua, History: hist, Rules: rulesSvc, Feeds: listFeeds}
	if or != nil {
		srv.Gateway = or.Status
	}
	srv.Alerts = &alerts.Engine{Store: st, Key: dataKey, Src: alerts.Sources{
		Traffic:     tc,
		RedisUp:     lv.Ping,
		UnderAttack: ua.Drain,
		Rooms: func(ctx context.Context) []live.Stats {
			rooms, err := st.ListWaitrooms(ctx)
			if err != nil {
				return nil
			}
			out := make([]live.Stats, 0, len(rooms))
			for _, r := range rooms {
				if r.Enabled {
					out = append(out, lv.Room(ctx, r))
				}
			}
			return out
		},
	}}
	if lv == nil {
		srv.Alerts.Src.RedisUp = nil // no Redis client: never report Redis as down
	}
	go srv.Alerts.Run(ctx)
	go srv.PurgeLoop(ctx)

	hs := &http.Server{
		Addr:              adminListen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       30 * time.Second,
		WriteTimeout:      60 * time.Second,
		IdleTimeout:       120 * time.Second,
		TLSConfig:         &tls.Config{MinVersion: tls.VersionTLS12},
	}
	errc := make(chan error, 1)
	go func() {
		slog.Info("admin UI/API listening", "addr", hs.Addr, "tls", tlsOn, "version", version)
		var err error
		if tlsOn {
			cert := env("CORVUS_ADMIN_CERT", filepath.Join(paths.DefaultCert, "fullchain.pem"))
			key := env("CORVUS_ADMIN_KEY", filepath.Join(paths.DefaultCert, "privkey.pem"))
			err = hs.ListenAndServeTLS(cert, key)
		} else {
			err = hs.ListenAndServe()
		}
		if !errors.Is(err, http.ErrServerClosed) {
			errc <- err
		}
	}()

	var runErr error
	select {
	case <-ctx.Done():
		slog.Info("shutting down")
	case err := <-errc:
		runErr = fmt.Errorf("admin server: %w", err)
	case err := <-gatewayDone:
		runErr = fmt.Errorf("gateway exited unexpectedly: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = hs.Shutdown(shutdownCtx)
	_ = acmeSrv.Shutdown(shutdownCtx)
	if or != nil {
		or.Stop(10 * time.Second)
	}
	if rd != nil {
		rd.Stop(5 * time.Second)
	}
	return runErr
}
