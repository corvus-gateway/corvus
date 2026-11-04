// Package api is the Corvus admin REST API (JSON over HTTPS on the admin port).
package api

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"corvus/control/internal/alerts"
	"corvus/control/internal/auth"
	"corvus/control/internal/autoban"
	"corvus/control/internal/compiler"
	"corvus/control/internal/deploy"
	"corvus/control/internal/history"
	"corvus/control/internal/live"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
	"corvus/control/internal/validate"
)

type Server struct {
	Store        *store.Store
	Deployer     *deploy.Deployer
	Paths        compiler.Paths
	Live         *live.Live
	Setup        *auth.SetupToken
	Limiter      *auth.Limiter
	UI           http.Handler
	SecureCookie bool
	Version      string
	Traffic      *traffic.Collector
	Gateway      func() (running bool, since time.Time) // nil when running without a gateway
	ACME         ACMEService
	NPMImportDir string
	DataKey      []byte // AES-256 key for secrets stored in the database
	MFA          *auth.MFAChallenges
	AcctLimiter  *auth.Limiter // failed sign-ins per account (in addition to per IP)
	Alerts       *alerts.Engine
	GeoIP        GeoIPService // country database for security rules (nil = unavailable)
	AutoBan      *autoban.Service
	History      *history.Service
	Rules        RulesService
	allow        allowCache
	ssoRT        ssoRuntime // OIDC sign-in flows + provider cache (sso.go)
}

type ctxKey struct{}

func userFrom(r *http.Request) *store.User {
	u, _ := r.Context().Value(ctxKey{}).(*store.User)
	return u
}

func userID(r *http.Request) *int64 {
	if u := userFrom(r); u != nil {
		return &u.ID
	}
	return nil
}

