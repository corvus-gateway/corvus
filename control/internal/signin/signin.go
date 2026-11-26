// Package signin checks the credentials entered on the login page in front of proxy hosts
// (#294). The gateway never sees password hashes or TOTP secrets: it forwards the login
// form to this service over a Unix socket that only the gateway can reach, and signs its
// own session cookie when the answer is "ok".
package signin

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"strings"
	"sync"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"corvus/control/internal/auth"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// Results of a sign-in attempt.
const (
	ResultOK          = "ok"          // signed in: the gateway issues a session
	ResultFail        = "fail"        // wrong name, password, PIN or code (never says which)
	ResultTOTP        = "totp"        // password accepted, ask for the authenticator code
	ResultEnroll      = "enroll"      // password accepted, two-step required but not set up: show the QR
	ResultLimited     = "limited"     // too many failures from this client or for this user
	ResultUnavailable = "unavailable" // the host has no login page (any more)
)

const (
	stepTTL      = 5 * time.Minute
	stepAttempts = 5
	maxSteps     = 10000
	issuer       = "Corvus"
)

// Request is what the gateway sends for one form submission. Client is the visitor's
// address as the gateway sees it (IPv4, or the IPv6 /64), used for rate limiting.
type Request struct {
	HostID   int64  `json:"host_id"`
	Host     string `json:"host"` // the host name the visitor used (shown in the authenticator app)
	Client   string `json:"client"`
	Username string `json:"username"`
	Secret   string `json:"secret"` // password or PIN
	Step     string `json:"step"`   // second stage: the token from a "totp"/"enroll" answer
	Code     string `json:"code"`   // second stage: the authenticator code
}

// Response is the answer to the gateway.
type Response struct {
	Result     string `json:"result"`
	UserID     int64  `json:"user_id,omitempty"` // 0 for PIN sign-ins
	User       string `json:"user,omitempty"`
	Version    int    `json:"version,omitempty"` // session version to put into the cookie
	Hours      int    `json:"hours,omitempty"`   // session lifetime
	PassUser   bool   `json:"pass_user,omitempty"`
	Step       string `json:"step,omitempty"`
	QRPNG      string `json:"qr_png,omitempty"`      // enroll: data: URL of the otpauth QR code
	KeyText    string `json:"key,omitempty"`         // enroll: the secret for manual entry
	RetryAfter int    `json:"retry_after,omitempty"` // limited: seconds
	// Event is what happened, for the access log and statistics (gateway side).
	Event string `json:"event"`
}

// Store is the part of the store the service needs.
type Store interface {
	ProxyHostByID(ctx context.Context, id int64) (*store.ProxyHost, error)
	SigninUserByName(ctx context.Context, name string) (*store.SigninUser, error)
	SigninUserByID(ctx context.Context, id int64) (*store.SigninUser, error)
	SaveSigninUser(ctx context.Context, u *store.SigninUser) error
	TouchSigninUser(ctx context.Context, id int64) error
	ClaimSigninTOTPStep(ctx context.Context, userID, step int64) (bool, error)
}

// Service answers sign-in attempts.
type Service struct {
	Store   Store
	DataKey []byte
	IP      *auth.Limiter // failures per client address
	Account *auth.Limiter // failures per user name
	Now     func() time.Time

	mu    sync.Mutex
	steps map[string]*pending
}

type pending struct {
	hostID   int64
	userID   int64
	enroll   string // the new TOTP secret while enrolling (not stored yet)
	expires  time.Time
	attempts int
}

func (s *Service) now() time.Time {
	if s.Now != nil {
		return s.Now()
	}
	return time.Now()
}

// Handler serves POST /v1/signin (JSON). It must only be reachable by the gateway.
func (s *Service) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /v1/signin", func(w http.ResponseWriter, r *http.Request) {
		var in Request
		dec := json.NewDecoder(http.MaxBytesReader(w, r.Body, 8<<10))
		if err := dec.Decode(&in); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		out, err := s.SignIn(r.Context(), in)
		if err != nil {
			slog.Error("sign-in", "host", in.HostID, "err", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(out)
	})
	return mux
}

