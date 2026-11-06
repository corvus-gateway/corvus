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
