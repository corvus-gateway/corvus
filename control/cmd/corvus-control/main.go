// Command corvus-control is the Corvus control plane. In the corvus container it is
// PID 1: it owns the configuration database, renders and deploys gateway
// configuration, supervises the OpenResty data plane and serves the admin UI/API.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io/fs"
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
	"corvus/control/internal/ipset"
	"corvus/control/internal/live"
	"corvus/control/internal/netcfg"
	"corvus/control/internal/sigs"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
	"corvus/control/internal/underattack"
	"corvus/control/internal/update"
	"corvus/control/internal/validate"
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
	if len(os.Args) > 1 && os.Args[1] == "reset-admin" {
		os.Exit(resetAdmin(os.Args[2:], os.Stdout, os.Stderr))
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))
	if err := run(); err != nil {
		slog.Error("corvus-control stopped", "err", err)
		os.Exit(1)
	}
}

// secretMinLen: generated secrets are 43 characters (32 random bytes, base64); shorter
// content is damaged (for example a file truncated by a crash).
const secretMinLen = 32

// secret returns a persistent random secret stored at path, created when the file does
// not exist. A file that exists but is damaged (empty, too short) is handled per secret:
//   - replaceable: a new secret is generated in its place. Safe for secrets that only
//     protect short-lived state or are handed to their users at every start: the
//     access-queue HMAC (visitors in a queue get new tickets), the debug key and the
//     Redis password (the ACL file is rewritten at every start).
//   - not replaceable (secrets/data_key): startup fails with an error, since a new key
//     would make every sealed setting unreadable.
func secret(path string, replaceable bool) (string, error) {
	b, err := os.ReadFile(path)
	switch {
	case err == nil:
		if v := strings.TrimSpace(string(b)); len(v) >= secretMinLen {
			return v, nil
		}
		if !replaceable {
			return "", fmt.Errorf("%s is damaged (empty or too short): restore it from a backup; "+
				"deleting it starts with a new key, and settings sealed with the old one "+
				"(DNS credentials, two-factor secrets, alert and SSO secrets) must be entered again", path)
		}
		slog.Warn("secret file damaged, generating a new one", "file", path)
	case errors.Is(err, fs.ErrNotExist):
	default:
		return "", err
	}
	v := auth.RandomToken(32)
	if err := writeFileAtomic(path, []byte(v+"\n"), 0o600); err != nil {
		return "", err
	}
	return v, nil
}

// writeFileAtomic replaces path with data so that a crash leaves either the old or the
// new content: temporary file, fsync, rename, fsync of the directory.
func writeFileAtomic(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	f, err := os.CreateTemp(dir, "."+filepath.Base(path)+".tmp-")
	if err != nil {
		return err
	}
	tmp := f.Name()
	defer os.Remove(tmp) // after a successful rename there is nothing left to remove
	if err := f.Chmod(mode); err != nil && !errors.Is(err, errors.ErrUnsupported) {
		f.Close()
		return err
	}
	if _, err := f.Write(data); err != nil {
		f.Close()
		return err
	}
	if err := f.Sync(); err != nil {
		f.Close()
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return err
	}
	if d, err := os.Open(dir); err == nil {
		_ = d.Sync() // persist the rename (not supported everywhere)
		d.Close()
	}
	return nil
}

// applyStagedRestore applies a restore confirmed in the console, before anything opens
// the data. When it fails the previous state stays in place and startup continues with
// it. An interrupted restore that could not be rolled back is the exception: the data
// directory may lack the database or the data key, and starting would create an empty
// database and a new key, so startup stops with the error.
func applyStagedRestore(dataDir string, now time.Time) error {
	rb, err := backup.ApplyPending(dataDir, slog.Default(), now)
	var ie *backup.InterruptedRestoreError
	switch {
	case errors.As(err, &ie):
		return fmt.Errorf("refusing to start: %w; restore-journal.json in the restore-backup-* directory "+
			"lists the moves to undo by hand (or restore the data directory from a copy), then start again", err)
	case err != nil:
		slog.Error("staged restore was not applied", "err", err)
	case rb != "":
		slog.Warn("configuration restored from backup; the replaced files are kept for a manual rollback", "dir", rb)
	}
	return nil
}

