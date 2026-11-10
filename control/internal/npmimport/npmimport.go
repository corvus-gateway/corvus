// Package npmimport reads an existing Nginx Proxy Manager installation (SQLite file or
// MySQL/MariaDB database, plus certificate files mounted read-only) and converts proxy hosts,
// redirection hosts, certificates and access lists into Corvus objects.
package npmimport

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"corvus/control/internal/acme"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"

	"github.com/go-sql-driver/mysql"
	_ "modernc.org/sqlite"
)

// Source locates an NPM installation: DataDir is NPM's /data, LetsEncryptDir its /etc/letsencrypt.
// NPM keeps its settings in DataDir/database.sqlite by default; installations configured with
// DB_MYSQL_* use a MySQL/MariaDB server instead, given by MySQL.
type Source struct {
	DataDir        string
	LetsEncryptDir string
	MySQL          *MySQL
}

// MySQL is the connection to an NPM MySQL/MariaDB database (NPM's DB_MYSQL_* settings).
type MySQL struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	User     string `json:"user"`
	Password string `json:"password"`
	Database string `json:"database"`
}

type Host struct {
	NPMID          int64    `json:"npm_id"`
	Domains        []string `json:"domains"`
	ForwardScheme  string   `json:"forward_scheme"`
	ForwardHost    string   `json:"forward_host"`
	ForwardPort    int      `json:"forward_port"`
	Forward        string   `json:"forward"`
	CertificateID  int64    `json:"certificate_id,omitempty"` // NPM id
	AccessListID   int64    `json:"access_list_id,omitempty"` // NPM id
	SSLForced      bool     `json:"ssl_forced"`
	HTTP2          bool     `json:"http2"`
	Websocket      bool     `json:"websocket"`
	BlockExploits  bool     `json:"block_exploits"`
	Enabled        bool     `json:"enabled"`
	AdvancedConfig string   `json:"-"`
	Locations      int      `json:"locations"` // custom locations in NPM
	// CustomLocations are the NPM custom locations that are imported.
	CustomLocations []store.Location `json:"custom_locations"`
	Conflict        string           `json:"conflict,omitempty"`
	Warnings        []string         `json:"warnings,omitempty"`
}

type Cert struct {
	NPMID          int64             `json:"npm_id"`
	Name           string            `json:"name"`
	Provider       string            `json:"provider"` // letsencrypt | other
	Domains        []string          `json:"domains"`
	DNSProvider    string            `json:"dns_provider,omitempty"`
	Email          string            `json:"email,omitempty"`
	FilesFound     bool              `json:"files_found"`
	Warnings       []string          `json:"warnings,omitempty"`
	CertPEM        []byte            `json:"-"`
	KeyPEM         []byte            `json:"-"`
	DNSCredentials *acme.Credentials `json:"-"` // parsed DNS-01 credentials (sensitive); nil = no automatic renewal
}

type ACLClient struct {
	Address   string
	Directive string
}

type ACLUser struct {
	Username string
	Password string // plaintext in NPM's database (sensitive)
}

type AccessList struct {
	NPMID      int64       `json:"npm_id"`
	Name       string      `json:"name"`
	SatisfyAll bool        `json:"satisfy_all"`
	Clients    []ACLClient `json:"-"`
	Users      []ACLUser   `json:"-"`
	UserCount  int         `json:"users"`
	RuleCount  int         `json:"rules"`
}

