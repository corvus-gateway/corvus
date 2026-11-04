package npmimport

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"testing"

	"github.com/go-sql-driver/mysql"
)

// NPM's MySQL/MariaDB schema subset (knex migrations: JSON columns, unsigned ints).
var npmMySQLSchema = []string{
	"CREATE TABLE `user` (id INT UNSIGNED PRIMARY KEY, email VARCHAR(255), is_deleted INT UNSIGNED NOT NULL DEFAULT 0)",
	"CREATE TABLE certificate (id INT UNSIGNED PRIMARY KEY, owner_user_id INT UNSIGNED, provider VARCHAR(255), nice_name VARCHAR(255), domain_names JSON NOT NULL, meta JSON NOT NULL, is_deleted INT UNSIGNED NOT NULL DEFAULT 0)",
	"CREATE TABLE access_list (id INT UNSIGNED PRIMARY KEY, name VARCHAR(255), satisfy_any TINYINT(1) NOT NULL DEFAULT 0, pass_auth TINYINT(1) NOT NULL DEFAULT 1, is_deleted INT UNSIGNED NOT NULL DEFAULT 0)",
	"CREATE TABLE access_list_client (id INT UNSIGNED PRIMARY KEY, access_list_id INT UNSIGNED, address VARCHAR(255), directive VARCHAR(255))",
	"CREATE TABLE access_list_auth (id INT UNSIGNED PRIMARY KEY, access_list_id INT UNSIGNED, username VARCHAR(255), password VARCHAR(255))",
	"CREATE TABLE proxy_host (id INT UNSIGNED PRIMARY KEY, domain_names JSON NOT NULL, forward_scheme VARCHAR(255), forward_host VARCHAR(255), forward_port INT UNSIGNED, access_list_id INT UNSIGNED NOT NULL DEFAULT 0, certificate_id INT UNSIGNED NOT NULL DEFAULT 0, ssl_forced INT UNSIGNED NOT NULL DEFAULT 0, block_exploits INT UNSIGNED NOT NULL DEFAULT 0, allow_websocket_upgrade INT UNSIGNED NOT NULL DEFAULT 0, http2_support INT UNSIGNED NOT NULL DEFAULT 0, enabled INT UNSIGNED NOT NULL DEFAULT 1, advanced_config TEXT, locations JSON, is_deleted INT UNSIGNED NOT NULL DEFAULT 0)",
	"CREATE TABLE stream (id INT UNSIGNED PRIMARY KEY, is_deleted INT UNSIGNED NOT NULL DEFAULT 0)",
	"CREATE TABLE redirection_host (id INT UNSIGNED PRIMARY KEY, is_deleted INT UNSIGNED NOT NULL DEFAULT 0)",
	"INSERT INTO `user` VALUES (1, 'ops@example.com', 0)",
	`INSERT INTO certificate VALUES (1, 1, 'letsencrypt', 'shop', '["shop.example.com"]', '{"dns_challenge":true,"dns_provider":"cloudflare","dns_provider_credentials":"dns_cloudflare_api_token = cf-token-123"}', 0)`,
	`INSERT INTO certificate VALUES (2, 1, 'other', 'gone', '["old.example.com"]', '{}', 1)`,
	"INSERT INTO access_list VALUES (1, 'office', 1, 1, 0)",
	"INSERT INTO access_list_client VALUES (1, 1, '192.168.0.0/24', 'allow')",
	"INSERT INTO access_list_auth VALUES (1, 1, 'alice', 'plain-secret')",
	`INSERT INTO proxy_host VALUES (1, '["shop.example.com","www.shop.example.com"]', 'http', 'app', 8080, 1, 1, 1, 1, 1, 1, 1, 'client_max_body_size 20m;', NULL, 0)`,
	`INSERT INTO proxy_host VALUES (2, '["deleted.example.com"]', 'http', 'c', 80, 0, 0, 0, 0, 0, 0, 1, NULL, '[]', 1)`,
	"INSERT INTO redirection_host VALUES (1, 0)",
}

