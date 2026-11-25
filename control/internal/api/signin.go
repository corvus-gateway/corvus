package api

import (
	"context"
	"fmt"
	"net/http"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// Sign-in users and the login page in front of proxy hosts (#294). Users and groups are
// plain data (no gateway deployment); a host's login settings are part of the host.

// loginInput is a host's login page in a request. pin is write-only: omitted or "" keeps
// the stored PIN, a new value replaces it (and ends the sessions signed in with the old
// one). pin_set is read-only and ignored.
type loginInput struct {
	Enabled      bool     `json:"enabled"`
	Mode         string   `json:"mode"`
	GroupIDs     []int64  `json:"group_ids"`
	SessionHours int      `json:"session_hours"`
	ExcludePaths []string `json:"exclude_paths"`
	PassUser     bool     `json:"pass_user"`
	PIN          *string  `json:"pin"`
	PINSet       *bool    `json:"pin_set"`
}

// applyLogin copies a login page from the request onto h (nil = unchanged).
func applyLogin(h *store.ProxyHost, in *loginInput) error {
	if in == nil {
		return nil
	}
	l := store.HostLogin{Enabled: in.Enabled, Mode: in.Mode, GroupIDs: in.GroupIDs, SessionHours: in.SessionHours,
		ExcludePaths: in.ExcludePaths, PassUser: in.PassUser, PINHash: h.Login.PINHash, PINVersion: h.Login.PINVersion}
	if in.PIN != nil && *in.PIN != "" {
		if err := validate.PIN(*in.PIN); err != nil {
			return err
		}
		hash, err := auth.HashSecret(*in.PIN)
		if err != nil {
			return err
		}
		l.PINHash = hash
		l.PINVersion++
	}
	l.PINSet = l.PINHash != ""
	h.Login = l
	return nil
}

// checkHostLogin checks a host's login page against the rest of the configuration; it
// runs after Basic auth was applied (inside the deploy transaction).
func checkHostLogin(ctx context.Context, tx *store.Store, h *store.ProxyHost) error {
	if !h.Login.Enabled {
		return nil
	}
	var errs validate.Errors
	if h.AccessListID != nil {
		if al, err := tx.AccessListByID(ctx, *h.AccessListID); err == nil && len(al.Users) > 0 {
			errs = append(errs, validate.FieldError{Field: "login", Code: "basic_auth",
				Message: "turn off password protection (Basic auth) to use the login page"})
		}
	}
	if h.Login.Mode == store.LoginUsers {
		groups, err := tx.ListSigninGroups(ctx)
		if err != nil {
			return err
		}
		known := map[int64]bool{}
		for _, g := range groups {
			known[g.ID] = true
		}
		for _, id := range h.Login.GroupIDs {
			if !known[id] {
				errs = append(errs, validate.FieldError{Field: "login.group_ids", Code: "not_found", Message: fmt.Sprintf("group %d does not exist", id)})
			}
		}
	}
	return errs.OrNil()
}

// ------------------------------------------------------------------ users

type signinUserInput struct {
	Username     string  `json:"username"`
	Password     string  `json:"password"` // "" = unchanged
	TOTPRequired bool    `json:"totp_required"`
	Enabled      *bool   `json:"enabled"` // omitted = enabled
	Note         string  `json:"note"`
	GroupIDs     []int64 `json:"group_ids"`
}

func (s *Server) listSigninUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSigninUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

// buildSigninUser applies the input to u, hashing a new password. A changed password,
// a disabled user or a dropped TOTP requirement end the user's sessions.
func (s *Server) buildSigninUser(ctx context.Context, u *store.SigninUser, in signinUserInput) error {
	wasEnabled, wasRequired := u.Enabled, u.TOTPRequired
	u.Username, u.TOTPRequired, u.Note, u.GroupIDs = in.Username, in.TOTPRequired, in.Note, in.GroupIDs
	u.Enabled = in.Enabled == nil || *in.Enabled
	if err := validate.SigninUser(u, in.Password); err != nil {
		return err
	}
	if err := s.checkSigninGroups(ctx, u.GroupIDs); err != nil {
		return err
	}
	if in.Password != "" {
		hash, err := auth.HashSecret(in.Password)
		if err != nil {
			return err
		}
		if u.PasswordHash != "" {
			u.SessionVersion++
		}
		u.PasswordHash = hash
	}
	if u.ID != 0 && ((wasEnabled && !u.Enabled) || (wasRequired && !u.TOTPRequired)) {
		u.SessionVersion++
	}
	if !u.TOTPRequired {
		u.TOTPSecret = "" // enrolment starts over if two-step is required again
	}
	return nil
}

func (s *Server) checkSigninGroups(ctx context.Context, ids []int64) error {
	groups, err := s.Store.ListSigninGroups(ctx)
	if err != nil {
		return err
	}
	known := map[int64]bool{}
	for _, g := range groups {
		known[g.ID] = true
	}
	var errs validate.Errors
	for _, id := range ids {
		if !known[id] {
			errs = append(errs, validate.FieldError{Field: "group_ids", Code: "not_found", Message: fmt.Sprintf("group %d does not exist", id)})
		}
	}
	return errs.OrNil()
}

func (s *Server) createSigninUser(w http.ResponseWriter, r *http.Request) {
	var in signinUserInput
	if !decode(w, r, &in) {
		return
	}
	u := &store.SigninUser{}
	if err := s.buildSigninUser(r.Context(), u, in); err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SaveSigninUser(r.Context(), u); err != nil {
		if store.IsUnique(err) {
			err = validate.Errors{{Field: "username", Code: "duplicate", Message: "a user with this name already exists"}}
		}
		fail(w, err)
		return
	}
	s.audit(r, "signin_user.create", fmt.Sprintf("signin_user:%d", u.ID), u)
	writeJSON(w, 201, u)
}

func (s *Server) updateSigninUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in signinUserInput
	if !decode(w, r, &in) {
		return
	}
	before, err := s.Store.SigninUserByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	cp := *before
	u := &cp
	if err := s.buildSigninUser(r.Context(), u, in); err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SaveSigninUser(r.Context(), u); err != nil {
		if store.IsUnique(err) {
			err = validate.Errors{{Field: "username", Code: "duplicate", Message: "a user with this name already exists"}}
		}
		fail(w, err)
		return
	}
	s.audit(r, "signin_user.update", fmt.Sprintf("signin_user:%d", id), map[string]any{
		"before": before, "after": u, "password_changed": in.Password != ""})
	writeJSON(w, 200, u)
}