// Redirect is an NPM redirection host.
type Redirect struct {
	NPMID          int64    `json:"npm_id"`
	Domains        []string `json:"domains"`
	ForwardScheme  string   `json:"forward_scheme"` // auto | http | https
	ForwardDomain  string   `json:"forward_domain"`
	HTTPCode       int      `json:"http_code"`
	PreservePath   bool     `json:"preserve_path"`
	Forward        string   `json:"forward"`                  // display: "301 https://example.com$request_uri"
	CertificateID  int64    `json:"certificate_id,omitempty"` // NPM id
	SSLForced      bool     `json:"ssl_forced"`
	HTTP2          bool     `json:"http2"`
	BlockExploits  bool     `json:"block_exploits"`
	Enabled        bool     `json:"enabled"`
	AdvancedConfig string   `json:"-"`
	Conflict       string   `json:"conflict,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
}

// Stream is an NPM stream (TCP/UDP port forwarding).
type Stream struct {
	NPMID         int64    `json:"npm_id"`
	IncomingPort  int      `json:"incoming_port"`
	ForwardHost   string   `json:"forward_host"`
	ForwardPort   int      `json:"forward_port"`
	TCP           bool     `json:"tcp_forwarding"`
	UDP           bool     `json:"udp_forwarding"`
	CertificateID int64    `json:"certificate_id,omitempty"` // NPM id (TLS termination)
	Enabled       bool     `json:"enabled"`
	Conflict      string   `json:"conflict,omitempty"`
	Warnings      []string `json:"warnings,omitempty"`
}

type Snapshot struct {
	ProxyHosts   []Host       `json:"proxy_hosts"`
	Redirects    []Redirect   `json:"redirection_hosts"`
	Streams      []Stream     `json:"streams"`
	Certificates []Cert       `json:"certificates"`
	AccessLists  []AccessList `json:"access_lists"`
	// UnknownHosts is NPM's default site setting when it differs from the Corvus default.
	UnknownHosts *store.UnknownHosts `json:"unknown_hosts,omitempty"`
	Warnings     []string            `json:"warnings"`
}

// npmRefs detects advanced config that depends on NPM-specific variables or includes.
var npmRefs = regexp.MustCompile(`\$(server|port|forward_scheme|hsts_header|x_forwarded_proto)\b|include\s+/(etc/nginx|data)/`)

// Read loads the NPM installation. Only rows with is_deleted = 0 are considered.
func Read(ctx context.Context, src Source) (*Snapshot, error) {
	db, err := open(ctx, src)
	if err != nil {
		return nil, err
	}
	defer db.Close()
	snap := &Snapshot{Warnings: []string{}}

	// Queries below are plain SQL understood by SQLite, MySQL and MariaDB.
	var n int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM proxy_host`).Scan(&n); err != nil {
		return nil, fmt.Errorf("this does not look like an NPM database (no proxy_host table): %w", err)
	}

	// owner emails (Let's Encrypt registration email in NPM is the owner's email)
	emails := map[int64]string{}
	if rows, err := db.QueryContext(ctx, `SELECT id, email FROM `+"`user`"+` WHERE is_deleted = 0`); err == nil {
		for rows.Next() {
			var id int64
			var e string
			if rows.Scan(&id, &e) == nil {
				emails[id] = e
			}
		}
		rows.Close()
	}

	// ---- certificates
	rows, err := db.QueryContext(ctx, `SELECT id, owner_user_id, provider, nice_name, domain_names, COALESCE(meta, '{}') FROM certificate WHERE is_deleted = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("read certificates: %w", err)
	}
	for rows.Next() {
		var c Cert
		var owner int64
		var domains, meta string
		if err := rows.Scan(&c.NPMID, &owner, &c.Provider, &c.Name, &domains, &meta); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(domains), &c.Domains)
		var m struct {
			DNSChallenge bool   `json:"dns_challenge"`
			DNSProvider  string `json:"dns_provider"`
			Credentials  string `json:"dns_provider_credentials"`
		}
		_ = json.Unmarshal([]byte(meta), &m)
		c.Email = emails[owner]
		if c.Provider == "letsencrypt" {
			if m.DNSChallenge {
				c.DNSProvider = m.DNSProvider
				if p, ok := acme.ProviderByID(m.DNSProvider); !ok {
					c.Warnings = append(c.Warnings, fmt.Sprintf("DNS provider %q is not supported by Corvus yet; imported without automatic renewal", m.DNSProvider))
				} else if cred, err := acme.ParseCredentials(p.ID, m.Credentials); err != nil {
					c.Warnings = append(c.Warnings, fmt.Sprintf("%s credentials can't be used (%v); imported without automatic renewal", p.Name, err))
				} else {
					c.DNSCredentials = &cred
				}
			}
			dir := filepath.Join(src.LetsEncryptDir, "live", fmt.Sprintf("npm-%d", c.NPMID))
			c.CertPEM, _ = os.ReadFile(filepath.Join(dir, "fullchain.pem"))
			c.KeyPEM, _ = os.ReadFile(filepath.Join(dir, "privkey.pem"))
		} else {
			dir := filepath.Join(src.DataDir, "custom_ssl", fmt.Sprintf("npm-%d", c.NPMID))
			c.CertPEM, _ = os.ReadFile(filepath.Join(dir, "fullchain.pem"))
			c.KeyPEM, _ = os.ReadFile(filepath.Join(dir, "privkey.pem"))
		}
		c.FilesFound = len(c.CertPEM) > 0 && len(c.KeyPEM) > 0
		if !c.FilesFound {
			c.Warnings = append(c.Warnings, "certificate files not found (mount NPM's /etc/letsencrypt and /data read-only)")
		}
		snap.Certificates = append(snap.Certificates, c)
	}
	rows.Close()

	// ---- access lists
	rows, err = db.QueryContext(ctx, `SELECT id, name, satisfy_any FROM access_list WHERE is_deleted = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("read access lists: %w", err)
	}
	for rows.Next() {
		var a AccessList
		var any int
		if err := rows.Scan(&a.NPMID, &a.Name, &any); err != nil {
			rows.Close()
			return nil, err
		}
		a.SatisfyAll = any == 0
		snap.AccessLists = append(snap.AccessLists, a)
	}
	rows.Close()
	for i := range snap.AccessLists {
		a := &snap.AccessLists[i]
		if r, err := db.QueryContext(ctx, `SELECT address, directive FROM access_list_client WHERE access_list_id = ? ORDER BY id`, a.NPMID); err == nil {
			for r.Next() {
				var c ACLClient
				if r.Scan(&c.Address, &c.Directive) == nil {
					a.Clients = append(a.Clients, c)
				}
			}
			r.Close()
		}
		if r, err := db.QueryContext(ctx, `SELECT username, password FROM access_list_auth WHERE access_list_id = ? ORDER BY id`, a.NPMID); err == nil {
			for r.Next() {
				var u ACLUser
				if r.Scan(&u.Username, &u.Password) == nil {
					a.Users = append(a.Users, u)
				}
			}
			r.Close()
		}
		a.UserCount, a.RuleCount = len(a.Users), len(a.Clients)
	}

	// ---- proxy hosts
	rows, err = db.QueryContext(ctx, `SELECT id, domain_names, forward_scheme, forward_host, forward_port, access_list_id, certificate_id,
		ssl_forced, block_exploits, allow_websocket_upgrade, http2_support, enabled, COALESCE(advanced_config, ''), COALESCE(locations, '[]')
		FROM proxy_host WHERE is_deleted = 0 ORDER BY id`)
	if err != nil {
		return nil, fmt.Errorf("read proxy hosts: %w", err)
	}
	for rows.Next() {
		var h Host
		var domains, locations string
		var ssl, blk, ws, h2, en int
		if err := rows.Scan(&h.NPMID, &domains, &h.ForwardScheme, &h.ForwardHost, &h.ForwardPort, &h.AccessListID, &h.CertificateID,
			&ssl, &blk, &ws, &h2, &en, &h.AdvancedConfig, &locations); err != nil {
			rows.Close()
			return nil, err
		}
		_ = json.Unmarshal([]byte(domains), &h.Domains)
		npmLocations(&h, locations)
		h.SSLForced, h.BlockExploits, h.Websocket, h.HTTP2, h.Enabled = ssl == 1, blk == 1, ws == 1, h2 == 1, en == 1
		h.Forward = fmt.Sprintf("%s://%s:%d", h.ForwardScheme, h.ForwardHost, h.ForwardPort)
		if strings.TrimSpace(h.AdvancedConfig) != "" && npmRefs.MatchString(h.AdvancedConfig) {
			h.Warnings = append(h.Warnings, "advanced configuration references NPM-specific variables/includes and was not imported")
			h.AdvancedConfig = ""
		}
		if strings.Contains(h.ForwardHost, "/") {
			h.Warnings = append(h.Warnings, "forward host contains a path; only the host part is imported")
			h.ForwardHost = strings.SplitN(h.ForwardHost, "/", 2)[0]
		}
		snap.ProxyHosts = append(snap.ProxyHosts, h)
	}
	rows.Close()

	if err := readRedirects(ctx, db, snap); err != nil {
		return nil, err
	}

	if err := readStreams(ctx, db, snap); err != nil {
		return nil, err
	}
	readDefaultSite(ctx, db, snap)
	return snap, nil
}