// Handler builds the full HTTP handler (API + embedded UI).
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	pub := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, h) }
	priv := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireAuth(h)) }           // any signed-in user
	op := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireRole("operator", h)) } // change configuration
	adm := func(pattern string, h http.HandlerFunc) { mux.Handle(pattern, s.requireRole("admin", h)) }   // users & system settings

	pub("GET /api/status", s.handleStatus)
	pub("POST /api/setup", s.handleSetup)
	pub("POST /api/auth/login", s.handleLogin)
	pub("GET /api/auth/sso", s.publicSSO) // SSO (OIDC), sso.go
	pub("GET /api/auth/oidc/start", s.oidcStart)
	pub("GET /api/auth/oidc/callback", s.oidcCallback)
	adm("GET /api/settings/sso", s.getSSOSettings)
	adm("PUT /api/settings/sso", s.putSSOSettings)
	adm("POST /api/settings/sso/test", s.testSSODiscovery)
	priv("POST /api/auth/logout", s.handleLogout)
	priv("GET /api/auth/me", s.handleMe)
	priv("POST /api/auth/password", s.handlePassword)

	priv("GET /api/overview", s.handleOverview)

	priv("GET /api/proxy-hosts", s.listProxyHosts)
	op("POST /api/proxy-hosts", s.createProxyHost)
	priv("GET /api/proxy-hosts/{id}", s.getProxyHost)
	op("PUT /api/proxy-hosts/{id}", s.updateProxyHost)
	op("DELETE /api/proxy-hosts/{id}", s.deleteProxyHost)

	priv("GET /api/redirection-hosts", s.listRedirectionHosts)
	op("POST /api/redirection-hosts", s.createRedirectionHost)
	priv("GET /api/redirection-hosts/{id}", s.getRedirectionHost)
	op("PUT /api/redirection-hosts/{id}", s.updateRedirectionHost)
	op("DELETE /api/redirection-hosts/{id}", s.deleteRedirectionHost)

	priv("GET /api/waitrooms", s.listWaitrooms)
	op("POST /api/waitrooms", s.createWaitroom)
	op("PUT /api/waitrooms/{id}", s.updateWaitroom)
	op("DELETE /api/waitrooms/{id}", s.deleteWaitroom)
	priv("GET /api/waitrooms/{id}/live", s.waitroomLive)

	priv("GET /api/certificates", s.listCertificates)
	op("POST /api/certificates", s.uploadCertificate)
	op("DELETE /api/certificates/{id}", s.deleteCertificate)

	priv("GET /api/deployments", s.listDeployments)
	op("POST /api/deployments", s.redeploy)
	priv("GET /api/deployments/{id}/files", s.deploymentFiles)
	op("POST /api/deployments/{id}/rollback", s.rollback)

	priv("GET /api/audit", s.listAudit)

	adm("GET /api/users", s.listUsers)
	adm("POST /api/users", s.createUser)
	adm("PUT /api/users/{id}", s.updateUser)
	adm("DELETE /api/users/{id}", s.deleteUser)
	adm("POST /api/users/{id}/reset-mfa", s.resetUserMFA)

	priv("GET /api/rate-limits", s.listRateLimits)
	op("POST /api/rate-limits", s.createRateLimit)
	op("PUT /api/rate-limits/{id}", s.updateRateLimit)
	op("DELETE /api/rate-limits/{id}", s.deleteRateLimit)

	priv("GET /api/security-rules", s.listSecurityRules)
	op("POST /api/security-rules", s.createSecurityRule)
	op("PUT /api/security-rules/order", s.reorderSecurityRules)
	op("PUT /api/security-rules/{id}", s.updateSecurityRule)
	op("DELETE /api/security-rules/{id}", s.deleteSecurityRule)
	priv("GET /api/security-rules/events", s.securityRuleEvents)
	priv("GET /api/security-rules/geoip", s.geoipStatus)
	adm("POST /api/security-rules/geoip/refresh", s.geoipRefresh)
	priv("GET /api/rule-templates", s.listTemplates)
	op("POST /api/security-rules/from-template", s.ruleFromTemplate)
	priv("GET /api/managed-rules", s.managedRules)
	adm("PUT /api/settings/managed-rules", s.putManagedRules)
	adm("POST /api/managed-rules/check", s.checkManagedRules)
	adm("POST /api/managed-rules/upload", s.uploadManagedRules)
	adm("GET /api/settings/retention", s.getRetention)
	adm("PUT /api/settings/retention", s.putRetention)
	op("GET /api/traffic/archives", s.listArchives)
	op("GET /api/traffic/archives/{name}", s.downloadArchive)
	priv("GET /api/bans", s.listBans)
	op("POST /api/bans", s.createBan)
	op("DELETE /api/bans/{id}", s.deleteBan)
	op("POST /api/bans/{id}/permanent", s.permanentBan)
	priv("GET /api/settings/autoban", s.getAutoBan)
	op("PUT /api/settings/autoban", s.putAutoBan)
	priv("GET /api/ip-lists", s.listIPLists)
	op("POST /api/ip-lists", s.createIPList)
	op("PUT /api/ip-lists/{id}", s.updateIPList)
	op("DELETE /api/ip-lists/{id}", s.deleteIPList)

	priv("GET /api/access-lists", s.listAccessLists)
	op("POST /api/access-lists", s.createAccessList)
	op("PUT /api/access-lists/{id}", s.updateAccessList)
	op("DELETE /api/access-lists/{id}", s.deleteAccessList)

	priv("GET /api/settings/network", s.getNetwork)
	adm("PUT /api/settings/network", s.putNetwork)
	adm("POST /api/settings/network/cloudflare/refresh", s.refreshCloudflare)

	priv("GET /api/traffic/summary", s.trafficSummary)
	priv("GET /api/traffic/timeseries", s.trafficTimeseries)
	priv("GET /api/traffic/hosts", s.trafficHosts)
	priv("GET /api/traffic/top-paths", s.trafficTopPaths)
	priv("GET /api/traffic/logs", s.trafficLogs)
	priv("GET /api/traffic/events", s.trafficEvents)

	op("POST /api/certificates/acme", s.issueACME)
	priv("GET /api/acme/dns-providers", s.dnsProviders)
	op("POST /api/certificates/{id}/renew", s.renewACME)

	priv("POST /api/auth/mfa/setup", s.mfaSetup)
	priv("POST /api/auth/mfa/enable", s.mfaEnable)
	priv("POST /api/auth/mfa/disable", s.mfaDisable)
	pub("POST /api/auth/login/mfa", s.handleLoginMFA)

	adm("POST /api/import/npm/preview", s.npmPreview)
	adm("POST /api/import/npm/apply", s.npmApply)

	priv("GET /preview/wait/{room}", s.previewWaitPage)

	priv("GET /api/alerts/channels", s.listChannels)
	op("POST /api/alerts/channels", s.createChannel)
	op("PUT /api/alerts/channels/{id}", s.updateChannel)
	op("DELETE /api/alerts/channels/{id}", s.deleteChannel)
	op("POST /api/alerts/channels/{id}/test", s.testChannel)
	priv("GET /api/alerts/rules", s.getAlertRules)
	op("PUT /api/alerts/rules", s.putAlertRules)
	priv("GET /api/alerts/history", s.alertHistory)
	priv("GET /api/alerts/active", s.alertActive)

	// API tokens (managed with a real admin session only) and Prometheus metrics (Bearer token).
	mux.Handle("GET /api/tokens", s.admSession(s.listTokens))
	mux.Handle("POST /api/tokens", s.admSession(s.createToken))
	mux.Handle("DELETE /api/tokens/{id}", s.admSession(s.deleteToken))
	mux.Handle("GET /metrics", http.HandlerFunc(s.handleMetrics))

	// Backup & restore (contains secrets: admin with a real session only).
	mux.Handle("GET /api/backup", s.admSession(s.downloadBackup))
	mux.Handle("GET /api/backup/restore", s.admSession(s.getRestore))
	mux.Handle("POST /api/backup/restore", s.admSession(s.uploadRestore))
	mux.Handle("POST /api/backup/restore/confirm", s.admSession(s.confirmRestore))
	mux.Handle("DELETE /api/backup/restore", s.admSession(s.discardRestore))

	mux.Handle("/api/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		writeErr(w, http.StatusNotFound, "not_found", "unknown API endpoint")
	}))
	if s.UI != nil {
		mux.Handle("/", s.UI)
	}
	return securityHeaders(s.adminAllow(s.csrf(mux)))
}