func (s *Server) deleteSigninUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	before, err := s.Store.SigninUserByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.DeleteSigninUser(r.Context(), id); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "signin_user.delete", fmt.Sprintf("signin_user:%d", id), before)
	writeJSON(w, 200, map[string]any{"deleted": id})
}

// endSigninSessions signs the user out everywhere.
func (s *Server) endSigninSessions(w http.ResponseWriter, r *http.Request) {
	s.bumpSigninUser(w, r, "signin_user.end_sessions", false)
}

// resetSigninMFA removes the user's authenticator; the next sign-in enrols a new one.
func (s *Server) resetSigninMFA(w http.ResponseWriter, r *http.Request) {
	s.bumpSigninUser(w, r, "signin_user.reset_mfa", true)
}

func (s *Server) bumpSigninUser(w http.ResponseWriter, r *http.Request, action string, resetMFA bool) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, err := s.Store.SigninUserByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	u.SessionVersion++
	if resetMFA {
		u.TOTPSecret = ""
	}
	if err := s.Store.SaveSigninUser(r.Context(), u); err != nil {
		fail(w, err)
		return
	}
	s.audit(r, action, fmt.Sprintf("signin_user:%d", id), map[string]any{"username": u.Username})
	writeJSON(w, 200, u)
}

// ------------------------------------------------------------------ groups

type signinGroupInput struct {
	Name    string  `json:"name"`
	UserIDs []int64 `json:"user_ids"`
}

func (s *Server) listSigninGroups(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListSigninGroups(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) saveSigninGroup(w http.ResponseWriter, r *http.Request, g *store.SigninGroup, in signinGroupInput) bool {
	g.Name, g.UserIDs = in.Name, in.UserIDs
	if err := validate.SigninGroup(g); err != nil {
		fail(w, err)
		return false
	}
	users, err := s.Store.ListSigninUsers(r.Context())
	if err != nil {
		fail(w, err)
		return false
	}
	known := map[int64]bool{}
	for _, u := range users {
		known[u.ID] = true
	}
	var errs validate.Errors
	for _, id := range g.UserIDs {
		if !known[id] {
			errs = append(errs, validate.FieldError{Field: "user_ids", Code: "not_found", Message: fmt.Sprintf("user %d does not exist", id)})
		}
	}
	if len(errs) > 0 {
		fail(w, errs)
		return false
	}
	if err := s.Store.SaveSigninGroup(r.Context(), g); err != nil {
		if store.IsUnique(err) {
			err = validate.Errors{{Field: "name", Code: "duplicate", Message: "a group with this name already exists"}}
		}
		fail(w, err)
		return false
	}
	return true
}

func (s *Server) createSigninGroup(w http.ResponseWriter, r *http.Request) {
	var in signinGroupInput
	if !decode(w, r, &in) {
		return
	}
	g := &store.SigninGroup{}
	if !s.saveSigninGroup(w, r, g, in) {
		return
	}
	s.audit(r, "signin_group.create", fmt.Sprintf("signin_group:%d", g.ID), g)
	writeJSON(w, 201, g)
}

func (s *Server) updateSigninGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in signinGroupInput
	if !decode(w, r, &in) {
		return
	}
	before, err := s.Store.SigninGroupByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	cp := *before
	if !s.saveSigninGroup(w, r, &cp, in) {
		return
	}
	s.audit(r, "signin_group.update", fmt.Sprintf("signin_group:%d", id), map[string]any{"before": before, "after": cp})
	writeJSON(w, 200, cp)
}

func (s *Server) deleteSigninGroup(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	before, err := s.Store.SigninGroupByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.DeleteSigninGroup(r.Context(), id); err != nil {
		fail(w, conflictError{err.Error()})
		return
	}
	s.audit(r, "signin_group.delete", fmt.Sprintf("signin_group:%d", id), before)
	writeJSON(w, 200, map[string]any{"deleted": id})
}