// readDefaultSite maps NPM's "Default Site" setting (what unknown hosts get) to the
// Corvus unknown host setting. Only a setting other than the Corvus default (not
// found) is offered for import; a custom HTML page cannot be carried over.
func readDefaultSite(ctx context.Context, db *sql.DB, snap *Snapshot) {
	var value, meta string
	if db.QueryRowContext(ctx, "SELECT COALESCE(value, ''), COALESCE(meta, '{}') FROM `setting` WHERE id = 'default-site'").Scan(&value, &meta) != nil {
		return // no setting table (older NPM) or no row: NPM's default
	}
	u, warn := DefaultSite(value, meta)
	if warn != "" {
		snap.Warnings = append(snap.Warnings, warn)
	}
	if u != nil && u.Mode != store.UnknownNotFound {
		snap.UnknownHosts = u
	}
}

// DefaultSite converts NPM's default-site value (congratulations | 404 | 444 | redirect
// | html) and meta ({"redirect": "…", "html": "…"}) into a validated Corvus unknown host
// setting. A non-empty warning explains what could not be imported.
func DefaultSite(value, meta string) (*store.UnknownHosts, string) {
	var m struct {
		Redirect string `json:"redirect"`
	}
	_ = json.Unmarshal([]byte(meta), &m)
	u := &store.UnknownHosts{Mode: store.UnknownNotFound}
	warn := ""
	switch strings.TrimSpace(value) {
	case "", "congratulations", "404":
	case "444":
		u.Mode = store.UnknownClose
	case "redirect":
		u.Mode, u.RedirectURL = store.UnknownRedirect, m.Redirect
	case "html":
		warn = "the default site shows a custom HTML page, which is not imported; unknown hosts get the Corvus not found page"
	default:
		warn = fmt.Sprintf("default site %q is not supported; unknown hosts get the Corvus not found page", value)
	}
	if err := validate.UnknownHosts(u); err != nil {
		return nil, fmt.Sprintf("the default site redirect %q is not a valid http(s) URL and was not imported", m.Redirect)
	}
	return u, warn
}