func limitedFor(d time.Duration) Response {
	return Response{Result: ResultLimited, RetryAfter: int(math.Ceil(d.Seconds())), Event: "login_limited"}
}

// SignIn checks one submission of the login form.
func (s *Service) SignIn(ctx context.Context, in Request) (Response, error) {
	h, err := s.Store.ProxyHostByID(ctx, in.HostID)
	if errors.Is(err, store.ErrNotFound) || (err == nil && !h.Login.Enabled) {
		return Response{Result: ResultUnavailable, Event: "login_unavailable"}, nil
	}
	if err != nil {
		return Response{}, err
	}
	ipKey := "ip:" + in.Client
	if blocked, d := s.IP.Blocked(ipKey); blocked {
		return limitedFor(d), nil
	}
	if in.Step != "" {
		return s.secondFactor(ctx, h, in, ipKey)
	}
	if h.Login.Mode == store.LoginPIN {
		if h.Login.PINHash == "" || !auth.CheckPassword(h.Login.PINHash, in.Secret) {
			s.IP.Fail(ipKey)
			return Response{Result: ResultFail, Event: "login_failed"}, nil
		}
		s.IP.Reset(ipKey)
		return Response{Result: ResultOK, Version: h.Login.PINVersion, Hours: h.Login.SessionHours, Event: "login_succeeded"}, nil
	}

	name := strings.ToLower(strings.TrimSpace(in.Username))
	acctKey := "user:" + name
	if blocked, d := s.Account.Blocked(acctKey); blocked {
		s.IP.Fail(ipKey) // probing a locked account still counts against the client
		return limitedFor(d), nil
	}
	u, err := s.Store.SigninUserByName(ctx, name)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return Response{}, err
	}
	hash := ""
	if u != nil && u.Enabled && allowed(h, u) {
		hash = u.PasswordHash
	}
	if !auth.CheckPasswordOrDummy(hash, in.Secret) {
		s.IP.Fail(ipKey)
		if name != "" {
			s.Account.Fail(acctKey)
		}
		return Response{Result: ResultFail, Event: "login_failed"}, nil
	}
	if !u.TOTPRequired {
		return s.success(ctx, h, u, ipKey, acctKey)
	}
	p := &pending{hostID: h.ID, userID: u.ID, expires: s.now().Add(stepTTL)}
	if u.TOTPSecret == "" {
		p.enroll = auth.NewTOTPSecret()
		uri := auth.TOTPURL(issuer, u.Username+"@"+hostLabel(in.Host, h), p.enroll)
		png, err := qrcode.Encode(uri, qrcode.Medium, 256)
		if err != nil {
			return Response{}, err
		}
		tok := s.put(p)
		return Response{Result: ResultEnroll, Step: tok, KeyText: p.enroll, Event: "login_enroll",
			QRPNG: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)}, nil
	}
	return Response{Result: ResultTOTP, Step: s.put(p), Event: "login_totp"}, nil
}

