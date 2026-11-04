package api

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// API tokens: long-lived Bearer credentials for automation and Prometheus.
//
//   - format "cvs_" + 43 base62 characters (≈256 bits); only SHA-256(token) is stored
//   - a request with "Authorization: Bearer cvs_…" runs as a synthetic principal:
//     the creator's user id (audit attribution) with role = min(token role, creator's
//     current role); a disabled or deleted creator invalidates the token
//   - Bearer requests are exempt from the CSRF header (browsers never attach them
//     automatically); cookie requests keep the check
//   - tokens can never reach session-only endpoints: sign-in/password/MFA, users and
//     token management (a leaked token cannot mint more tokens or escalate), and
//     backup/restore (the archive contains the secrets)

const tokenPrefix = "cvs_"

type tokenCtxKey struct{}

func tokenFrom(r *http.Request) *store.APIToken {
	t, _ := r.Context().Value(tokenCtxKey{}).(*store.APIToken)
	return t
}

// bearerToken returns the Bearer credential of r, if any.
func bearerToken(r *http.Request) (string, bool) {
	h := r.Header.Get("Authorization")
	if len(h) < 7 || !strings.EqualFold(h[:7], "Bearer ") {
		return "", false
	}
	return strings.TrimSpace(h[7:]), true
}

const base62 = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

// newAPIToken returns "cvs_" + 32 random bytes encoded in base62 (fixed 43 chars).
func newAPIToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	n := new(big.Int).SetBytes(b)
	out := make([]byte, 43)
	base := big.NewInt(62)
	m := new(big.Int)
	for i := len(out) - 1; i >= 0; i-- {
		n.DivMod(n, base, m)
		out[i] = base62[m.Int64()]
	}
	return tokenPrefix + string(out)
}

// sessionOnly lists the endpoints a Bearer token may never call.
func sessionOnly(r *http.Request) bool {
	p := r.URL.Path
	if p == "/api/auth/me" && r.Method == http.MethodGet {
		return false
	}
	return strings.HasPrefix(p, "/api/auth/") || p == "/api/users" || strings.HasPrefix(p, "/api/users/") ||
		p == "/api/tokens" || strings.HasPrefix(p, "/api/tokens/") ||
		p == "/api/backup" || strings.HasPrefix(p, "/api/backup/")
}

// tokenTouched limits last_used_at writes to once a minute per token.
var tokenTouched sync.Map // token id → time.Time

func (s *Server) touchToken(ctx context.Context, id int64) {
	now := time.Now()
	if v, ok := tokenTouched.Load(id); ok && now.Sub(v.(time.Time)) < time.Minute {
		return
	}
	tokenTouched.Store(id, now)
	_ = s.Store.TouchAPIToken(ctx, id)
}

// bearerPrincipal resolves a Bearer token to the synthetic user the request runs as.
func (s *Server) bearerPrincipal(ctx context.Context, tok string) (*store.User, *store.APIToken, string) {
	if !strings.HasPrefix(tok, tokenPrefix) || len(tok) > 128 {
		return nil, nil, "invalid token"
	}
	t, err := s.Store.APITokenByHash(ctx, auth.TokenHash(tok))
	if err != nil {
		return nil, nil, "invalid token"
	}
	if t.Expired(time.Now()) {
		return nil, nil, "token expired"
	}
	creator, err := s.Store.UserByID(ctx, t.CreatedBy)
	if err != nil || creator.Disabled {
		return nil, nil, "token owner is disabled"
	}
	role := t.Role
	if !store.RoleAtLeast(creator.Role, role) {
		role = creator.Role // never more than the owner currently has
	}
	s.touchToken(ctx, t.ID)
	return &store.User{ID: creator.ID, Email: "token:" + t.Name, Name: t.Name, Role: role, CreatedAt: t.CreatedAt}, t, ""
}

