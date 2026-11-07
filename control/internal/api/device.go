package api

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"net/http"
	"strconv"
	"strings"
	"time"

	"corvus/control/internal/auth"
)

// Known browsers (#142): a browser that completed a sign-in to an account gets a signed
// cookie for that account. Its failed sign-ins are counted on their own, so the account
// lock caused by failures from elsewhere does not keep it out. It is no credential:
// the password (and second factor) are still required.

const (
	deviceCookie = "corvus_device"
	devicePath   = "/api/auth/login" // also covers /api/auth/login/mfa
	deviceTTL    = 30 * 24 * time.Hour
)

func (s *Server) deviceMAC(email, id string, exp int64) string {
	key := hmac.New(sha256.New, s.DataKey)
	key.Write([]byte("corvus known-device v1"))
	m := hmac.New(sha256.New, key.Sum(nil))
	m.Write([]byte(normEmail(email) + "|" + id + "|" + strconv.FormatInt(exp, 10)))
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func normEmail(e string) string { return strings.ToLower(strings.TrimSpace(e)) }

// knownDevice returns the id of this browser when it signed in to email before.
func (s *Server) knownDevice(r *http.Request, email string) (string, bool) {
	if len(s.DataKey) == 0 {
		return "", false
	}
	c, err := r.Cookie(deviceCookie)
	if err != nil {
		return "", false
	}
	parts := strings.Split(c.Value, ".")
	if len(parts) != 3 || len(parts[0]) != 32 {
		return "", false
	}
	exp, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil || time.Now().Unix() > exp {
		return "", false
	}
	if !hmac.Equal([]byte(parts[2]), []byte(s.deviceMAC(email, parts[0], exp))) {
		return "", false
	}
	return parts[0], true
}

// rememberDevice marks this browser as known for email after a completed sign-in.
func (s *Server) rememberDevice(w http.ResponseWriter, r *http.Request, email string) {
	if len(s.DataKey) == 0 {
		return
	}
	id, ok := s.knownDevice(r, email)
	if !ok {
		id = auth.RandomToken(24) // 32 characters
	}
	exp := time.Now().Add(deviceTTL).Unix()
	http.SetCookie(w, &http.Cookie{Name: deviceCookie, Value: id + "." + strconv.FormatInt(exp, 10) + "." + s.deviceMAC(email, id, exp),
		Path: devicePath, HttpOnly: true, Secure: s.SecureCookie, SameSite: http.SameSiteStrictMode, MaxAge: int(deviceTTL.Seconds())})
}

// accountKey is the key failed sign-ins to email count against: the account, or for a
// known browser that browser's own count for the account.
func (s *Server) accountKey(r *http.Request, email string) string {
	key := "acct:" + normEmail(email)
	if id, ok := s.knownDevice(r, email); ok {
		key += "|dev:" + id
	}
	return key
}