// ------------------------------------------------------------------ middleware

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("X-Frame-Options", "DENY")
		h.Set("Referrer-Policy", "same-origin")
		h.Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; frame-ancestors 'none'; base-uri 'none'; form-action 'self'")
		if strings.HasPrefix(r.URL.Path, "/api/") {
			h.Set("Cache-Control", "no-store")
		}
		next.ServeHTTP(w, r)
	})
}

// csrf: state-changing API calls must carry X-Corvus-Request (not settable cross-site
// without CORS, which we never grant) and, when present, a same-origin Origin header.
func (s *Server) csrf(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, bearer := bearerToken(r) // Bearer requests are not cookie-authenticated (see tokens.go)
		if strings.HasPrefix(r.URL.Path, "/api/") && r.Method != http.MethodGet && r.Method != http.MethodHead && !bearer {
			if r.Header.Get("X-Corvus-Request") != "1" {
				writeErr(w, http.StatusForbidden, "csrf", "missing X-Corvus-Request header")
				return
			}
			if o := r.Header.Get("Origin"); o != "" {
				if u, err := url.Parse(o); err != nil || u.Host != r.Host {
					writeErr(w, http.StatusForbidden, "csrf", "cross-origin request rejected")
					return
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) requireAuth(next http.HandlerFunc) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if tok, ok := bearerToken(r); ok { // API token: exclusive of the session cookie
			s.authBearer(w, r, tok, next)
			return
		}
		c, err := r.Cookie(auth.SessionCookie)
		if err != nil || c.Value == "" {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "login required")
			return
		}
		u, err := s.Store.SessionUser(r.Context(), auth.TokenHash(c.Value))
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "unauthorized", "session expired")
			return
		}
		next(w, r.WithContext(context.WithValue(r.Context(), ctxKey{}, u)))
	})
}

// ------------------------------------------------------------------ helpers

func clientIP(r *http.Request) string {
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return host
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, kind, msg string) {
	writeJSON(w, code, map[string]any{"error": kind, "message": msg})
}

