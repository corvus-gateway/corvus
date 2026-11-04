package api

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"corvus/control/internal/certs"
	"corvus/control/internal/deploy"
	"corvus/control/internal/live"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// ------------------------------------------------------------------ overview

func (s *Server) handleOverview(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	hosts, err := s.Store.ListProxyHosts(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	rooms, err := s.Store.ListWaitrooms(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	certList, err := s.Store.ListCertificates(ctx)
	if err != nil {
		fail(w, err)
		return
	}
	enabled := 0
	for _, h := range hosts {
		if h.Enabled {
			enabled++
		}
	}
	liveRooms := make([]live.Stats, 0, len(rooms))
	for _, wr := range rooms {
		liveRooms = append(liveRooms, s.Live.Room(ctx, wr))
	}
	active, _ := s.Store.ActiveVersion(ctx)
	dirty, _ := s.Deployer.Dirty(ctx)
	expiring := 0
	for _, c := range certList {
		if c.Status == "valid" && time.Until(c.NotAfter) < 30*24*time.Hour {
			expiring++
		}
	}
	var tr any
	if s.Traffic != nil {
		sum := s.Traffic.Summary("15m", "", time.Now())
		tr = sum
	}
	gw := map[string]any{"running": false}
	if s.Gateway != nil {
		running, since := s.Gateway()
		gw["running"] = running
		if running {
			gw["uptime_s"] = int64(time.Since(since).Seconds())
		}
	}
	writeJSON(w, 200, map[string]any{
		"proxy_hosts": len(hosts), "proxy_hosts_enabled": enabled,
		"waitrooms": len(rooms), "certificates": len(certList), "certificates_expiring": expiring,
		"active_version": active, "dirty": dirty, "gateway": gw, "traffic": tr,
		"redis": s.Live.Ping(ctx), "rooms": liveRooms, "version": s.Version,
	})
}

// ------------------------------------------------------------------ proxy hosts

type proxyHostInput struct {
	Domains        []string `json:"domains"`
	ForwardScheme  string   `json:"forward_scheme"`
	ForwardHost    string   `json:"forward_host"`
	ForwardPort    int      `json:"forward_port"`
	Websocket      bool     `json:"websocket"`
	CertificateID  *int64   `json:"certificate_id"`
	SSLForced      bool     `json:"ssl_forced"`
	HTTP2          *bool    `json:"http2"`
	WaitroomID     *int64   `json:"waitroom_id"`
	WaitroomPath   string   `json:"waitroom_path"`
	AdvancedConfig string   `json:"advanced_config"`
	Enabled        *bool    `json:"enabled"`
	ClientIP       string   `json:"client_ip"`
	RateLimitID    *int64   `json:"rate_limit_id"`
	AccessListID   *int64   `json:"access_list_id"`
	BlockExploits  bool     `json:"block_exploits"`
	BodyInspection string   `json:"body_inspection"`
	BodyMaxKB      int      `json:"body_max_kb"`

	SecurityHeaders string `json:"security_headers"` // off | basic | strict
	// Under attack mode; omitted = unchanged (new hosts: off, default parameters).
	UnderAttack       *string                  `json:"under_attack"`
	UnderAttackConfig *store.UnderAttackConfig `json:"under_attack_config"`

	// BasicAuth edits the host's password protection; when present it replaces access_list_id.
	BasicAuth      *basicAuthInput `json:"basic_auth"`
	BasicAuthUsers []string        `json:"basic_auth_users"` // read-only; accepted and ignored
	// CertificateRequest orders a new ACME certificate for the host's domains
	// (NPM's "Request a new SSL Certificate"); it replaces certificate_id.
	CertificateRequest *certRequest `json:"certificate_request"`
}

// requestCertificate creates a pending ACME certificate for h inside tx.
func (s *Server) requestCertificate(r *http.Request, tx *store.Store, h *store.ProxyHost, req *certRequest) (*store.Certificate, error) {
	c, err := s.orderCertificate(r, tx, h.Domains, req)
	if err != nil {
		return nil, err
	}
	h.CertificateID = &c.ID
	return c, nil
}

// orderCertificate creates a pending ACME certificate for domains inside tx.
func (s *Server) orderCertificate(r *http.Request, tx *store.Store, hostDomains []string, req *certRequest) (*store.Certificate, error) {
	if s.ACME == nil {
		return nil, validate.Errors{{Field: "certificate_request", Code: "unavailable", Message: "ACME is not available"}}
	}
	info, domains, errs := s.buildACME(hostDomains, *req, "certificate_request.")
	if len(errs) > 0 {
		return nil, errs
	}
	c := &store.Certificate{Name: domains[0], Kind: "acme", Domains: domains, NotAfter: time.Now().UTC(), Status: "pending", AutoRenew: true, ACME: info}
	if err := tx.CreateCertificate(r.Context(), c); err != nil {
		return nil, err
	}
	c.Challenge = info.Challenge
	return c, nil
}

func (in proxyHostInput) apply(h *store.ProxyHost) {
	h.Domains, h.ForwardScheme, h.ForwardHost, h.ForwardPort = in.Domains, in.ForwardScheme, in.ForwardHost, in.ForwardPort
	h.Websocket, h.CertificateID, h.SSLForced = in.Websocket, in.CertificateID, in.SSLForced
	h.WaitroomID, h.WaitroomPath, h.AdvancedConfig = in.WaitroomID, in.WaitroomPath, in.AdvancedConfig
	h.HTTP2 = in.HTTP2 == nil || *in.HTTP2
	h.Enabled = in.Enabled == nil || *in.Enabled
	h.ClientIP, h.RateLimitID, h.AccessListID, h.BlockExploits = in.ClientIP, in.RateLimitID, in.AccessListID, in.BlockExploits
	h.BodyInspection, h.BodyMaxKB, h.SecurityHeaders = in.BodyInspection, in.BodyMaxKB, in.SecurityHeaders
	if in.UnderAttack != nil {
		h.UnderAttack = *in.UnderAttack
	}
	if in.UnderAttackConfig != nil {
		h.UnderAttackConfig = *in.UnderAttackConfig
	}
}

// checkProxyHost validates a host against the rest of the configuration (inside the tx).
func checkProxyHost(r *http.Request, tx *store.Store, h *store.ProxyHost) error {
	if err := validate.ProxyHost(h); err != nil {
		return err
	}
	ctx := r.Context()
	var errs validate.Errors
	if h.CertificateID != nil {
		// A pending/failed certificate is allowed: the host serves HTTP until it is issued.
		if _, err := tx.CertificateByID(ctx, *h.CertificateID); err != nil {
			errs = append(errs, validate.FieldError{Field: "certificate_id", Code: "not_found", Message: "certificate does not exist"})
		}
	}
	if h.WaitroomID != nil {
		if _, err := tx.WaitroomByID(ctx, *h.WaitroomID); err != nil {
			errs = append(errs, validate.FieldError{Field: "waitroom_id", Code: "not_found", Message: "access queue does not exist"})
		}
	}
	if h.RateLimitID != nil {
		if _, err := tx.RateLimitByID(ctx, *h.RateLimitID); err != nil {
			errs = append(errs, validate.FieldError{Field: "rate_limit_id", Code: "not_found", Message: "rate limit policy does not exist"})
		}
	}
	if h.AccessListID != nil {
		if _, err := tx.AccessListByID(ctx, *h.AccessListID); err != nil {
			errs = append(errs, validate.FieldError{Field: "access_list_id", Code: "not_found", Message: "access list does not exist"})
		}
	}
	if h.ClientIP == "trusted_proxies" {
		if n, err := tx.Network(ctx); err == nil && len(n.TrustedProxies) == 0 {
			errs = append(errs, validate.FieldError{Field: "client_ip", Code: "no_trusted_proxies", Message: "configure trusted proxies in network settings first"})
		}
	}
	dup, err := domainConflicts(ctx, tx, h.Domains, h.ID, 0)
	if err != nil {
		return err
	}
	return append(errs, dup...).OrNil()
}

// domainConflicts reports domains already used by another proxy host or redirection
// host (skipProxy / skipRedirect: the host being edited, 0 = none).
func domainConflicts(ctx context.Context, tx *store.Store, domains []string, skipProxy, skipRedirect int64) (validate.Errors, error) {
	mine := map[string]bool{}
	for _, d := range domains {
		mine[d] = true
	}
	var errs validate.Errors
	dup := func(d, kind string, id int64) {
		errs = append(errs, validate.FieldError{Field: "domains", Code: "duplicate", Message: fmt.Sprintf("%s is already used by %s #%d", d, kind, id)})
	}
	proxies, err := tx.ListProxyHosts(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range proxies {
		if o.ID == skipProxy {
			continue
		}
		for _, d := range o.Domains {
			if mine[d] {
				dup(d, "proxy host", o.ID)
			}
		}
	}
	redirects, err := tx.ListRedirectionHosts(ctx)
	if err != nil {
		return nil, err
	}
	for _, o := range redirects {
		if o.ID == skipRedirect {
			continue
		}
		for _, d := range o.Domains {
			if mine[d] {
				dup(d, "redirection host", o.ID)
			}
		}
	}
	return errs, nil
}

func (s *Server) listProxyHosts(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListProxyHosts(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	s.withAuthUsers(r.Context(), list)
	writeJSON(w, 200, list)
}

func (s *Server) getProxyHost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	h, err := s.Store.ProxyHostByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	one := []store.ProxyHost{*h}
	s.withAuthUsers(r.Context(), one)
	writeJSON(w, 200, one[0])
}

func (s *Server) createProxyHost(w http.ResponseWriter, r *http.Request) {
	var in proxyHostInput
	if !decode(w, r, &in) {
		return
	}
	h := &store.ProxyHost{}
	in.apply(h)
	var cert *store.Certificate
	if err := advancedAllowed(r, "", in.AdvancedConfig); err != nil {
		fail(w, err)
		return
	}
	v, err := s.Deployer.Apply(r.Context(), userID(r), "create proxy host "+strings.Join(in.Domains, ","), func(tx *store.Store) error {
		if in.CertificateRequest != nil {
			var err error
			if cert, err = s.requestCertificate(r, tx, h, in.CertificateRequest); err != nil {
				return err
			}
		}
		if in.BasicAuth != nil {
			h.AccessListID = nil
		}
		if err := checkProxyHost(r, tx, h); err != nil {
			return err
		}
		if err := tx.SaveProxyHost(r.Context(), h); err != nil {
			return err
		}
		if in.BasicAuth != nil {
			return applyBasicAuth(r.Context(), tx, h, nil, in.BasicAuth)
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	if cert != nil {
		s.ACME.Start(cert.ID)
	}
	s.audit(r, "proxy_host.create", fmt.Sprintf("proxy_host:%d", h.ID), h)
	s.underAttackChanged()
	writeJSON(w, 201, map[string]any{"proxy_host": h, "version": v, "certificate": cert})
}

func (s *Server) updateProxyHost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in proxyHostInput
	if !decode(w, r, &in) {
		return
	}
	var before, h *store.ProxyHost
	var cert *store.Certificate
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("update proxy host #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.ProxyHostByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		h = &cp
		in.apply(h)
		if err := advancedAllowed(r, before.AdvancedConfig, h.AdvancedConfig); err != nil {
			return err
		}
		if in.CertificateRequest != nil {
			if cert, err = s.requestCertificate(r, tx, h, in.CertificateRequest); err != nil {
				return err
			}
		}
		if in.BasicAuth != nil {
			h.AccessListID = before.AccessListID // replaced by applyBasicAuth below
		}
		if err := checkProxyHost(r, tx, h); err != nil {
			return err
		}
		if err := tx.SaveProxyHost(r.Context(), h); err != nil {
			return err
		}
		if in.BasicAuth != nil {
			return applyBasicAuth(r.Context(), tx, h, before, in.BasicAuth)
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	if cert != nil {
		s.ACME.Start(cert.ID)
	}
	s.audit(r, "proxy_host.update", fmt.Sprintf("proxy_host:%d", id), map[string]any{"before": before, "after": h})
	s.underAttackChanged()
	writeJSON(w, 200, map[string]any{"proxy_host": h, "version": v, "certificate": cert})
}

func (s *Server) deleteProxyHost(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.ProxyHost
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete proxy host #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.ProxyHostByID(r.Context(), id); err != nil {
			return err
		}
		if err := tx.DeleteProxyHost(r.Context(), id); err != nil {
			return err
		}
		if before.AccessListID != nil { // remove the host's own password list
			if l, err := tx.AccessListByID(r.Context(), *before.AccessListID); err == nil && l.Name == ownListName(id) {
				_ = tx.DeleteAccessList(r.Context(), l.ID)
			}
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "proxy_host.delete", fmt.Sprintf("proxy_host:%d", id), before)
	s.underAttackChanged()
	writeJSON(w, 200, map[string]any{"version": v})
}

// ------------------------------------------------------------------ access queues

type waitroomInput struct {
	Name      string `json:"name"`
	Enabled   *bool  `json:"enabled"`
	Capacity  int    `json:"capacity"`
	ActiveTTL int    `json:"active_ttl"`
	GrantTTL  int    `json:"grant_ttl"`
	QueueTTL  int    `json:"queue_ttl"`
	Refresh   int    `json:"refresh"`
	AdmitRate int    `json:"admit_rate"`
	FailMode  string `json:"fail_mode"`
	MaxQueue  *int   `json:"max_queue"`
	MaxNewIP  *int   `json:"max_new_per_ip"`
	BindIP    bool   `json:"bind_ip"`
}

func (in waitroomInput) apply(wr *store.Waitroom) {
	def := func(v, d int) int {
		if v == 0 {
			return d
		}
		return v
	}
	wr.Name = in.Name
	wr.Enabled = in.Enabled == nil || *in.Enabled
	wr.Capacity = in.Capacity
	wr.ActiveTTL = def(in.ActiveTTL, 120)
	wr.GrantTTL = def(in.GrantTTL, 30)
	wr.QueueTTL = def(in.QueueTTL, 120)
	wr.Refresh = def(in.Refresh, 15)
	wr.AdmitRate = in.AdmitRate
	wr.FailMode = in.FailMode
	if wr.FailMode == "" {
		wr.FailMode = "open"
	}
	wr.MaxQueue, wr.MaxNewIP, wr.BindIP = 100000, 60, in.BindIP
	if in.MaxQueue != nil {
		wr.MaxQueue = *in.MaxQueue
	}
	if in.MaxNewIP != nil {
		wr.MaxNewIP = *in.MaxNewIP
	}
}

type waitroomView struct {
	store.Waitroom
	Live live.Stats `json:"live"`
}

func (s *Server) listWaitrooms(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListWaitrooms(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	out := make([]waitroomView, 0, len(list))
	for _, wr := range list {
		out = append(out, waitroomView{Waitroom: wr, Live: s.Live.Room(r.Context(), wr)})
	}
	writeJSON(w, 200, out)
}

func (s *Server) waitroomLive(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	wr, err := s.Store.WaitroomByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, s.Live.Room(r.Context(), *wr))
}

func (s *Server) createWaitroom(w http.ResponseWriter, r *http.Request) {
	var in waitroomInput
	if !decode(w, r, &in) {
		return
	}
	wr := &store.Waitroom{}
	in.apply(wr)
	v, err := s.Deployer.Apply(r.Context(), userID(r), "create access queue "+in.Name, func(tx *store.Store) error {
		if err := validate.Waitroom(wr); err != nil {
			return err
		}
		list, err := tx.ListWaitrooms(r.Context())
		if err != nil {
			return err
		}
		for _, o := range list {
			if o.Name == wr.Name {
				return validate.Errors{{Field: "name", Code: "duplicate", Message: "a access queue with this name already exists"}}
			}
		}
		return tx.SaveWaitroom(r.Context(), wr)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "waitroom.create", "waitroom:"+wr.Name, wr)
	writeJSON(w, 201, map[string]any{"waitroom": wr, "version": v})
}

func (s *Server) updateWaitroom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in waitroomInput
	if !decode(w, r, &in) {
		return
	}
	var before, wr *store.Waitroom
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("update access queue #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.WaitroomByID(r.Context(), id); err != nil {
			return err
		}
		cp := *before
		wr = &cp
		in.apply(wr)
		// The name keys Redis state and visitor cookies; renaming would orphan the live queue.
		if wr.Name != before.Name {
			return validate.Errors{{Field: "name", Code: "immutable", Message: "a access queue cannot be renamed"}}
		}
		if err := validate.Waitroom(wr); err != nil {
			return err
		}
		return tx.SaveWaitroom(r.Context(), wr)
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "waitroom.update", "waitroom:"+wr.Name, map[string]any{"before": before, "after": wr})
	writeJSON(w, 200, map[string]any{"waitroom": wr, "version": v})
}

func (s *Server) deleteWaitroom(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var before *store.Waitroom
	v, err := s.Deployer.Apply(r.Context(), userID(r), fmt.Sprintf("delete access queue #%d", id), func(tx *store.Store) error {
		var err error
		if before, err = tx.WaitroomByID(r.Context(), id); err != nil {
			return err
		}
		if err := tx.DeleteWaitroom(r.Context(), id); err != nil && !errors.Is(err, store.ErrNotFound) {
			return conflictError{err.Error()}
		}
		return nil
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "waitroom.delete", "waitroom:"+before.Name, before)
	writeJSON(w, 200, map[string]any{"version": v})
}

// ------------------------------------------------------------------ certificates

func (s *Server) listCertificates(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListCertificates(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) uploadCertificate(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name        string `json:"name"`
		Certificate string `json:"certificate"`
		PrivateKey  string `json:"private_key"`
	}
	if !decode(w, r, &in) {
		return
	}
	info, err := certs.Parse([]byte(in.Certificate), []byte(in.PrivateKey))
	if err != nil {
		fail(w, validate.Errors{{Field: "certificate", Code: "invalid", Message: err.Error()}})
		return
	}
	name := strings.TrimSpace(in.Name)
	if name == "" && len(info.Domains) > 0 {
		name = info.Domains[0]
	}
	c := &store.Certificate{Name: name, Kind: "custom", Domains: info.Domains, NotAfter: info.NotAfter}
	err = s.Store.WithTx(r.Context(), func(tx *store.Store) error {
		if err := tx.CreateCertificate(r.Context(), c); err != nil {
			return err
		}
		return certs.Write(s.Paths.CertDir(c.ID), []byte(in.Certificate), []byte(in.PrivateKey))
	})
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "certificate.upload", fmt.Sprintf("certificate:%d", c.ID), map[string]any{"name": c.Name, "domains": c.Domains, "not_after": c.NotAfter})
	writeJSON(w, 201, c)
}

func (s *Server) deleteCertificate(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	c, err := s.Store.CertificateByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.DeleteCertificate(r.Context(), id); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			fail(w, err)
		} else {
			fail(w, conflictError{err.Error()})
		}
		return
	}
	// The files stay on disk because older config versions (rollback targets) may
	// still reference them. TODO: remove once no retained version uses them.
	s.audit(r, "certificate.delete", fmt.Sprintf("certificate:%d", id), c)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// ------------------------------------------------------------------ deployments

func (s *Server) listDeployments(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	if offset < 0 {
		offset = 0
	}
	list, err := s.Store.ListVersions(r.Context(), limit, offset)
	if err != nil {
		fail(w, err)
		return
	}
	total, _ := s.Store.CountVersions(r.Context())
	dirty, _ := s.Deployer.Dirty(r.Context())
	writeJSON(w, 200, map[string]any{"versions": list, "dirty": dirty, "total": total, "limit": limit, "offset": offset})
}

func (s *Server) redeploy(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Message string `json:"message"`
	}
	if r.ContentLength > 0 && !decode(w, r, &in) {
		return
	}
	msg := strings.TrimSpace(in.Message)
	if msg == "" {
		msg = "manual deployment"
	}
	v, err := s.Deployer.Apply(r.Context(), userID(r), msg, nil)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "deploy.apply", fmt.Sprintf("version:%d", v.ID), map[string]string{"message": msg})
	writeJSON(w, 201, v)
}

func (s *Server) rollback(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := s.Deployer.Rollback(r.Context(), userID(r), id)
	if err != nil {
		var terr *deploy.TestError
		if errors.As(err, &terr) || errors.Is(err, store.ErrNotFound) {
			fail(w, err)
		} else {
			fail(w, conflictError{err.Error()})
		}
		return
	}
	s.audit(r, "deploy.rollback", fmt.Sprintf("version:%d", id), map[string]any{"new_version": v.ID})
	writeJSON(w, 201, v)
}

// deploymentFiles returns the generated configuration of a version (read-only view).
func (s *Server) deploymentFiles(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	v, err := s.Store.VersionByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if v.Dir == "" {
		writeJSON(w, 200, map[string]any{"files": map[string]string{}})
		return
	}
	root := filepath.Join(s.Deployer.Dir, "versions", v.Dir)
	files := map[string]string{}
	_ = filepath.WalkDir(root, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		b, err := os.ReadFile(p)
		if err == nil && len(b) < 256<<10 {
			rel, _ := filepath.Rel(root, p)
			if strings.HasSuffix(rel, ".htpasswd") {
				b = []byte("# password hashes are not displayed\n") // F12
			}
			files[filepath.ToSlash(rel)] = string(b)
		}
		return nil
	})
	s.audit(r, "deploy.files_viewed", fmt.Sprintf("version:%d", id), nil)
	names := make([]string, 0, len(files))
	for n := range files {
		names = append(names, n)
	}
	sort.Strings(names)
	writeJSON(w, 200, map[string]any{"version": v, "names": names, "files": files})
}

// ------------------------------------------------------------------ audit

func (s *Server) listAudit(w http.ResponseWriter, r *http.Request) {
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	if limit <= 0 || limit > 500 {
		limit = 100
	}
	list, err := s.Store.ListAudit(r.Context(), limit)
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}
