package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"path/filepath"
	"strings"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/certs"
	"corvus/control/internal/npmimport"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

type npmRequest struct {
	Path        string `json:"path"`             // NPM /data mounted inside the container
	LetsEncrypt string `json:"letsencrypt_path"` // NPM /etc/letsencrypt (default: sibling "letsencrypt")
	// MySQL reads NPM's settings from a MySQL/MariaDB server (NPM's DB_MYSQL_*) instead of
	// Path/database.sqlite. The password is used for this request only and never stored.
	MySQL          *npmimport.MySQL `json:"mysql"`
	ProxyHostIDs   []int64          `json:"proxy_host_ids"`
	RedirectIDs    []int64          `json:"redirection_host_ids"`
	CertificateIDs []int64          `json:"certificate_ids"`
	AccessListIDs  []int64          `json:"access_list_ids"`
}

// source resolves and confines the requested paths to the import root.
func (s *Server) npmSource(in npmRequest) (npmimport.Source, error) {
	root := filepath.Clean(s.NPMImportDir)
	if root == "" || root == "." {
		return npmimport.Source{}, fmt.Errorf("NPM import is not configured")
	}
	data := in.Path
	if data == "" {
		data = filepath.Join(root, "npm", "data")
	}
	le := in.LetsEncrypt
	if le == "" {
		le = filepath.Join(filepath.Dir(filepath.Clean(data)), "letsencrypt")
	}
	for _, p := range []string{data, le} {
		c := filepath.Clean(p)
		if c != root && !strings.HasPrefix(c, root+string(filepath.Separator)) {
			return npmimport.Source{}, fmt.Errorf("paths must be inside %s (mount the NPM volumes there read-only)", root)
		}
	}
	return npmimport.Source{DataDir: filepath.Clean(data), LetsEncryptDir: filepath.Clean(le), MySQL: in.MySQL}, nil
}

func (s *Server) npmRead(w http.ResponseWriter, r *http.Request) (*npmimport.Snapshot, npmRequest, bool) {
	var in npmRequest
	if !decode(w, r, &in) {
		return nil, in, false
	}
	src, err := s.npmSource(in)
	if err != nil {
		fail(w, validate.Errors{{Field: "path", Code: "invalid", Message: err.Error()}})
		return nil, in, false
	}
	snap, err := npmimport.Read(r.Context(), src)
	if err != nil {
		fail(w, validate.Errors{{Field: "path", Code: "unreadable", Message: err.Error()}})
		return nil, in, false
	}
	// mark domain conflicts with existing Corvus hosts
	existing, err := s.Store.ListProxyHosts(r.Context())
	if err != nil {
		fail(w, err)
		return nil, in, false
	}
	redirects, err := s.Store.ListRedirectionHosts(r.Context())
	if err != nil {
		fail(w, err)
		return nil, in, false
	}
	used := map[string]string{}
	for _, h := range existing {
		for _, d := range h.Domains {
			used[d] = fmt.Sprintf("proxy host #%d", h.ID)
		}
	}
	for _, h := range redirects {
		for _, d := range h.Domains {
			used[d] = fmt.Sprintf("redirection host #%d", h.ID)
		}
	}
	conflict := func(domains []string) string {
		for _, d := range domains {
			if what, ok := used[strings.ToLower(strings.TrimSpace(d))]; ok {
				return fmt.Sprintf("%s is already used by Corvus %s", d, what)
			}
		}
		return ""
	}
	for i := range snap.ProxyHosts {
		snap.ProxyHosts[i].Conflict = conflict(snap.ProxyHosts[i].Domains)
	}
	for i := range snap.Redirects {
		snap.Redirects[i].Conflict = conflict(snap.Redirects[i].Domains)
	}
	if snap.Redirects == nil {
		snap.Redirects = []npmimport.Redirect{}
	}
	return snap, in, true
}

func (s *Server) npmPreview(w http.ResponseWriter, r *http.Request) {
	snap, _, ok := s.npmRead(w, r)
	if !ok {
		return
	}
	writeJSON(w, 200, snap)
}

func selected(ids []int64, id int64) bool {
	if ids == nil {
		return true // nothing specified = everything
	}
	for _, v := range ids {
		if v == id {
			return true
		}
	}
	return false
}

type skipped struct {
	Kind   string `json:"kind"`
	NPMID  int64  `json:"npm_id"`
	Reason string `json:"reason"`
}