// TestReadMySQL runs against real servers listed in CORVUS_TEST_MYSQL
// ("name=host:port,..."; root password "test"), e.g. the MySQL and MariaDB services in CI.
func TestReadMySQL(t *testing.T) {
	targets := os.Getenv("CORVUS_TEST_MYSQL")
	if targets == "" {
		t.Skip("CORVUS_TEST_MYSQL not set")
	}
	for i, target := range strings.Split(targets, ",") {
		name, addr, _ := strings.Cut(target, "=")
		t.Run(name, func(t *testing.T) {
			host, port, err := net.SplitHostPort(addr)
			if err != nil {
				t.Fatal(err)
			}
			p, _ := strconv.Atoi(port)
			dbName := fmt.Sprintf("npm_test_%d", i)
			cfg := mysql.NewConfig()
			cfg.User, cfg.Passwd, cfg.Net, cfg.Addr, cfg.MultiStatements = "root", "test", "tcp", addr, true
			admin, err := sql.Open("mysql", cfg.FormatDSN())
			if err != nil {
				t.Fatal(err)
			}
			defer admin.Close()
			for _, q := range []string{"DROP DATABASE IF EXISTS " + dbName, "CREATE DATABASE " + dbName, "USE " + dbName} {
				if _, err := admin.Exec(q); err != nil {
					t.Fatal(err)
				}
			}
			for _, q := range npmMySQLSchema {
				if _, err := admin.Exec(q); err != nil {
					t.Fatalf("%s: %v", q, err)
				}
			}

			src := Source{DataDir: t.TempDir(), LetsEncryptDir: t.TempDir(), MySQL: &MySQL{Host: host, Port: p, User: "root", Password: "test", Database: dbName}}
			snap, err := Read(context.Background(), src)
			if err != nil {
				t.Fatal(err)
			}
			if len(snap.ProxyHosts) != 1 || len(snap.Certificates) != 1 || len(snap.AccessLists) != 1 {
				t.Fatalf("soft-deleted rows not filtered: %d hosts %d certs %d acls", len(snap.ProxyHosts), len(snap.Certificates), len(snap.AccessLists))
			}
			h := snap.ProxyHosts[0]
			if len(h.Domains) != 2 || h.ForwardPort != 8080 || !h.SSLForced || !h.Websocket || !h.HTTP2 || !h.Enabled || h.CertificateID != 1 || h.AccessListID != 1 || h.AdvancedConfig == "" {
				t.Fatalf("host mapping: %+v", h)
			}
			c := snap.Certificates[0]
			if c.DNSToken != "cf-token-123" || c.Email != "ops@example.com" || c.Domains[0] != "shop.example.com" {
				t.Fatalf("certificate mapping: %+v", c)
			}
			a := snap.AccessLists[0]
			if a.SatisfyAll || len(a.Clients) != 1 || len(a.Users) != 1 || a.Users[0].Password != "plain-secret" {
				t.Fatalf("access list mapping: %+v", a)
			}
			if len(snap.Warnings) != 1 {
				t.Fatalf("expected the redirection warning, got %v", snap.Warnings)
			}

			// the import session is read-only
			db, err := open(context.Background(), src)
			if err != nil {
				t.Fatal(err)
			}
			defer db.Close()
			if _, err := db.Exec("INSERT INTO stream VALUES (9, 0)"); err == nil {
				t.Fatal("write succeeded on the read-only import session")
			}
		})
	}
}

func TestOpenMySQLValidates(t *testing.T) {
	if _, err := open(context.Background(), Source{MySQL: &MySQL{Host: "db"}}); err == nil {
		t.Fatal("missing user/database accepted")
	}
	if _, err := open(context.Background(), Source{MySQL: &MySQL{Host: "db", User: "npm", Database: "npm", Port: 70000}}); err == nil {
		t.Fatal("invalid port accepted")
	}
}