func decode(w http.ResponseWriter, r *http.Request, v any) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	dec := json.NewDecoder(r.Body)
	dec.DisallowUnknownFields()
	if err := dec.Decode(v); err != nil {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid JSON body: "+err.Error())
		return false
	}
	return true
}

func pathID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil || id <= 0 {
		writeErr(w, http.StatusBadRequest, "bad_request", "invalid id")
		return 0, false
	}
	return id, true
}

// fail maps domain errors to HTTP responses.
func fail(w http.ResponseWriter, err error) {
	var verr validate.Errors
	var terr *deploy.TestError
	var cerr conflictError
	switch {
	case errors.As(err, &verr):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "validation", "message": verr.Error(), "fields": verr})
	case errors.As(err, &terr):
		writeJSON(w, http.StatusUnprocessableEntity, map[string]any{"error": "config_rejected",
			"message": "the gateway rejected the generated configuration; nothing was changed", "output": terr.Output, "version_id": terr.VersionID})
	case errors.As(err, &cerr):
		writeErr(w, http.StatusConflict, "conflict", cerr.Error())
	case errors.Is(err, store.ErrNotFound):
		writeErr(w, http.StatusNotFound, "not_found", "not found")
	default:
		slog.Error("api error", "err", err)
		writeErr(w, http.StatusInternalServerError, "internal", "internal error")
	}
}

type conflictError struct{ msg string }

func (e conflictError) Error() string { return e.msg }

func (s *Server) audit(r *http.Request, action, target string, detail any) {
	if err := s.Store.Audit(r.Context(), userID(r), action, target, auditDetail(r, detail), clientIP(r)); err != nil {
		slog.Error("audit write failed", "err", err)
	}
}

// ------------------------------------------------------------------ status & auth

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request) {
	n, err := s.Store.CountUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, map[string]any{"setup_required": n == 0, "version": s.Version})
}

func (s *Server) setSession(w http.ResponseWriter, r *http.Request, u *store.User) error {
	token := auth.RandomToken(32)
	if err := s.Store.CreateSession(r.Context(), auth.TokenHash(token), u.ID, auth.SessionTTL, clientIP(r), r.UserAgent()); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Value: token, Path: "/", HttpOnly: true,
		Secure: s.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: int(auth.SessionTTL.Seconds())})
	return nil
}