// npmLocation is an entry of NPM's proxy_host.locations JSON. NPM keeps the forward
// path in forward_host ("backend/api" = host "backend", path "/api").
type npmLocation struct {
	Path           string          `json:"path"`
	ForwardScheme  string          `json:"forward_scheme"`
	ForwardHost    string          `json:"forward_host"`
	ForwardPort    json.RawMessage `json:"forward_port"` // number (or a string in some versions)
	AdvancedConfig string          `json:"advanced_config"`
}

// npmLocations converts NPM custom locations into Corvus locations; entries Corvus
// cannot express (regular-expression paths, NPM-specific snippets) are skipped or
// trimmed with a warning.
func npmLocations(h *Host, raw string) {
	var locs []npmLocation
	_ = json.Unmarshal([]byte(raw), &locs)
	h.Locations = len(locs)
	h.CustomLocations = []store.Location{}
	for _, nl := range locs {
		port, _ := strconv.Atoi(strings.Trim(string(nl.ForwardPort), `" `))
		host, path, _ := strings.Cut(strings.TrimSpace(nl.ForwardHost), "/")
		l := store.Location{Path: nl.Path, ForwardScheme: nl.ForwardScheme, ForwardHost: host, ForwardPort: port, AdvancedConfig: nl.AdvancedConfig}
		if path != "" {
			l.ForwardPath = "/" + path
		}
		if strings.TrimSpace(l.AdvancedConfig) != "" && npmRefs.MatchString(l.AdvancedConfig) {
			h.Warnings = append(h.Warnings, fmt.Sprintf("location %s: advanced configuration references NPM-specific variables/includes and was not imported", nl.Path))
			l.AdvancedConfig = ""
		}
		if err := validate.Location(&l); err != nil {
			h.Warnings = append(h.Warnings, fmt.Sprintf("location %s was not imported: %v", nl.Path, err))
			continue
		}
		h.CustomLocations = append(h.CustomLocations, l)
	}
}