func run() error {
	dataDir := env("CORVUS_DATA_DIR", "/data")
	withGateway := envBool("CORVUS_GATEWAY", true)
	validate.SetDataDir(dataDir)
	if err := applyStagedRestore(dataDir, time.Now()); err != nil {
		return err
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

	wrSecret, err := secret(filepath.Join(dataDir, "secrets", "waitroom_hmac"), true)
	if err != nil {
		return err
	}
	debugKey := ""
	if envBool("CORVUS_DEBUG_ENDPOINTS", false) {
		if debugKey, err = secret(filepath.Join(dataDir, "secrets", "debug_key"), true); err != nil {
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
		if redisPass, err = secret(filepath.Join(dataDir, "secrets", "redis_password"), true); err != nil {
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

	dataKeyB64, err := secret(filepath.Join(dataDir, "secrets", "data_key"), false)
	if err != nil {
		return err
	}
	dataKey, err := base64.RawURLEncoding.DecodeString(dataKeyB64)
	if err != nil || len(dataKey) != 32 {
		return fmt.Errorf("secrets/data_key is not a valid key (32 bytes, base64): restore it from a backup")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	tc := traffic.New()
	tc.MaskIPs = func() bool { n, _ := st.Network(context.Background()); return n.MaskIPs }

	// Traffic history: statistics/events survive restarts; request logs are archived
	// per day (compressed) with retention by age and total size. Restored before the
	// gateway starts, so live events come after the restored ones.
	stopHistory := func() {}
	hist := &history.Service{Dir: filepath.Join(dataDir, "history"), Store: st, Collector: tc}
	if err := hist.Open(ctx); err != nil {
		slog.Warn("traffic history disabled", "err", err)
		hist = nil
	} else {
		hist.Restore(ctx, time.Now())
		tc.Sink = hist.Sink
		// own context, so shutdown can wait for the final save (see stopHistory below)
		histCtx, histStop := context.WithCancel(context.Background())
		histDone := make(chan struct{})
		go func() { hist.Run(histCtx); close(histDone) }()
		stopHistory = func() {
			histStop()
			select {
			case <-histDone:
			case <-time.After(15 * time.Second):
				slog.Warn("traffic history: final save timed out")
			}
		}
	}
	// Access-log collector must listen before the gateway starts writing to it, and only
	// once it is set up (Sink): Listen starts reading at once.
	if err := tc.Listen(filepath.Join(dataDir, "run", "corvus-access.sock"), ctx.Done()); err != nil {
		slog.Warn("traffic collector disabled", "err", err)
	}

	// Managed rule bundle: body signatures + virtual-patch templates (signed updates).
	rulesSvc := &sigs.Service{Dir: filepath.Join(dataDir, "rules"), Store: st}
	if err := rulesSvc.Init(); err != nil {
		return err
	}

	comp := &compiler.Compiler{P: paths, Lists: &ipset.Dir{Path: paths.ListDir()}, Bundle: rulesSvc.Bundle, Cloudflare: func(ctx context.Context, s *store.Store) []string {
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
	// Per-start key with which the gateway marks requests it proxies to the console;
	// the console trusts X-Forwarded-For from local peers only with it (api.ConsoleProxies).
	consoleKey := ""
	// Per-start key the gateway requires on its internal status endpoints (/bans,
	// /bad-behaviour, /under-attack), so they are not reachable through a proxied target.
	statusKey := ""
	if withGateway {
		consoleKey = auth.RandomToken(32)
		statusKey = auth.RandomToken(32)
	}
	dep := &deploy.Deployer{Store: st, Compiler: comp, Dir: filepath.Join(dataDir, "nginx"), Keep: 20}
	if withGateway {
		or = &gateway.OpenResty{
			Bin:         env("CORVUS_OPENRESTY_BIN", "/usr/local/openresty/bin/openresty"),
			CurrentConf: dep.CurrentConf(),
			StatusURL:   "http://127.0.0.1:9180/version",
			Env: []string{
				"CORVUS_WR_SECRET=" + wrSecret, "CORVUS_DEBUG_KEY=" + debugKey, "CORVUS_CONSOLE_KEY=" + consoleKey,
				"CORVUS_STATUS_KEY=" + statusKey,
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

	// Degraded mode: whatever goes wrong with the gateway, the control plane (PID 1) and
	// its console keep running so the operator can fix it.
	if err := dep.EnsureCurrent(ctx); err != nil {
		slog.Error("DEGRADED: the configuration could not be deployed at startup; falling back to an earlier one", "err", err)
		if _, ferr := dep.Fallback(ctx, "", "startup deployment failed"); ferr != nil {
			slog.Error("DEGRADED: no usable gateway configuration; the gateway will not start until the settings are fixed in the console", "err", ferr)
		}
	}
	var rd *gateway.Redis
	if embeddedRedis {
		rd = &gateway.Redis{Bin: env("CORVUS_REDIS_BIN", "/usr/bin/valkey-server"), Port: redisPort, ACLFile: aclPath,
			Dir: filepath.Join(dataDir, "run", "redis"), MaxMemory: env("CORVUS_REDIS_MAXMEMORY", "256mb")}
		startRedis(ctx, rd)
	}
	if or != nil {
		go or.Supervise(ctx, gatewayBootFailure(ctx, dep))
	}

	lv, err := live.New(net.JoinHostPort(redisHost, redisPort), redisUser, redisPass, redisScript)
	if err != nil {
		slog.Warn("live access queue stats disabled", "err", err)
	}

	setup := auth.NewSetupToken()
	preset := env("CORVUS_SETUP_TOKEN", "") // trimmed: a stray newline (CRLF env file) could never be typed
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
			if err := dep.Refresh(ctx, "certificate issued/renewed"); err != nil {
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
			if err := dep.Refresh(ctx, "country database updated"); err != nil {
				slog.Error("redeploy after country database update failed", "err", err)
			}
		}}
	go geo.Run(ctx)

	rulesSvc.OnUpdated = func(ctx context.Context) {
		if err := dep.Refresh(ctx, "rule bundle updated"); err != nil {
			slog.Error("redeploy after rule bundle update failed", "err", err)
		}
	}
	go rulesSvc.Run(ctx)

	// Automatic IP bans (fail2ban-style) pushed to the gateway's shared memory.
	ban := &autoban.Service{Store: st, Traffic: tc}
	if withGateway {
		ban.PushURL = "http://127.0.0.1:9180/bans"
		ban.HitsURL = "http://127.0.0.1:9180/bad-behaviour"
		ban.StatusKey = statusKey
	}
	// "Under attack" mode, automatic activations pushed to the gateway's shared memory.
	ua := &underattack.Service{Store: st, Traffic: tc}
	if withGateway {
		ua.PushURL = "http://127.0.0.1:9180/under-attack"
		ua.StatusKey = statusKey
		ban.OnGatewayRestart = ua.Trigger // empty shared memory: push the activations at once
	}
	go ban.Run(ctx)
	go ua.Run(ctx)

	// Lists fed from a URL (e.g. Tor exit nodes, bad-bot User-Agents).
	listFeeds := &feeds.Service{Store: st, Lists: comp.Lists, Apply: func(ctx context.Context, msg string, mutate func(tx *store.Store) error) error {
		if dep.RollbackActive(ctx) {
			// keep the operator's rollback: store the new list, deploy it later
			slog.Warn("rollback active: list stored, not deployed", "reason", msg)
			return st.WithTx(ctx, mutate)
		}
		_, err := dep.Apply(ctx, nil, msg, mutate)
		return err
	}}
	// Test only (deploy/dev integration checks): feeds from a private fixture server.
	if hosts := env("CORVUS_TEST_FEED_HOSTS", ""); hosts != "" {
		var ca []byte
		if p := env("CORVUS_TEST_FEED_CA", ""); p != "" {
			if ca, err = os.ReadFile(p); err != nil {
				return fmt.Errorf("CORVUS_TEST_FEED_CA: %w", err)
			}
		}
		if listFeeds.Client, err = feeds.TestClient(strings.Split(hosts, ","), ca); err != nil {
			return fmt.Errorf("CORVUS_TEST_FEED_CA: %w", err)
		}
		slog.Warn("TEST ONLY: list feeds may use these private hosts", "hosts", hosts)
	}
	go listFeeds.Run(ctx)

	// New release notice in the console (GitHub "latest release", twice a day).
	var updates *update.Checker
	if envBool("CORVUS_UPDATE_CHECK", true) {
		updates = &update.Checker{Current: version}
		go updates.Run(ctx)
	}

	tlsOn := envBool("CORVUS_ADMIN_TLS", true)
	adminProxies, err := netcfg.NormalizeTrustedCIDRs(strings.Split(env("CORVUS_ADMIN_TRUSTED_PROXIES", ""), ","))
	if err != nil {
		return fmt.Errorf("CORVUS_ADMIN_TRUSTED_PROXIES: %w", err)
	}
	srv := &api.Server{Store: st, Deployer: dep, Paths: paths, Live: lv, Setup: setup,
		Limiter: auth.NewLimiter(10, 10*time.Minute), UI: webui.Handler(), SecureCookie: tlsOn, Version: version, Updates: updates, Proxies: api.NewConsoleProxies(adminProxies, consoleKey),
		AcctLimiter: auth.NewLimiter(10, 15*time.Minute),
		Traffic:     tc, ACME: acmeSvc, NPMImportDir: env("CORVUS_IMPORT_DIR", "/import"), DataKey: dataKey, MFA: auth.NewMFAChallenges(), GeoIP: geo, AutoBan: ban, UnderAttack: ua, History: hist, Rules: rulesSvc, Feeds: listFeeds}
	if or != nil {
		srv.Gateway = or.Status
		srv.GatewayProblem = or.Problem
		srv.UpstreamFailures = or.UpstreamFailures
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
		// A save ends in a deploy: openresty -t (up to 30 s) and the reload check (up to
		// 15 s) on slow devices, plus rendering. Long downloads and refreshes extend
		// their own deadline.
		WriteTimeout: 150 * time.Second,
		IdleTimeout:  120 * time.Second,
		TLSConfig:    &tls.Config{MinVersion: tls.VersionTLS12},
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
	}
	// Save traffic history first: the container runtime kills the process 10 s after the
	// stop signal (Docker's default), and both a slow console request (admin server
	// shutdown) and open WebSockets (graceful nginx stop) can take that long.
	stopHistory()
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
