package validate

import "testing"

func TestAdvancedConfigWebRoot(t *testing.T) {
	refused := []string{
		"root /data;", "root //data;", "alias /./data/;", "root /var/../data;", "root /srv/../../data;",
		"root ../../../../data;", "root data;", `root "/data/secrets";`, "set $d /data; root $d;",
		"root /srv/$host;", `alias \/data;`, "root /;", "root /etc/nginx;", "location /x { root //data; }",
	}
	for _, s := range refused {
		if err := AdvancedConfig(s); err == nil {
			t.Errorf("accepted %q", s)
		}
	}
	allowed := []string{"root /srv/www;", "alias /srv/static/;", `root "/srv/my site";`, "root /srv/www/../static;"}
	for _, s := range allowed {
		if err := AdvancedConfig(s); err != nil {
			t.Errorf("refused %q: %v", s, err)
		}
	}
}

// The gateway can write to the whole data volume: a snippet may not make it store a
// response or a log under a name of its choosing.
func TestAdvancedConfigFileWrites(t *testing.T) {
	for _, bad := range []string{
		`location = /_s { proxy_pass http://203.0.113.9/p; proxy_store /data/nginx/current/hosts/zz.conf; }`,
		`proxy_store on;`,
		`proxy_store_access user:rw;`,
		`fastcgi_store /data/x;`,
		`proxy_temp_path /data/nginx/current/hosts;`,
		`client_body_temp_path /data/certs;`,
		`client_body_in_file_only on;`,
		`dav_methods PUT;`,
		`create_full_put_path on;`,
		`access_log /data/nginx/current/hosts/zz.conf;`,
		`access_log /data/corvus.db;`,
		`access_log "/data/logs/$host.log";`,
		`access_log logs/x.log;`,
		`error_log /data/nginx/current/hosts/zz.conf;`,
		`error_log /data/x.log/../corvus.db;`,
	} {
		if err := AdvancedConfig(bad); err == nil {
			t.Errorf("accepted: %s", bad)
		}
	}
	for _, good := range []string{
		`access_log off;`,
		`access_log syslog:server=10.0.0.5:514 combined;`,
		`access_log /data/logs/app.log;`,
		`error_log stderr warn;`,
		`error_log /dev/null;`,
		`proxy_buffering off; client_max_body_size 1g;`,
	} {
		if err := AdvancedConfig(good); err != nil {
			t.Errorf("refused: %s: %v", good, err)
		}
	}
}

// root/alias: the data volume wherever it is, and directories above it or above the
// system directories.
func TestAdvancedConfigWebRootDataDir(t *testing.T) {
	t.Cleanup(func() { dataDir = "/data" })
	SetDataDir("/srv/corvus/")
	for _, bad := range []string{`root /srv/corvus;`, `root /srv/corvus/certs;`, `alias /srv/;`, `root /srv/x/../corvus;`, `root /var;`, `root /data;`} {
		if err := AdvancedConfig(bad); err == nil {
			t.Errorf("accepted: %s", bad)
		}
	}
	for _, good := range []string{`root /srv/www;`, `root /var/www/html;`, `alias /srv/corvus-site/;`} {
		if err := AdvancedConfig(good); err != nil {
			t.Errorf("refused: %s: %v", good, err)
		}
	}
}
