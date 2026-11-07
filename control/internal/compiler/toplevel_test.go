package compiler

import "testing"

func TestTopLevelDirective(t *testing.T) {
	for cfg, want := range map[string]bool{
		"client_max_body_size 1g;":                             true,
		"location /u { client_max_body_size 1g; }":             false,
		"# client_max_body_size 1g;\nproxy_read_timeout 5s;":   false,
		`add_header X "client_max_body_size 1g;";`:             false,
		"location /a { } client_max_body_size 5m;":             true,
		"client_max_body_size_x 1;":                            false,
		"if ($a) { return 404; }\n  client_max_body_size\t2m;": true,
	} {
		if got := topLevelDirective(cfg, "client_max_body_size"); got != want {
			t.Errorf("%q: got %v, want %v", cfg, got, want)
		}
	}
	if topLevelDirective(`location ~ \.php$ { error_page 404 /x; }`, "error_page") {
		t.Error("nested error_page counted")
	}
	if !topLevelDirective("error_page 502 /e.html;", "error_page") || !topLevelDirective("gzip off;", "gzip") || topLevelDirective("gzip_types text/css;", "gzip") {
		t.Error("top-level directives")
	}
}
