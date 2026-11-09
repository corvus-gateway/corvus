package api

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/store"
)

// ------------------------------------------------------------------ access queues

func TestZZHWaitrooms(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer := e.as("viewer")

	m := e.zzhExpect(201, "POST", "/api/waitrooms", map[string]any{"name": " Shop ", "capacity": 50})
	wr := zzhObj(t, m, "waitroom")
	wid := id(wr)
	if wr["name"] != "shop" || wr["active_ttl"] != float64(120) || wr["grant_ttl"] != float64(30) || wr["queue_ttl"] != float64(120) ||
		wr["refresh"] != float64(15) || wr["fail_mode"] != "open" || wr["max_queue"] != float64(100000) || wr["max_new_per_ip"] != float64(60) ||
		wr["enabled"] != true {
		t.Fatalf("defaults: %v", wr)
	}
	if zzhObj(t, m, "version")["status"] != "active" {
		t.Fatalf("not deployed: %v", m)
	}
	m = e.zzhExpect(201, "POST", "/api/waitrooms", map[string]any{"name": "tickets", "capacity": 5, "enabled": false, "active_ttl": 600,
		"grant_ttl": 60, "queue_ttl": 300, "refresh": 20, "admit_rate": 3, "fail_mode": "closed", "max_queue": 0, "max_new_per_ip": 0, "bind_ip": true})
	tk := zzhObj(t, m, "waitroom")
	if tk["max_queue"] != float64(0) || tk["max_new_per_ip"] != float64(0) || tk["bind_ip"] != true || tk["enabled"] != false || tk["fail_mode"] != "closed" {
		t.Fatalf("explicit values: %v", tk)
	}

	m = e.zzhExpect(422, "POST", "/api/waitrooms", map[string]any{"name": "shop", "capacity": 1})
	zzhHasField(t, m, "name", "duplicate")
	m = e.zzhExpect(422, "POST", "/api/waitrooms", map[string]any{"name": "Bad Name!", "capacity": 0, "refresh": 100, "fail_mode": "maybe"})
	for _, f := range []string{"name:invalid", "capacity:range", "refresh:too_long", "fail_mode:invalid"} {
		p := strings.SplitN(f, ":", 2)
		zzhHasField(t, m, p[0], p[1])
	}
	e.zzhExpect(400, "POST", "/api/waitrooms", map[string]any{"name": 1})

	code, l := e.callList("GET", "/api/waitrooms")
	if code != 200 || len(l) != 2 {
		t.Fatalf("list: %d %v", code, l)
	}
	if live, _ := l[0].(map[string]any)["live"].(map[string]any); live == nil || live["name"] != "shop" || live["capacity"] != float64(50) {
		t.Fatalf("live stats: %v", l[0])
	}
	if m := e.zzhExpect(200, "GET", zzhPath("/api/waitrooms", wid)+"/live", nil); m["name"] != "shop" {
		t.Fatalf("live: %v", m)
	}
	e.zzhExpect(404, "GET", "/api/waitrooms/999/live", nil)
	e.zzhExpect(400, "GET", "/api/waitrooms/x/live", nil)

	// update: the name is immutable, the rest is validated
	m = e.zzhExpect(422, "PUT", zzhPath("/api/waitrooms", wid), map[string]any{"name": "other", "capacity": 50})
	zzhHasField(t, m, "name", "immutable")
	m = e.zzhExpect(422, "PUT", zzhPath("/api/waitrooms", wid), map[string]any{"name": "shop", "capacity": -1})
	zzhHasField(t, m, "capacity", "range")
	m = e.zzhExpect(200, "PUT", zzhPath("/api/waitrooms", wid), map[string]any{"name": "shop", "capacity": 75, "admit_rate": 10})
	if w := zzhObj(t, m, "waitroom"); w["capacity"] != float64(75) || w["admit_rate"] != float64(10) {
		t.Fatalf("update: %v", w)
	}
	if w, _ := e.s.Store.WaitroomByID(ctx, wid); w.Capacity != 75 {
		t.Fatalf("stored: %+v", w)
	}
	e.zzhExpect(404, "PUT", "/api/waitrooms/999", map[string]any{"name": "x", "capacity": 1})
	e.zzhExpect(400, "PUT", "/api/waitrooms/0", map[string]any{"name": "x", "capacity": 1})
	e.zzhExpect(400, "PUT", zzhPath("/api/waitrooms", wid), map[string]any{"nope": true})

	// roles
	if code, _ := e.zzhAs(viewer, "GET", "/api/waitrooms", nil); code != 200 {
		t.Fatalf("viewer list: %d", code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/waitrooms"}, {"PUT", zzhPath("/api/waitrooms", wid)}, {"DELETE", zzhPath("/api/waitrooms", wid)},
	} {
		if code, _ := e.zzhAs(viewer, c.method, c.path, map[string]any{"name": "v", "capacity": 1}); code != 403 {
			t.Fatalf("viewer %s %s: %d", c.method, c.path, code)
		}
	}

	e.zzhExpect(200, "DELETE", zzhPath("/api/waitrooms", wid), nil)
	e.zzhExpect(404, "DELETE", zzhPath("/api/waitrooms", wid), nil)
	e.zzhExpect(400, "DELETE", "/api/waitrooms/-1", nil)
	for _, a := range []string{"waitroom.create", "waitroom.update", "waitroom.delete"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}

// ------------------------------------------------------------------ certificates

func TestZZHCertificates(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer := e.as("viewer")

	certPEM, keyPEM := selfSigned(t, 10, time.Now().Add(60*24*time.Hour), "c.example.com", "www.c.example.com")
	m := e.zzhExpect(201, "POST", "/api/certificates", map[string]any{"certificate": certPEM, "private_key": keyPEM})
	cid := id(m)
	if m["name"] != "c.example.com" || m["kind"] != "custom" || m["status"] != "valid" || len(m["domains"].([]any)) != 2 {
		t.Fatalf("upload: %v", m)
	}
	if b, err := os.ReadFile(filepath.Join(e.s.Paths.CertDir(cid), "fullchain.pem")); err != nil || string(b) != certPEM {
		t.Fatalf("certificate file: %v", err)
	}
	m = e.zzhExpect(201, "POST", "/api/certificates", map[string]any{"name": "  Named  ", "certificate": certPEM, "private_key": keyPEM})
	named := id(m)
	if m["name"] != "Named" {
		t.Fatalf("named upload: %v", m)
	}
	otherPEM, otherKey := selfSigned(t, 11, time.Now().Add(time.Hour), "other.example.com")
	m = e.zzhExpect(422, "POST", "/api/certificates", map[string]any{"certificate": certPEM, "private_key": otherKey})
	zzhHasField(t, m, "certificate", "invalid")
	m = e.zzhExpect(422, "POST", "/api/certificates", map[string]any{"certificate": "junk", "private_key": "junk"})
	zzhHasField(t, m, "certificate", "invalid")
	e.zzhExpect(400, "POST", "/api/certificates", map[string]any{"cert": certPEM})

	if code, l := e.callList("GET", "/api/certificates"); code != 200 || len(l) != 2 {
		t.Fatalf("list: %d %v", code, l)
	}
	if code, _ := e.zzhAs(viewer, "GET", "/api/certificates", nil); code != 200 {
		t.Fatalf("viewer list: %d", code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/certificates"}, {"PUT", zzhPath("/api/certificates", cid)}, {"DELETE", zzhPath("/api/certificates", cid)},
	} {
		if code, _ := e.zzhAs(viewer, c.method, c.path, map[string]any{}); code != 403 {
			t.Fatalf("viewer %s %s: %d", c.method, c.path, code)
		}
	}

	// replace in place: same id, new pair; the name is kept unless given
	hid := e.zzhCreateHost(zzhWith(zzhHost("c.example.com"), "certificate_id", cid, "ssl_forced", true))
	m = e.zzhExpect(200, "PUT", zzhPath("/api/certificates", cid), map[string]any{"certificate": otherPEM, "private_key": otherKey})
	rc := zzhObj(t, m, "certificate")
	if id(rc) != cid || rc["name"] != "c.example.com" || rc["domains"].([]any)[0] != "other.example.com" || zzhObj(t, m, "version")["status"] != "active" {
		t.Fatalf("replace: %v", m)
	}
	if b, _ := os.ReadFile(filepath.Join(e.s.Paths.CertDir(cid), "fullchain.pem")); string(b) != otherPEM {
		t.Fatal("certificate file not replaced")
	}
	m = e.zzhExpect(200, "PUT", zzhPath("/api/certificates", cid), map[string]any{"name": "Renamed", "certificate": certPEM, "private_key": keyPEM})
	if zzhObj(t, m, "certificate")["name"] != "Renamed" {
		t.Fatalf("replace with name: %v", m)
	}
	m = e.zzhExpect(422, "PUT", zzhPath("/api/certificates", cid), map[string]any{"certificate": "junk", "private_key": "junk"})
	zzhHasField(t, m, "certificate", "invalid")
	e.zzhExpect(404, "PUT", "/api/certificates/999", map[string]any{"certificate": certPEM, "private_key": keyPEM})
	e.zzhExpect(400, "PUT", "/api/certificates/abc", map[string]any{})
	e.zzhExpect(400, "PUT", zzhPath("/api/certificates", cid), map[string]any{"x": 1})
	acme := &store.Certificate{Name: "le", Kind: "acme", Domains: []string{"le.example.com"}, Status: "pending"}
	if err := e.s.Store.CreateCertificate(ctx, acme); err != nil {
		t.Fatal(err)
	}
	m = e.zzhExpect(422, "PUT", zzhPath("/api/certificates", acme.ID), map[string]any{"certificate": certPEM, "private_key": keyPEM})
	zzhHasField(t, m, "certificate", "not_custom")

	// a rejected deployment puts the previous files back
	e.zzhBreakGateway()
	m = e.zzhExpect(422, "PUT", zzhPath("/api/certificates", cid), map[string]any{"certificate": otherPEM, "private_key": otherKey})
	if m["error"] != "config_rejected" {
		t.Fatalf("rejected replace: %v", m)
	}
	if b, _ := os.ReadFile(filepath.Join(e.s.Paths.CertDir(cid), "fullchain.pem")); string(b) != certPEM {
		t.Fatal("previous certificate file not restored")
	}
	if c, _ := e.s.Store.CertificateByID(ctx, cid); c.Name != "Renamed" || c.Domains[0] != "c.example.com" {
		t.Fatalf("rejected replace stored: %+v", c)
	}

	// delete: in use → 409, unknown → 404
	if code, m := e.call("DELETE", zzhPath("/api/certificates", cid), nil); code != 409 || m["error"] != "conflict" {
		t.Fatalf("delete used: %d %v", code, m)
	}
	_ = e.s.Store.DeleteProxyHost(ctx, hid)
	if m := e.zzhExpect(200, "DELETE", zzhPath("/api/certificates", cid), nil); m["status"] != "ok" {
		t.Fatalf("delete: %v", m)
	}
	e.zzhExpect(200, "DELETE", zzhPath("/api/certificates", named), nil)
	e.zzhExpect(404, "DELETE", zzhPath("/api/certificates", cid), nil)
	e.zzhExpect(400, "DELETE", "/api/certificates/0", nil)
	for _, a := range []string{"certificate.upload", "certificate.replace", "certificate.delete"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}

// ------------------------------------------------------------------ deployments & audit

func TestZZHDeployments(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	viewer := e.as("viewer")

	m := e.zzhExpect(200, "GET", "/api/deployments", nil)
	if m["total"] != float64(0) || m["dirty"] != true || m["limit"] != float64(50) || m["offset"] != float64(0) {
		t.Fatalf("empty deployments: %v", m)
	}

	v1 := e.zzhExpect(201, "POST", "/api/deployments", nil)
	if v1["message"] != "manual deployment" || v1["status"] != "active" {
		t.Fatalf("redeploy: %v", v1)
	}
	v1id := id(v1)
	e.zzhCreateHost(zzhHost("d.example.com"))
	v3 := e.zzhExpect(201, "POST", "/api/deployments", map[string]any{"message": "  after host  "})
	if v3["message"] != "after host" {
		t.Fatalf("redeploy with message: %v", v3)
	}
	e.zzhExpect(400, "POST", "/api/deployments", map[string]any{"msg": "x"})
	if code, _ := e.zzhAs(viewer, "POST", "/api/deployments", nil); code != 403 {
		t.Fatalf("viewer redeploy: %d", code)
	}

	m = e.zzhExpect(200, "GET", "/api/deployments?limit=2&offset=1", nil)
	if m["total"] != float64(3) || m["limit"] != float64(2) || m["offset"] != float64(1) || len(m["versions"].([]any)) != 2 || m["dirty"] != false {
		t.Fatalf("paged: %v", m)
	}
	m = e.zzhExpect(200, "GET", "/api/deployments?limit=9999&offset=-5", nil)
	if m["limit"] != float64(50) || m["offset"] != float64(0) || len(m["versions"].([]any)) != 3 {
		t.Fatalf("clamped: %v", m)
	}

	// files of a version (viewer may read them)
	code, f := e.zzhAs(viewer, "GET", zzhPath("/api/deployments", id(v3))+"/files", nil)
	if code != 200 || len(f["names"].([]any)) == 0 || f["files"].(map[string]any)["nginx.conf"] == nil {
		t.Fatalf("files: %d %v", code, f["names"])
	}
	hostConf := false
	for n, body := range f["files"].(map[string]any) {
		if strings.HasPrefix(n, "hosts/") && strings.Contains(fmt.Sprint(body), "d.example.com") {
			hostConf = true
		}
	}
	if !hostConf {
		t.Fatalf("host configuration missing from %v", f["names"])
	}
	e.zzhExpect(404, "GET", "/api/deployments/999/files", nil)
	e.zzhExpect(400, "GET", "/api/deployments/x/files", nil)

	// rollback to the first version
	rb := e.zzhExpect(201, "POST", zzhPath("/api/deployments", v1id)+"/rollback", nil)
	if rb["rollback_of"] != float64(v1id) || rb["status"] != "active" || !strings.Contains(fmt.Sprint(rb["message"]), "rollback") {
		t.Fatalf("rollback: %v", rb)
	}
	if m := e.zzhExpect(200, "GET", "/api/deployments", nil); m["dirty"] != true {
		t.Fatalf("rollback should leave the settings undeployed: %v", m)
	}
	e.zzhExpect(404, "POST", "/api/deployments/999/rollback", nil)
	e.zzhExpect(400, "POST", "/api/deployments/0/rollback", nil)
	if code, _ := e.zzhAs(viewer, "POST", zzhPath("/api/deployments", v1id)+"/rollback", nil); code != 403 {
		t.Fatalf("viewer rollback: %d", code)
	}

	// a failed version has no files and cannot be rolled back to
	failed := e.zzhExpect(422, "POST", "/api/proxy-hosts", zzhWith(zzhHost("r.example.com"), "advanced_config", "REJECT on;"))
	fid := int64(failed["version_id"].(float64))
	if f := e.zzhExpect(200, "GET", zzhPath("/api/deployments", fid)+"/files", nil); len(f["files"].(map[string]any)) != 0 {
		t.Fatalf("failed version files: %v", f)
	}
	if code, m := e.call("POST", zzhPath("/api/deployments", fid)+"/rollback", nil); code != 409 || !strings.Contains(fmt.Sprint(m["message"]), "never deployed") {
		t.Fatalf("rollback to failed: %d %v", code, m)
	}
	// files that no longer pass the gateway test
	conf := filepath.Join(e.dir, "nginx", "versions", fmt.Sprint(v3["dir"]), "nginx.conf")
	b, err := os.ReadFile(conf)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(conf, append(b, []byte("\n# REJECT\n")...), 0o644); err != nil {
		t.Fatal(err)
	}
	if code, m := e.call("POST", zzhPath("/api/deployments", id(v3))+"/rollback", nil); code != 422 || m["error"] != "config_rejected" {
		t.Fatalf("rollback rejected by the gateway: %d %v", code, m)
	}
	// pruned files
	if err := os.RemoveAll(filepath.Dir(conf)); err != nil {
		t.Fatal(err)
	}
	if code, m := e.call("POST", zzhPath("/api/deployments", id(v3))+"/rollback", nil); code != 409 || !strings.Contains(fmt.Sprint(m["message"]), "pruned") {
		t.Fatalf("rollback to pruned version: %d %v", code, m)
	}
	// a redeploy that the gateway rejects
	e.zzhBreakGateway()
	if code, m := e.call("POST", "/api/deployments", map[string]any{"message": "x"}); code != 422 || m["error"] != "config_rejected" {
		t.Fatalf("rejected redeploy: %d %v", code, m)
	}
	for _, a := range []string{"deploy.apply", "deploy.rollback", "deploy.files_viewed"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}

func TestZZHAudit(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	for i := 0; i < 3; i++ {
		e.zzhCreateHost(zzhHost(fmt.Sprintf("a%d.example.com", i)))
	}
	code, l := e.callList("GET", "/api/audit?limit=2")
	if code != 200 || len(l) != 2 || l[0].(map[string]any)["action"] != "proxy_host.create" {
		t.Fatalf("limited audit: %d %v", code, l)
	}
	if _, l = e.callList("GET", "/api/audit?limit=0"); len(l) != 3 {
		t.Fatalf("default limit: %v", l)
	}
	if _, l = e.callList("GET", "/api/audit?limit=100000"); len(l) != 3 {
		t.Fatalf("clamped limit: %v", l)
	}
	if code, _ := e.zzhAs(e.as("viewer"), "GET", "/api/audit", nil); code != 200 {
		t.Fatalf("viewer audit: %d", code)
	}
}

// ------------------------------------------------------------------ redirection hosts

func TestZZHRedirectionHosts(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer, operator := e.as("viewer"), e.as("operator")
	body := func(domain string, kv ...any) map[string]any {
		return zzhWith(map[string]any{"domains": []string{domain}, "forward_domain": "Target.Example.org/path"}, kv...)
	}

	m := e.zzhExpect(201, "POST", "/api/redirection-hosts", body("old.example.com", "notes", "moved"))
	rh := zzhObj(t, m, "redirection_host")
	rid := id(rh)
	if rh["forward_scheme"] != "auto" || rh["http_code"] != float64(301) || rh["preserve_path"] != true || rh["block_exploits"] != true ||
		rh["forward_domain"] != "target.example.org/path" || rh["notes"] != "moved" || m["certificate"] != nil {
		t.Fatalf("create: %v", m)
	}
	got := e.zzhExpect(200, "GET", zzhPath("/api/redirection-hosts", rid), nil)
	if got["http_code"] != float64(301) {
		t.Fatalf("get: %v", got)
	}
	e.zzhExpect(404, "GET", "/api/redirection-hosts/999", nil)
	e.zzhExpect(400, "GET", "/api/redirection-hosts/x", nil)
	if code, l := e.callList("GET", "/api/redirection-hosts"); code != 200 || len(l) != 1 {
		t.Fatalf("list: %d %v", code, l)
	}

	// validation
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", map[string]any{"domains": []string{}, "forward_domain": "https://x.example.com",
		"http_code": 200, "forward_scheme": "ftp", "ssl_forced": true})
	for _, f := range []string{"domains", "forward_domain", "http_code", "forward_scheme", "ssl_forced"} {
		zzhHasField(t, m, f, "")
	}
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", body("x.example.com", "forward_domain", "x.example.org:99999"))
	zzhHasField(t, m, "forward_domain", "invalid")
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", body("OLD.example.com"))
	zzhHasField(t, m, "domains", "duplicate")
	hid := e.zzhCreateHost(zzhHost("proxy.example.com"))
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", body("proxy.example.com"))
	if !strings.Contains(fmt.Sprint(m["message"]), fmt.Sprintf("proxy host #%d", hid)) {
		t.Fatalf("proxy host conflict: %v", m)
	}
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", body("c.example.com", "certificate_id", 999))
	zzhHasField(t, m, "certificate_id", "not_found")
	e.zzhExpect(400, "POST", "/api/redirection-hosts", map[string]any{"domain": "x"})

	// update
	m = e.zzhExpect(200, "PUT", zzhPath("/api/redirection-hosts", rid), body("old.example.com", "http_code", 308, "preserve_path", false,
		"forward_scheme", "https", "enabled", false, "http2", false, "block_exploits", false))
	rh = zzhObj(t, m, "redirection_host")
	if rh["http_code"] != float64(308) || rh["preserve_path"] != false || rh["enabled"] != false || rh["notes"] != "moved" {
		t.Fatalf("update: %v", rh)
	}
	if h, _ := e.s.Store.RedirectionHostByID(ctx, rid); h.HTTPCode != 308 || h.ForwardScheme != "https" {
		t.Fatalf("stored: %+v", h)
	}
	m = e.zzhExpect(422, "PUT", zzhPath("/api/redirection-hosts", rid), body("old.example.com", "http_code", 404))
	zzhHasField(t, m, "http_code", "invalid")
	e.zzhExpect(404, "PUT", "/api/redirection-hosts/999", body("z.example.com"))
	e.zzhExpect(400, "PUT", "/api/redirection-hosts/0", body("z.example.com"))
	e.zzhExpect(400, "PUT", zzhPath("/api/redirection-hosts", rid), map[string]any{"bogus": true})

	// roles and advanced configuration
	if code, _ := e.zzhAs(viewer, "GET", "/api/redirection-hosts", nil); code != 200 {
		t.Fatalf("viewer list: %d", code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/redirection-hosts"}, {"PUT", zzhPath("/api/redirection-hosts", rid)}, {"DELETE", zzhPath("/api/redirection-hosts", rid)},
	} {
		if code, _ := e.zzhAs(viewer, c.method, c.path, body("v.example.com")); code != 403 {
			t.Fatalf("viewer %s %s: %d", c.method, c.path, code)
		}
	}
	code, m := e.zzhAs(operator, "POST", "/api/redirection-hosts", body("op.example.com", "advanced_config", "add_header X 1;"))
	if code != 422 {
		t.Fatalf("operator advanced create: %d %v", code, m)
	}
	zzhHasField(t, m, "advanced_config", "forbidden")
	code, m = e.zzhAs(operator, "PUT", zzhPath("/api/redirection-hosts", rid), body("old.example.com", "advanced_config", "add_header X 1;"))
	if code != 422 {
		t.Fatalf("operator advanced update: %d %v", code, m)
	}
	zzhHasField(t, m, "advanced_config", "forbidden")
	if code, _ := e.zzhAs(operator, "POST", "/api/redirection-hosts", body("op.example.com")); code != 201 {
		t.Fatalf("operator create: %d", code)
	}

	// the gateway rejects the configuration: nothing is stored
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", body("rej.example.com", "advanced_config", "REJECT on;"))
	if m["error"] != "config_rejected" {
		t.Fatalf("rejected create: %v", m)
	}
	m = e.zzhExpect(422, "PUT", zzhPath("/api/redirection-hosts", rid), body("old.example.com", "advanced_config", "REJECT on;"))
	if m["error"] != "config_rejected" {
		t.Fatalf("rejected update: %v", m)
	}
	if h, _ := e.s.Store.RedirectionHostByID(ctx, rid); h.AdvancedConfig != "" {
		t.Fatalf("rejected update stored: %+v", h)
	}

	// certificates ordered with the host
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", body("le.example.com", "certificate_request", map[string]any{"email": "ops@example.com"}))
	zzhHasField(t, m, "certificate_request", "unavailable")
	rec := &startRecorder{}
	e.s.ACME = rec
	m = e.zzhExpect(201, "POST", "/api/redirection-hosts", body("le.example.com", "certificate_request", map[string]any{"email": "ops@example.com"}))
	cert := zzhObj(t, m, "certificate")
	if zzhObj(t, m, "redirection_host")["certificate_id"] != float64(id(cert)) || len(rec.started) != 1 || cert["status"] != "pending" {
		t.Fatalf("redirect with order: %v %v", m, rec.started)
	}
	m = e.zzhExpect(200, "PUT", zzhPath("/api/redirection-hosts", rid), body("old.example.com", "certificate_request", map[string]any{"email": "ops@example.com"}, "ssl_forced", true))
	if zzhObj(t, m, "redirection_host")["certificate_id"] != float64(id(zzhObj(t, m, "certificate"))) || len(rec.started) != 2 {
		t.Fatalf("update with order: %v", m)
	}
	m = e.zzhExpect(422, "POST", "/api/redirection-hosts", body("le2.example.com", "certificate_request", map[string]any{"email": "bad"}))
	zzhHasField(t, m, "certificate_request.email", "invalid")
	m = e.zzhExpect(422, "PUT", zzhPath("/api/redirection-hosts", rid), body("old.example.com", "certificate_request", map[string]any{"email": "bad"}))
	zzhHasField(t, m, "certificate_request.email", "invalid")

	// delete
	e.zzhExpect(200, "DELETE", zzhPath("/api/redirection-hosts", rid), nil)
	e.zzhExpect(404, "DELETE", zzhPath("/api/redirection-hosts", rid), nil)
	e.zzhExpect(400, "DELETE", "/api/redirection-hosts/abc", nil)
	for _, a := range []string{"redirection_host.create", "redirection_host.update", "redirection_host.delete"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}

// ------------------------------------------------------------------ streams

func TestZZHStreams(t *testing.T) {
	t.Parallel()
	e := newFullEnv(t)
	ctx := context.Background()
	viewer := e.as("viewer")

	m := e.zzhExpect(201, "POST", "/api/streams", map[string]any{"incoming_port": 5432, "forward_host": " DB ", "forward_port": 5432, "notes": "pg"})
	s := zzhObj(t, m, "stream")
	sid := id(s)
	if s["tcp_forwarding"] != true || s["udp_forwarding"] != false || s["enabled"] != true || s["forward_host"] != "db" || s["notes"] != "pg" {
		t.Fatalf("create: %v", s)
	}
	if zzhObj(t, m, "version")["status"] != "active" {
		t.Fatalf("not deployed: %v", m)
	}
	if got := e.zzhExpect(200, "GET", zzhPath("/api/streams", sid), nil); got["incoming_port"] != float64(5432) {
		t.Fatalf("get: %v", got)
	}
	e.zzhExpect(404, "GET", "/api/streams/999", nil)
	e.zzhExpect(400, "GET", "/api/streams/x", nil)
	if code, l := e.callList("GET", "/api/streams"); code != 200 || len(l) != 1 {
		t.Fatalf("list: %d %v", code, l)
	}

	// validation and conflicts
	m = e.zzhExpect(422, "POST", "/api/streams", map[string]any{"incoming_port": 0, "tcp_forwarding": false, "forward_host": "bad host", "forward_port": 70000})
	for _, f := range []string{"incoming_port", "tcp_forwarding", "forward_host", "forward_port"} {
		zzhHasField(t, m, f, "")
	}
	m = e.zzhExpect(422, "POST", "/api/streams", map[string]any{"incoming_port": 443, "forward_host": "x", "forward_port": 1})
	zzhHasField(t, m, "incoming_port", "conflict")
	m = e.zzhExpect(422, "POST", "/api/streams", map[string]any{"incoming_port": 5432, "forward_host": "x", "forward_port": 1})
	zzhHasField(t, m, "incoming_port", "conflict")
	// UDP on the same port does not conflict with TCP
	udp := id(zzhObj(t, e.zzhExpect(201, "POST", "/api/streams", map[string]any{"incoming_port": 5432, "tcp_forwarding": false, "udp_forwarding": true,
		"forward_host": "10.0.0.5", "forward_port": 53}), "stream"))
	m = e.zzhExpect(422, "POST", "/api/streams", map[string]any{"incoming_port": 6000, "forward_host": "x", "forward_port": 1, "certificate_id": 999})
	zzhHasField(t, m, "certificate_id", "not_found")
	m = e.zzhExpect(422, "POST", "/api/streams", map[string]any{"incoming_port": 6001, "tcp_forwarding": false, "udp_forwarding": true,
		"forward_host": "x", "forward_port": 1, "certificate_id": 1})
	zzhHasField(t, m, "certificate_id", "needs_tcp")
	m = e.zzhExpect(422, "POST", "/api/streams", map[string]any{"incoming_port": 6002, "forward_host": "::1", "forward_port": 1})
	zzhHasField(t, m, "forward_host", "ipv6_unsupported")
	e.zzhExpect(400, "POST", "/api/streams", map[string]any{"port": 1})

	// TLS termination with an uploaded certificate
	certPEM, keyPEM := selfSigned(t, 20, time.Now().Add(30*24*time.Hour), "tls.example.com")
	cid := id(e.zzhExpect(201, "POST", "/api/certificates", map[string]any{"certificate": certPEM, "private_key": keyPEM}))
	m = e.zzhExpect(201, "POST", "/api/streams", map[string]any{"incoming_port": 6443, "forward_host": "app", "forward_port": 6443, "certificate_id": cid})
	if zzhObj(t, m, "stream")["certificate_id"] != float64(cid) {
		t.Fatalf("stream with certificate: %v", m)
	}
	if code, _ := e.call("DELETE", zzhPath("/api/certificates", cid), nil); code != 409 {
		t.Fatalf("certificate used by a stream deleted: %d", code)
	}

	// update
	m = e.zzhExpect(200, "PUT", zzhPath("/api/streams", sid), map[string]any{"incoming_port": 5433, "forward_host": "db", "forward_port": 5433, "enabled": false})
	if s := zzhObj(t, m, "stream"); s["incoming_port"] != float64(5433) || s["enabled"] != false || s["notes"] != "pg" {
		t.Fatalf("update: %v", s)
	}
	if st, _ := e.s.Store.StreamByID(ctx, sid); st.IncomingPort != 5433 || st.Enabled {
		t.Fatalf("stored: %+v", st)
	}
	m = e.zzhExpect(422, "PUT", zzhPath("/api/streams", udp), map[string]any{"incoming_port": 80, "tcp_forwarding": false, "udp_forwarding": true, "forward_host": "x", "forward_port": 1})
	zzhHasField(t, m, "incoming_port", "conflict")
	e.zzhExpect(404, "PUT", "/api/streams/999", map[string]any{"incoming_port": 7000, "forward_host": "x", "forward_port": 1})
	e.zzhExpect(400, "PUT", "/api/streams/0", map[string]any{})
	e.zzhExpect(400, "PUT", zzhPath("/api/streams", sid), map[string]any{"nope": 1})

	// roles
	if code, _ := e.zzhAs(viewer, "GET", "/api/streams", nil); code != 200 {
		t.Fatalf("viewer list: %d", code)
	}
	for _, c := range []struct{ method, path string }{
		{"POST", "/api/streams"}, {"PUT", zzhPath("/api/streams", sid)}, {"DELETE", zzhPath("/api/streams", sid)},
	} {
		if code, _ := e.zzhAs(viewer, c.method, c.path, map[string]any{"incoming_port": 7001, "forward_host": "x", "forward_port": 1}); code != 403 {
			t.Fatalf("viewer %s %s: %d", c.method, c.path, code)
		}
	}

	// a rejected deployment stores nothing
	e.zzhBreakGateway()
	if m := e.zzhExpect(422, "POST", "/api/streams", map[string]any{"incoming_port": 7002, "forward_host": "x", "forward_port": 1}); m["error"] != "config_rejected" {
		t.Fatalf("rejected create: %v", m)
	}
	if l, _ := e.s.Store.ListStreams(ctx); len(l) != 3 {
		t.Fatalf("rejected stream stored: %d", len(l))
	}
	if m := e.zzhExpect(422, "DELETE", zzhPath("/api/streams", sid), nil); m["error"] != "config_rejected" {
		t.Fatalf("rejected delete: %v", m)
	}
	if _, err := e.s.Store.StreamByID(ctx, sid); err != nil {
		t.Fatalf("stream deleted despite the rejection: %v", err)
	}
	if err := e.s.Store.DeleteProxyHost(ctx, zzhBrokenHostID(t, e)); err != nil {
		t.Fatal(err)
	}

	e.zzhExpect(200, "DELETE", zzhPath("/api/streams", sid), nil)
	e.zzhExpect(404, "DELETE", zzhPath("/api/streams", sid), nil)
	e.zzhExpect(400, "DELETE", "/api/streams/x", nil)
	for _, a := range []string{"stream.create", "stream.update", "stream.delete"} {
		if !e.zzhAudited(a) {
			t.Errorf("audit %s missing", a)
		}
	}
}

// zzhBrokenHostID returns the id of the host stored by zzhBreakGateway.
func zzhBrokenHostID(t *testing.T, e *fullEnv) int64 {
	t.Helper()
	l, _ := e.s.Store.ListProxyHosts(context.Background())
	for _, h := range l {
		if h.AdvancedConfig == "REJECT on;" {
			return h.ID
		}
	}
	t.Fatal("no broken host")
	return 0
}