func (s *Server) npmApply(w http.ResponseWriter, r *http.Request) {
	snap, in, ok := s.npmRead(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	var skips []skipped
	counts := map[string]int{"proxy_hosts": 0, "redirection_hosts": 0, "certificates": 0, "access_lists": 0}
	type pendingFiles struct {
		id        int64
		cert, key []byte
	}
	var files []pendingFiles

	v, err := s.Deployer.Apply(ctx, userID(r), "import from Nginx Proxy Manager", func(tx *store.Store) error {
		certMap := map[int64]int64{}
		for _, c := range snap.Certificates {
			if !selected(in.CertificateIDs, c.NPMID) {
				continue
			}
			if !c.FilesFound {
				skips = append(skips, skipped{"certificate", c.NPMID, "certificate files not found"})
				continue
			}
			info, err := certs.Parse(c.CertPEM, c.KeyPEM)
			if err != nil {
				skips = append(skips, skipped{"certificate", c.NPMID, err.Error()})
				continue
			}
			xc := &store.Certificate{Name: c.Name, Kind: "custom", Domains: info.Domains, NotAfter: info.NotAfter, Status: "valid"}
			if xc.Name == "" {
				xc.Name = strings.Join(info.Domains, ", ")
			}
			// Let's Encrypt certificates are renewed by Corvus's own ACME client later.
			if c.Provider == "letsencrypt" && c.Email != "" {
				xc.Kind, xc.AutoRenew = "acme", true
				xc.ACME = store.ACMEInfo{Email: c.Email, Challenge: "http-01"}
				if c.DNSProvider != "" {
					xc.ACME.Challenge = "dns-01"
					if c.DNSCredentials != nil {
						cred, _ := json.Marshal(c.DNSCredentials)
						sealed, err := seal.Seal(s.DataKey, string(cred))
						if err != nil {
							return err
						}
						xc.ACME.DNSProvider, xc.ACME.Credentials = c.DNSProvider, sealed
					} else {
						xc.AutoRenew = false
					}
				}
			}
			if err := tx.CreateCertificate(ctx, xc); err != nil {
				return err
			}
			certMap[c.NPMID] = xc.ID
			files = append(files, pendingFiles{xc.ID, c.CertPEM, c.KeyPEM})
			counts["certificates"]++
		}
		// Files must exist before the configuration is tested.
		for _, f := range files {
			if err := certs.Write(s.Paths.CertDir(f.id), f.cert, f.key); err != nil {
				return err
			}
		}

		aclMap := map[int64]int64{}
		// IP allow/deny of NPM access lists become security rules scoped to the
		// imported hosts that use the list; the access list itself keeps only users.
		type ipRule struct {
			name, satisfy string
			allow, deny   []string
			users         bool
			hosts         []int64
		}
		ipRules := map[int64]*ipRule{}
		names := map[string]bool{}
		if list, err := tx.ListAccessLists(ctx); err == nil {
			for _, a := range list {
				names[a.Name] = true
			}
		}
		for _, a := range snap.AccessLists {
			if !selected(in.AccessListIDs, a.NPMID) {
				continue
			}
			xa := &store.AccessList{Name: a.Name, Satisfy: "any", Allow: []string{}, Deny: []string{}}
			if a.SatisfyAll {
				xa.Satisfy = "all"
			}
			for names[xa.Name] {
				xa.Name += " (npm)"
			}
			var allow, deny []string
			for _, c := range a.Clients {
				if c.Directive == "deny" {
					deny = append(deny, c.Address)
				} else {
					allow = append(allow, c.Address)
				}
			}
			var err error
			if xa.Allow, err = rules("allow", allow); err != nil {
				skips = append(skips, skipped{"access_list", a.NPMID, err.Error()})
				continue
			}
			if xa.Deny, err = rules("deny", deny); err != nil {
				skips = append(skips, skipped{"access_list", a.NPMID, err.Error()})
				continue
			}
			if len(xa.Allow) > 0 && !contains(xa.Deny, "all") {
				xa.Deny = append(xa.Deny, "all") // NPM appends "deny all" after client rules
			}
			for _, u := range a.Users {
				h, err := auth.HtpasswdHash(u.Password)
				if err != nil {
					return err
				}
				xa.Users = append(xa.Users, store.AccessListUser{Username: u.Username, PasswordHash: h})
			}
			ipRules[a.NPMID] = &ipRule{name: xa.Name, satisfy: xa.Satisfy, allow: xa.Allow, deny: xa.Deny, users: len(xa.Users) > 0}
			if len(xa.Users) == 0 {
				continue // IP-only list: becomes security rules only
			}
			if err := validate.AccessList(xa); err != nil {
				skips = append(skips, skipped{"access_list", a.NPMID, err.Error()})
				delete(ipRules, a.NPMID)
				continue
			}
			if err := tx.SaveAccessList(ctx, xa); err != nil {
				return err
			}
			names[xa.Name] = true
			aclMap[a.NPMID] = xa.ID
			counts["access_lists"]++
		}

		for _, h := range snap.ProxyHosts {
			if !selected(in.ProxyHostIDs, h.NPMID) {
				continue
			}
			if h.Conflict != "" {
				skips = append(skips, skipped{"proxy_host", h.NPMID, h.Conflict})
				continue
			}
			xh := &store.ProxyHost{Domains: h.Domains, ForwardScheme: h.ForwardScheme, ForwardHost: h.ForwardHost, ForwardPort: h.ForwardPort,
				Websocket: h.Websocket, HTTP2: h.HTTP2, BlockExploits: h.BlockExploits, Enabled: h.Enabled, AdvancedConfig: h.AdvancedConfig, ClientIP: "direct",
				SecurityHeaders: "off"} // NPM has no security header presets
			if id, ok := certMap[h.CertificateID]; ok {
				xh.CertificateID, xh.SSLForced = &id, h.SSLForced
			}
			if id, ok := aclMap[h.AccessListID]; ok {
				xh.AccessListID = &id
			}
			if err := checkProxyHost(r, tx, xh); err != nil {
				skips = append(skips, skipped{"proxy_host", h.NPMID, err.Error()})
				continue
			}
			if err := tx.SaveProxyHost(ctx, xh); err != nil {
				return err
			}
			if ir := ipRules[h.AccessListID]; ir != nil {
				ir.hosts = append(ir.hosts, xh.ID)
			}
			counts["proxy_hosts"]++
		}
		for _, h := range snap.Redirects {
			if !selected(in.RedirectIDs, h.NPMID) {
				continue
			}
			if h.Conflict != "" {
				skips = append(skips, skipped{"redirection_host", h.NPMID, h.Conflict})
				continue
			}
			xh := npmRedirect(h, certMap)
			if err := checkRedirectionHost(r, tx, xh); err != nil {
				skips = append(skips, skipped{"redirection_host", h.NPMID, err.Error()})
				continue
			}
			if err := tx.SaveRedirectionHost(ctx, xh); err != nil {
				return err
			}
			counts["redirection_hosts"]++
		}
		for _, ir := range ipRules {
			if len(ir.hosts) == 0 {
				continue
			}
			for _, rule := range store.AccessListRules(ir.name, ir.satisfy, ir.allow, ir.deny, ir.users) {
				rule.Scope, rule.HostIDs = "hosts", ir.hosts
				if err := tx.SaveSecurityRule(ctx, &rule); err != nil {
					return err
				}
				counts["security_rules"]++
			}
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	if skips == nil {
		skips = []skipped{}
	}
	s.audit(r, "import.npm", "npm", map[string]any{"imported": counts, "skipped": len(skips)})
	writeJSON(w, 200, map[string]any{"imported": counts, "skipped": skips, "version": v, "at": time.Now().UTC()})
}

// npmRedirect maps an NPM redirection host to a Corvus one; certMap maps NPM
// certificate ids to imported Corvus certificates (unmapped: no TLS, no forced HTTPS).
func npmRedirect(h npmimport.Redirect, certMap map[int64]int64) *store.RedirectionHost {
	xh := &store.RedirectionHost{Domains: h.Domains, ForwardScheme: h.ForwardScheme, ForwardDomain: h.ForwardDomain, HTTPCode: h.HTTPCode,
		PreservePath: h.PreservePath, HTTP2: h.HTTP2, BlockExploits: h.BlockExploits, Enabled: h.Enabled, AdvancedConfig: h.AdvancedConfig}
	if id, ok := certMap[h.CertificateID]; ok && h.CertificateID != 0 {
		xh.CertificateID, xh.SSLForced = &id, h.SSLForced
	}
	return xh
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