// authBearer authenticates r via its Bearer token (used by requireAuth).
func (s *Server) authBearer(w http.ResponseWriter, r *http.Request, tok string, next http.HandlerFunc) {
	u, t, msg := s.bearerPrincipal(r.Context(), tok)
	if u == nil {
		w.Header().Set("WWW-Authenticate", `Bearer realm="corvus"`)
		writeErr(w, http.StatusUnauthorized, "unauthorized", msg)
		return
	}
	ctx := context.WithValue(context.WithValue(r.Context(), ctxKey{}, u), tokenCtxKey{}, t)
	r = r.WithContext(ctx)
	if sessionOnly(r) {
		writeErr(w, http.StatusForbidden, "session_required", "this endpoint requires a signed-in session; API tokens cannot use it")
		return
	}
	next(w, r)
}

// auditDetail adds the token name ("via": "token:<name>") to audit details of token requests.
func auditDetail(r *http.Request, detail any) any {
	t := tokenFrom(r)
	if t == nil {
		return detail
	}
	via := "token:" + t.Name
	m := map[string]any{}
	if detail != nil {
		b, _ := json.Marshal(detail)
		if json.Unmarshal(b, &m) != nil || m == nil {
			m = map[string]any{"detail": detail}
		}
	}
	m["via"] = via
	return m
}

// ------------------------------------------------------------------ handlers

// admSession: admin with a real session (no Bearer tokens).
func (s *Server) admSession(h http.HandlerFunc) http.Handler {
	return s.requireRole("admin", func(w http.ResponseWriter, r *http.Request) {
		if tokenFrom(r) != nil {
			writeErr(w, http.StatusForbidden, "session_required", "this endpoint requires a signed-in session")
			return
		}
		h(w, r)
	})
}

func (s *Server) listTokens(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListAPITokens(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

type tokenInput struct {
	Name        string `json:"name"`
	Role        string `json:"role"`
	ExpiresDays int    `json:"expires_days"`
}

func (s *Server) createToken(w http.ResponseWriter, r *http.Request) {
	var in tokenInput
	if !decode(w, r, &in) {
		return
	}
	me := userFrom(r)
	var errs validate.Errors
	in.Name = strings.TrimSpace(in.Name)
	if in.Name == "" || len(in.Name) > 64 || strings.ContainsAny(in.Name, "\r\n\t\"\\") {
		errs = append(errs, validate.FieldError{Field: "name", Code: "invalid", Message: "name is required (at most 64 characters, no quotes or control characters)"})
	}
	if !store.ValidRole(in.Role) {
		errs = append(errs, validate.FieldError{Field: "role", Code: "invalid", Message: "role must be admin, operator or viewer"})
	} else if !store.RoleAtLeast(me.Role, in.Role) {
		errs = append(errs, validate.FieldError{Field: "role", Code: "forbidden", Message: "a token cannot have more privileges than its creator"})
	}
	if in.ExpiresDays < 0 || in.ExpiresDays > 3650 {
		errs = append(errs, validate.FieldError{Field: "expires_days", Code: "invalid", Message: "expires_days must be between 0 (never) and 3650"})
	}
	if len(errs) > 0 {
		fail(w, errs)
		return
	}
	var exp *time.Time
	if in.ExpiresDays > 0 {
		t := time.Now().UTC().Add(time.Duration(in.ExpiresDays) * 24 * time.Hour)
		exp = &t
	}
	plain := newAPIToken()
	t, err := s.Store.CreateAPIToken(r.Context(), in.Name, plain[:8], auth.TokenHash(plain), in.Role, me.ID, exp)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "token.create", t.Name, map[string]any{"role": t.Role, "prefix": t.Prefix, "expires_days": in.ExpiresDays})
	writeJSON(w, 201, struct {
		*store.APIToken
		Token string `json:"token"`
	}{t, plain})
}

func (s *Server) deleteToken(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	t, err := s.Store.APITokenByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.DeleteAPIToken(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	tokenTouched.Delete(id)
	s.audit(r, "token.delete", t.Name, map[string]string{"prefix": t.Prefix, "role": t.Role})
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