// readStreams loads NPM's stream table. certificate_id (TLS streams) was added in NPM 2.11;
// before 2.9 the forward host column was called forward_ip. No stream table: no streams.
func readStreams(ctx context.Context, db *sql.DB, snap *Snapshot) error {
	const cols = `id, incoming_port, forwarding_port, tcp_forwarding, udp_forwarding, enabled`
	var rows *sql.Rows
	var err error
	withCert := true
	for _, q := range []string{
		`SELECT ` + cols + `, forwarding_host, certificate_id FROM stream WHERE is_deleted = 0 ORDER BY id`,
		`SELECT ` + cols + `, forwarding_host FROM stream WHERE is_deleted = 0 ORDER BY id`,
		`SELECT ` + cols + `, forward_ip FROM stream WHERE is_deleted = 0 ORDER BY id`,
	} {
		if rows, err = db.QueryContext(ctx, q); err == nil {
			break
		}
		withCert = false
	}
	if err != nil {
		var n int
		if db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stream WHERE is_deleted = 0`).Scan(&n) == nil && n > 0 {
			snap.Warnings = append(snap.Warnings, fmt.Sprintf("streams could not be read and were skipped: %v", err))
		}
		return nil
	}
	defer rows.Close()
	for rows.Next() {
		var st Stream
		var tcp, udp, en int
		var cert sql.NullInt64
		dest := []any{&st.NPMID, &st.IncomingPort, &st.ForwardPort, &tcp, &udp, &en, &st.ForwardHost}
		if withCert {
			dest = append(dest, &cert)
		}
		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("read streams: %w", err)
		}
		st.TCP, st.UDP, st.Enabled = tcp == 1, udp == 1, en == 1
		st.ForwardHost = strings.TrimSpace(st.ForwardHost)
		if cert.Valid && cert.Int64 > 0 {
			st.CertificateID = cert.Int64
			if !st.TCP {
				st.Warnings = append(st.Warnings, "TLS termination requires TCP; the certificate is not used")
				st.CertificateID = 0
			}
		}
		snap.Streams = append(snap.Streams, st)
	}
	return rows.Err()
}

// readRedirects loads NPM's redirection_host table. forward_scheme and forward_http_code
// were added in NPM 2.8 (2021); older databases redirect with $scheme and 302.
func readRedirects(ctx context.Context, db *sql.DB, snap *Snapshot) error {
	const cols = `id, domain_names, forward_domain_name, preserve_path, certificate_id, ssl_forced, http2_support, block_exploits, enabled, COALESCE(advanced_config, '')`
	rows, err := db.QueryContext(ctx, `SELECT `+cols+`, forward_scheme, forward_http_code FROM redirection_host WHERE is_deleted = 0 ORDER BY id`)
	legacy := false
	if err != nil {
		if rows, err = db.QueryContext(ctx, `SELECT `+cols+` FROM redirection_host WHERE is_deleted = 0 ORDER BY id`); err != nil {
			snap.Warnings = append(snap.Warnings, fmt.Sprintf("redirection hosts could not be read and were skipped: %v", err))
			return nil
		}
		legacy = true
	}
	defer rows.Close()
	for rows.Next() {
		var h Redirect
		var domains string
		var pp, ssl, h2, blk, en int
		dest := []any{&h.NPMID, &domains, &h.ForwardDomain, &pp, &h.CertificateID, &ssl, &h2, &blk, &en, &h.AdvancedConfig}
		if !legacy {
			dest = append(dest, &h.ForwardScheme, &h.HTTPCode)
		}
		if err := rows.Scan(dest...); err != nil {
			return fmt.Errorf("read redirection hosts: %w", err)
		}
		_ = json.Unmarshal([]byte(domains), &h.Domains)
		h.PreservePath, h.SSLForced, h.HTTP2, h.BlockExploits, h.Enabled = pp == 1, ssl == 1, h2 == 1, blk == 1, en == 1
		if h.ForwardScheme == "" || h.ForwardScheme == "$scheme" {
			h.ForwardScheme = "auto"
		}
		if h.HTTPCode == 0 {
			h.HTTPCode = 302
		}
		switch h.HTTPCode {
		case 301, 302, 303, 307, 308:
		default:
			h.Warnings = append(h.Warnings, fmt.Sprintf("status code %d is not supported; imported as 302", h.HTTPCode))
			h.HTTPCode = 302
		}
		// Targets entered with a scheme ("https://example.com") keep it.
		fd := strings.TrimSpace(h.ForwardDomain)
		for _, sch := range []string{"http", "https"} {
			if len(fd) > len(sch)+3 && strings.EqualFold(fd[:len(sch)+3], sch+"://") {
				fd = fd[len(sch)+3:]
				if h.ForwardScheme == "auto" {
					h.ForwardScheme = sch
				}
			}
		}
		h.ForwardDomain = fd
		scheme := h.ForwardScheme
		if scheme == "auto" {
			scheme = "$scheme"
		}
		h.Forward = fmt.Sprintf("%d %s://%s", h.HTTPCode, scheme, h.ForwardDomain)
		if h.PreservePath {
			h.Forward += "$request_uri"
		}
		if strings.TrimSpace(h.AdvancedConfig) != "" && npmRefs.MatchString(h.AdvancedConfig) {
			h.Warnings = append(h.Warnings, "advanced configuration references NPM-specific variables/includes and was not imported")
			h.AdvancedConfig = ""
		}
		snap.Redirects = append(snap.Redirects, h)
	}
	return rows.Err()
}

// open connects read-only to NPM's SQLite file or MySQL/MariaDB server.
func open(ctx context.Context, src Source) (*sql.DB, error) {
	if m := src.MySQL; m != nil {
		return openMySQL(ctx, m)
	}
	dbPath := filepath.Join(src.DataDir, "database.sqlite")
	if f, err := os.Open(dbPath); err != nil {
		switch {
		case errors.Is(err, fs.ErrPermission):
			return nil, fmt.Errorf("cannot read %s: permission denied (corvus runs as uid %d; make the NPM data directory and database.sqlite readable, e.g. chmod o+rx on the directory and o+r on the file)", dbPath, os.Getuid())
		case errors.Is(err, fs.ErrNotExist):
			if _, derr := os.Stat(src.DataDir); derr != nil {
				return nil, fmt.Errorf("%s is not accessible: %v (check the volume mount)", src.DataDir, derr)
			}
			return nil, fmt.Errorf("NPM database not found at %s; if NPM uses MySQL/MariaDB (DB_MYSQL_* settings), choose MySQL/MariaDB and enter its connection", dbPath)
		default:
			return nil, fmt.Errorf("cannot read %s: %v", dbPath, err)
		}
	} else {
		f.Close()
	}
	return sql.Open("sqlite", "file:"+filepath.ToSlash(dbPath)+"?mode=ro&immutable=1")
}

func openMySQL(ctx context.Context, m *MySQL) (*sql.DB, error) {
	if m.Host == "" || m.User == "" || m.Database == "" {
		return nil, errors.New("MySQL host, user and database are required")
	}
	port := m.Port
	if port == 0 {
		port = 3306
	}
	if port < 1 || port > 65535 {
		return nil, errors.New("MySQL port must be between 1 and 65535")
	}
	cfg := mysql.NewConfig()
	cfg.User, cfg.Passwd, cfg.DBName = m.User, m.Password, m.Database
	cfg.Net, cfg.Addr = "tcp", net.JoinHostPort(m.Host, strconv.Itoa(port))
	cfg.Timeout, cfg.ReadTimeout = 5*time.Second, 30*time.Second
	cfg.TLSConfig = mysqlTLSMode(m.Host)
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one session, so the read-only setting below applies to every query
	if _, err := db.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY"); err != nil {
		db.Close()
		if cfg.TLSConfig == "true" {
			return nil, fmt.Errorf("cannot connect to MySQL at %s: %v (servers outside the local network must offer TLS with a certificate trusted by the system and valid for this host name; for a server on the local network, enter its private IP address or container name)", cfg.Addr, err)
		}
		return nil, fmt.Errorf("cannot connect to MySQL at %s: %v", cfg.Addr, err)
	}
	return db, nil
}

// mysqlTLSMode chooses the driver's TLS setting for host. Servers on the local machine
// or network (loopback/private addresses, container names and local-only domains) use
// TLS when they offer it and fall back to plain connections, as NPM's bundled MariaDB
// has no TLS. Every other server must offer TLS with a verified certificate, so the
// database password never crosses another network in clear or to an impostor.
func mysqlTLSMode(host string) string {
	h := strings.ToLower(strings.TrimSuffix(strings.Trim(host, "[]"), "."))
	if ip := net.ParseIP(h); ip != nil {
		if validate.PublicIP(ip) {
			return "true"
		}
		return "preferred"
	}
	if !strings.Contains(h, ".") {
		return "preferred" // container or LAN host name (e.g. "db", "localhost")
	}
	for _, suffix := range []string{".localhost", ".local", ".internal", ".lan", ".home.arpa"} {
		if strings.HasSuffix(h, suffix) {
			return "preferred"
		}
	}
	return "true"
}
