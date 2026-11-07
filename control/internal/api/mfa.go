package api

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	qrcode "github.com/skip2/go-qrcode"

	"corvus/control/internal/auth"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
)

// verifySecondFactor accepts a TOTP code (not reused) or a one-time recovery code.
func (s *Server) verifySecondFactor(ctx context.Context, u *store.User, code string) bool {
	code = strings.TrimSpace(code)
	if len(code) == 6 {
		secret, err := seal.Open(s.DataKey, u.TOTPSecret())
		if err != nil || secret == "" {
			return false
		}
		step := auth.TOTPStep(secret, code, time.Now())
		if step < 0 {
			return false
		}
		ok, err := s.Store.ClaimTOTPStep(ctx, u.ID, step) // persisted: no replay across restarts (H11)
		return err == nil && ok
	}
	var hashes []string
	_ = json.Unmarshal([]byte(u.RecoveryCodes()), &hashes)
	h := auth.RecoveryHash(code)
	for i, v := range hashes {
		if v == h {
			rest := append(append([]string{}, hashes[:i]...), hashes[i+1:]...)
			// only if the stored codes are still the ones read: two requests using codes
			// at the same time cannot both succeed or bring a used code back
			ok, err := s.Store.ReplaceRecoveryCodes(ctx, u.ID, u.RecoveryCodes(), rest)
			return err == nil && ok
		}
	}
	return false
}

func (s *Server) handleLoginMFA(w http.ResponseWriter, r *http.Request) {
	var in struct {
		MFAToken string `json:"mfa_token"`
		Code     string `json:"code"`
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
	uid, ok := s.MFA.Peek(in.MFAToken)
	if !ok {
		writeErr(w, http.StatusUnauthorized, "mfa_expired", "the sign-in attempt expired, sign in again")
		return
	}
	u, err := s.Store.UserByID(r.Context(), uid)
	if err != nil || u.Disabled {
		writeErr(w, http.StatusUnauthorized, "mfa_expired", "the sign-in attempt expired, sign in again")
		return
	}
	acct := fmt.Sprintf("mfa:%d", u.ID)
	if blocked, _ := s.AcctLimiter.Blocked(acct); blocked {
		writeErr(w, http.StatusTooManyRequests, "rate_limited", "too many attempts, try again later")
		return
	}
	if !s.verifySecondFactor(r.Context(), u, in.Code) {
		s.Limiter.Fail(ip)
		s.AcctLimiter.Fail(acct)
		s.audit(r, "auth.mfa_failed", u.Email, nil)
		writeErr(w, http.StatusUnauthorized, "invalid_code", "invalid verification code")
		return
	}
	s.MFA.Finish(in.MFAToken)
	s.Limiter.Reset(ip)
	s.AcctLimiter.Reset(acct)
	s.AcctLimiter.Reset(s.accountKey(r, u.Email))
	if err := s.setSession(w, r, u); err != nil {
		fail(w, err)
		return
	}
	s.rememberDevice(w, r, u.Email)
	_ = s.Store.TouchLogin(r.Context(), u.ID)
	r = r.WithContext(context.WithValue(r.Context(), ctxKey{}, u))
	s.audit(r, "auth.login", u.Email, map[string]bool{"mfa": true})
	writeJSON(w, 200, u)
}

func (s *Server) mfaSetup(w http.ResponseWriter, r *http.Request) {
	u := userFrom(r)
	if u.MFAEnabled {
		writeErr(w, http.StatusConflict, "mfa_enabled", "two-factor authentication is already enabled")
		return
	}
	secret := auth.NewTOTPSecret()
	enc, err := seal.Seal(s.DataKey, secret)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SetTOTPSecret(r.Context(), u.ID, enc); err != nil {
		fail(w, err)
		return
	}
	otpURL := auth.TOTPURL("Corvus", u.Email, secret)
	png, err := qrcode.Encode(otpURL, qrcode.Medium, 256)
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "auth.mfa_setup_started", u.Email, nil)
	writeJSON(w, 200, map[string]string{"secret": secret, "otpauth_url": otpURL, "qr_png": "data:image/png;base64," + base64.StdEncoding.EncodeToString(png)})
}

func (s *Server) mfaEnable(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Code string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Store.UserByID(r.Context(), userFrom(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	if u.MFAEnabled {
		writeErr(w, http.StatusConflict, "mfa_enabled", "two-factor authentication is already enabled")
		return
	}
	if u.TOTPSecret() == "" {
		writeErr(w, http.StatusBadRequest, "mfa_not_started", "start the setup first")
		return
	}
	if len(strings.TrimSpace(in.Code)) != 6 || !s.verifySecondFactor(r.Context(), u, in.Code) {
		writeErr(w, http.StatusUnprocessableEntity, "invalid_code", "the code does not match; check the device time and try again")
		return
	}
	codes, hashes := auth.NewRecoveryCodes(10)
	if err := s.Store.SetMFA(r.Context(), u.ID, true, hashes); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "auth.mfa_enabled", u.Email, nil)
	writeJSON(w, 200, map[string]any{"recovery_codes": codes})
}

func (s *Server) mfaDisable(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Password string `json:"password"`
		Code     string `json:"code"`
	}
	if !decode(w, r, &in) {
		return
	}
	u, err := s.Store.UserByID(r.Context(), userFrom(r).ID)
	if err != nil {
		fail(w, err)
		return
	}
	if !u.MFAEnabled {
		writeErr(w, http.StatusConflict, "mfa_disabled", "two-factor authentication is not enabled")
		return
	}
	if !auth.CheckPassword(u.PasswordHash(), in.Password) || !s.verifySecondFactor(r.Context(), u, in.Code) {
		s.audit(r, "auth.mfa_disable_failed", u.Email, nil)
		writeErr(w, http.StatusForbidden, "invalid_credentials", "password or verification code is incorrect")
		return
	}
	if err := s.Store.SetMFA(r.Context(), u.ID, false, nil); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "auth.mfa_disabled", u.Email, nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}