func (s *Service) secondFactor(ctx context.Context, h *store.ProxyHost, in Request, ipKey string) (Response, error) {
	p := s.get(in.Step)
	if p == nil || p.hostID != h.ID {
		s.IP.Fail(ipKey)
		return Response{Result: ResultFail, Event: "login_failed"}, nil
	}
	u, err := s.Store.SigninUserByID(ctx, p.userID)
	if errors.Is(err, store.ErrNotFound) {
		s.drop(in.Step)
		return Response{Result: ResultFail, Event: "login_failed"}, nil
	}
	if err != nil {
		return Response{}, err
	}
	acctKey := "user:" + u.Username
	if !u.Enabled || !allowed(h, u) {
		s.drop(in.Step)
		return Response{Result: ResultFail, Event: "login_failed"}, nil
	}
	secret := p.enroll
	if secret == "" {
		if secret, err = seal.Open(s.DataKey, u.TOTPSecret); err != nil {
			return Response{}, err
		}
	}
	step := auth.TOTPStep(secret, strings.TrimSpace(in.Code), s.now())
	ok := step > 0
	if ok {
		if ok, err = s.Store.ClaimSigninTOTPStep(ctx, u.ID, step); err != nil {
			return Response{}, err
		}
	}
	if !ok {
		s.IP.Fail(ipKey)
		s.Account.Fail(acctKey)
		if !s.attempt(in.Step) {
			return Response{Result: ResultFail, Event: "login_failed"}, nil
		}
		// keep the step: the visitor may retype the code
		r := Response{Result: ResultTOTP, Step: in.Step, Event: "login_failed"}
		if p.enroll != "" {
			r.Result, r.KeyText = ResultEnroll, p.enroll
		}
		return r, nil
	}
	s.drop(in.Step)
	if p.enroll != "" {
		sealed, err := seal.Seal(s.DataKey, p.enroll)
		if err != nil {
			return Response{}, err
		}
		u.TOTPSecret = sealed
		if err := s.Store.SaveSigninUser(ctx, u); err != nil {
			return Response{}, err
		}
	}
	return s.success(ctx, h, u, ipKey, acctKey)
}

func (s *Service) success(ctx context.Context, h *store.ProxyHost, u *store.SigninUser, ipKey, acctKey string) (Response, error) {
	s.IP.Reset(ipKey)
	s.Account.Reset(acctKey)
	if err := s.Store.TouchSigninUser(ctx, u.ID); err != nil {
		slog.Warn("sign-in: recording the last sign-in failed", "user", u.ID, "err", err)
	}
	return Response{Result: ResultOK, UserID: u.ID, User: u.Username, Version: u.SessionVersion,
		Hours: h.Login.SessionHours, PassUser: h.Login.PassUser, Event: "login_succeeded"}, nil
}

// allowed reports whether u is in one of the groups the host's login page lets in.
func allowed(h *store.ProxyHost, u *store.SigninUser) bool {
	for _, g := range h.Login.GroupIDs {
		for _, ug := range u.GroupIDs {
			if g == ug {
				return true
			}
		}
	}
	return false
}

// hostLabel names the site in the authenticator app: the host name the visitor used if it
// belongs to the host, else its first domain.
func hostLabel(name string, h *store.ProxyHost) string {
	name = strings.ToLower(strings.TrimSpace(name))
	for _, d := range h.Domains {
		if d == name {
			return name
		}
	}
	if len(h.Domains) > 0 {
		return h.Domains[0]
	}
	return "corvus"
}

// ------------------------------------------------------------------ pending second steps

func (s *Service) put(p *pending) string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	tok := base64.RawURLEncoding.EncodeToString(b)
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.steps == nil {
		s.steps = map[string]*pending{}
	}
	now := s.now()
	if len(s.steps) >= maxSteps {
		for k, v := range s.steps {
			if now.After(v.expires) {
				delete(s.steps, k)
			}
		}
		if len(s.steps) >= maxSteps { // still full: drop an arbitrary one
			for k := range s.steps {
				delete(s.steps, k)
				break
			}
		}
	}
	s.steps[tok] = p
	return tok
}

func (s *Service) get(tok string) *pending {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.steps[tok]
	if p == nil {
		return nil
	}
	if s.now().After(p.expires) || p.attempts >= stepAttempts {
		delete(s.steps, tok)
		return nil
	}
	cp := *p
	return &cp
}

// attempt counts a wrong code; false when the step is used up.
func (s *Service) attempt(tok string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	p := s.steps[tok]
	if p == nil {
		return false
	}
	p.attempts++
	if p.attempts >= stepAttempts {
		delete(s.steps, tok)
		return false
	}
	return true
}

func (s *Service) drop(tok string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.steps, tok)
}