func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Token    string `json:"token"`
		Email    string `json:"email"`
		Name     string `json:"name"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip := clientIP(r)
	if blocked, wait := s.Limiter.Blocked(ip); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	n, err := s.Store.CountUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	if n > 0 {
		writeErr(w, http.StatusConflict, "already_setup", "an administrator already exists")
		return
	}
	in.Email = strings.TrimSpace(in.Email)
	var verr validate.Errors
	if !strings.Contains(in.Email, "@") || len(in.Email) > 254 {
		verr = append(verr, validate.FieldError{Field: "email", Code: "invalid", Message: "invalid email"})
	}
	if strings.TrimSpace(in.Name) == "" {
		in.Name = strings.SplitN(in.Email, "@", 2)[0]
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		verr = append(verr, validate.FieldError{Field: "password", Code: "weak", Message: err.Error()})
	}
	if len(verr) > 0 {
		fail(w, verr)
		return
	}
	if !s.Setup.Consume(strings.TrimSpace(in.Token)) {
		s.Limiter.Fail(ip)
		writeErr(w, http.StatusForbidden, "invalid_token", "invalid setup token (see the corvus container logs)")
		return
	}
	u, err := s.Store.CreateUser(r.Context(), in.Email, strings.TrimSpace(in.Name), hash, "admin")
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.setSession(w, r, u); err != nil {
		fail(w, err)
		return
	}
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
	s.audit(r, "setup.admin_created", u.Email, nil)
	writeJSON(w, 201, u)
}

func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Email    string `json:"email"`
		Password string `json:"password"`
	}
	if !decode(w, r, &in) {
		return
	}
	ip := clientIP(r)
	if blocked, wait := s.Limiter.Blocked(ip); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many failed logins, try again later")
		return
	}
	acct := "acct:" + strings.ToLower(strings.TrimSpace(in.Email))
	if blocked, wait := s.AcctLimiter.Blocked(acct); blocked {
		w.Header().Set("Retry-After", strconv.Itoa(int(wait.Seconds())+1))
		s.audit(r, "auth.login_locked", strings.ToLower(strings.TrimSpace(in.Email)), nil)
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "this account is temporarily locked after repeated failures")
		return
	}
	u, err := s.Store.UserByEmail(r.Context(), strings.TrimSpace(in.Email))
	hash := ""
	if err == nil && !u.Disabled {
		hash = u.PasswordHash()
	}
	if !auth.CheckPasswordOrDummy(hash, in.Password) {
		s.Limiter.Fail(ip)
		s.AcctLimiter.Fail(acct)
		s.audit(r, "auth.login_failed", strings.ToLower(strings.TrimSpace(in.Email)), nil)
		writeErr(w, http.StatusUnauthorized, "invalid_credentials", "invalid email or password")
		return
	}
	if u.MFAEnabled {
		// Password verified; the session is only issued after the second factor.
		writeJSON(w, 200, map[string]any{"mfa_required": true, "mfa_token": s.MFA.Start(u.ID)})
		return
	}
	s.Limiter.Reset(ip)
	if err := s.setSession(w, r, u); err != nil {
		fail(w, err)
		return
	}
	_ = s.Store.TouchLogin(r.Context(), u.ID)
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
	s.audit(r, "auth.login", u.Email, nil)
	writeJSON(w, 200, u)
}

func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	if c, err := r.Cookie(auth.SessionCookie); err == nil {
		_ = s.Store.DeleteSession(r.Context(), auth.TokenHash(c.Value))
	}
	http.SetCookie(w, &http.Cookie{Name: auth.SessionCookie, Value: "", Path: "/", HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: -1})
	s.audit(r, "auth.logout", userFrom(r).Email, nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) handleMe(w http.ResponseWriter, r *http.Request) { writeJSON(w, 200, userFrom(r)) }

func (s *Server) handlePassword(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Current string `json:"current"`
		New     string `json:"new"`
	}
	if !decode(w, r, &in) {
		return
	}
	u := userFrom(r)
	if !auth.CheckPassword(u.PasswordHash(), in.Current) {
		s.audit(r, "auth.password_change_failed", u.Email, nil)
		writeErr(w, http.StatusForbidden, "invalid_credentials", "current password is incorrect")
		return
	}
	hash, err := auth.HashPassword(in.New)
	if err != nil {
		fail(w, validate.Errors{{Field: "new", Code: "weak", Message: err.Error()}})
		return
	}
	if err := s.Store.UpdatePassword(r.Context(), u.ID, hash); err != nil {
		fail(w, err)
		return
	}
	_ = s.Store.DeleteUserSessions(r.Context(), u.ID) // sign out everywhere else
	if err := s.setSession(w, r, u); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "auth.password_changed", u.Email, nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// PurgeLoop removes expired sessions periodically.
func (s *Server) PurgeLoop(ctx context.Context) {
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			_ = s.Store.PurgeExpiredSessions(ctx)
		}
	}
}

var roomNameRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,31}$`)

// previewWaitPage serves the visitor waiting page in preview mode (sample
// numbers, no polling) so operators can see it without a real queue.
func (s *Server) previewWaitPage(w http.ResponseWriter, r *http.Request) {
	if !roomNameRe.MatchString(r.PathValue("room")) {
		http.NotFound(w, r)
		return
	}
	if r.URL.Query().Get("preview") != "1" {
		q := r.URL.Query()
		q.Set("preview", "1")
		http.Redirect(w, r, r.URL.Path+"?"+q.Encode(), http.StatusFound)
		return
	}
	page, err := os.ReadFile(filepath.Join(s.Paths.HTMLDir, "wait.html"))
	if err != nil {
		http.Error(w, "waiting page not found", http.StatusNotFound)
		return
	}
	// The page is self-contained (inline style/script); relax CSP for this route only.
	w.Header().Set("Content-Security-Policy", "default-src 'none'; style-src 'unsafe-inline'; script-src 'unsafe-inline'; img-src data:; frame-ancestors 'self'; base-uri 'none'")
	w.Header().Set("X-Frame-Options", "SAMEORIGIN")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = w.Write(page)
}
