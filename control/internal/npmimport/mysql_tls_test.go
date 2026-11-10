package npmimport

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestMySQLTLSMode(t *testing.T) {
	for host, want := range map[string]string{
		"db":                   "preferred",
		"npm-db":               "preferred",
		"localhost":            "preferred",
		"127.0.0.1":            "preferred",
		"::1":                  "preferred",
		"[fd00::5]":            "preferred",
		"172.17.0.3":           "preferred",
		"192.168.1.20":         "preferred",
		"nas.local":            "preferred",
		"db.home.arpa.":        "preferred",
		"mysql.corp.internal":  "preferred",
		"8.8.8.8":              "true",
		"2606:4700::1":         "true",
		"db.example.com":       "true",
		"mysql.example.com.":   "true",
		"localhost.example.io": "true",
	} {
		if got := mysqlTLSMode(host); got != want {
			t.Errorf("mysqlTLSMode(%q) = %q, want %q", host, got, want)
		}
	}
}

func TestOpenMySQLRemoteErrorExplainsTLS(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_, err := open(ctx, Source{MySQL: &MySQL{Host: "npm-db.example.invalid", User: "npm", Password: "pw-secret", Database: "npm"}})
	if err == nil || !strings.Contains(err.Error(), "must offer TLS") || strings.Contains(err.Error(), "pw-secret") {
		t.Fatalf("err = %v", err)
	}
}
