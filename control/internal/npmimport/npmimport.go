// Package npmimport reads an existing Nginx Proxy Manager installation (SQLite file or
// MySQL/MariaDB database, plus certificate files mounted read-only) and converts proxy hosts,
// certificates and access lists into Corvus objects.
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
	Locations      int      `json:"locations"`
	Conflict       string   `json:"conflict,omitempty"`
	Warnings       []string `json:"warnings,omitempty"`
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

type Snapshot struct {
	ProxyHosts   []Host       `json:"proxy_hosts"`
	Certificates []Cert       `json:"certificates"`
	AccessLists  []AccessList `json:"access_lists"`
	Warnings     []string     `json:"warnings"`
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
		var locs []json.RawMessage
		_ = json.Unmarshal([]byte(locations), &locs)
		h.Locations = len(locs)
		h.SSLForced, h.BlockExploits, h.Websocket, h.HTTP2, h.Enabled = ssl == 1, blk == 1, ws == 1, h2 == 1, en == 1
		h.Forward = fmt.Sprintf("%s://%s:%d", h.ForwardScheme, h.ForwardHost, h.ForwardPort)
		if h.Locations > 0 {
			h.Warnings = append(h.Warnings, fmt.Sprintf("%d custom location(s) are not imported; recreate them as advanced configuration", h.Locations))
		}
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

	var streams, redirs int
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM stream WHERE is_deleted = 0`).Scan(&streams)
	_ = db.QueryRowContext(ctx, `SELECT COUNT(*) FROM redirection_host WHERE is_deleted = 0`).Scan(&redirs)
	if streams > 0 {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("%d stream(s) (TCP/UDP) are not supported yet and were skipped", streams))
	}
	if redirs > 0 {
		snap.Warnings = append(snap.Warnings, fmt.Sprintf("%d redirection host(s) are not supported yet and were skipped", redirs))
	}
	return snap, nil
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
	cfg.TLSConfig = "preferred"
	cfg.Params = map[string]string{"charset": "utf8mb4"}
	db, err := sql.Open("mysql", cfg.FormatDSN())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // one session, so the read-only setting below applies to every query
	if _, err := db.ExecContext(ctx, "SET SESSION TRANSACTION READ ONLY"); err != nil {
		db.Close()
		return nil, fmt.Errorf("cannot connect to MySQL at %s: %v", cfg.Addr, err)
	}
	return db, nil
}
