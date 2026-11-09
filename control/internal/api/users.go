package api

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
	"corvus/control/internal/validate"
)

// requireRole wraps a handler so only users with at least role min may call it.
func (s *Server) requireRole(min string, next http.HandlerFunc) http.Handler {
	return s.requireAuth(func(w http.ResponseWriter, r *http.Request) {
		u := userFrom(r)
		if !store.RoleAtLeast(u.Role, min) {
			s.audit(r, "auth.forbidden", r.Method+" "+r.URL.Path, map[string]string{"role": u.Role, "required": min})
			writeErr(w, http.StatusForbidden, "forbidden", "you do not have permission for this action")
			return
		}
		next(w, r)
	})
}

type userInput struct {
	Email    string `json:"email"`
	Name     string `json:"name"`
	Role     string `json:"role"`
	Password string `json:"password"`
	Disabled *bool  `json:"disabled"`
}

func (s *Server) listUsers(w http.ResponseWriter, r *http.Request) {
	list, err := s.Store.ListUsers(r.Context())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

func (s *Server) createUser(w http.ResponseWriter, r *http.Request) {
	var in userInput
	if !decode(w, r, &in) {
		return
	}
	var errs validate.Errors
	in.Email = strings.ToLower(strings.TrimSpace(in.Email))
	if !emailOK(in.Email) {
		errs = append(errs, validate.FieldError{Field: "email", Code: "invalid", Message: "a valid email is required"})
	}
	if !store.ValidRole(in.Role) {
		errs = append(errs, validate.FieldError{Field: "role", Code: "invalid", Message: "role must be admin, operator or viewer"})
	}
	name := strings.TrimSpace(in.Name)
	if name == "" {
		name = strings.SplitN(in.Email, "@", 2)[0]
	}
	if len(name) > 64 {
		errs = append(errs, validate.FieldError{Field: "name", Code: "invalid", Message: "name must be at most 64 characters"})
	}
	hash, err := auth.HashPassword(in.Password)
	if err != nil {
		errs = append(errs, validate.FieldError{Field: "password", Code: "weak", Message: err.Error()})
	}
	if len(errs) > 0 {
		fail(w, errs)
		return
	}
	u, err := s.Store.CreateUser(r.Context(), in.Email, name, hash, in.Role)
	if err != nil {
		if store.IsUnique(err) {
			fail(w, validate.Errors{{Field: "email", Code: "duplicate", Message: "a user with this email already exists"}})
			return
		}
		fail(w, err)
		return
	}
	s.audit(r, "user.create", u.Email, map[string]string{"role": u.Role})
	writeJSON(w, 201, u)
}

func (s *Server) updateUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	var in userInput
	if !decode(w, r, &in) {
		return
	}
	ctx := r.Context()
	me := userFrom(r)
	u, err := s.Store.UserByID(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	role, disabled, name := u.Role, u.Disabled, u.Name
	if in.Role != "" {
		if !store.ValidRole(in.Role) {
			fail(w, validate.Errors{{Field: "role", Code: "invalid", Message: "role must be admin, operator or viewer"}})
			return
		}
		role = in.Role
	}
	if in.Disabled != nil {
		disabled = *in.Disabled
	}
	if n := strings.TrimSpace(in.Name); n != "" {
		name = n
	}
	if id == me.ID && (role != u.Role || disabled) {
		fail(w, conflictError{"you cannot change your own role or disable your own account"})
		return
	}
	if id == me.ID && in.Password != "" {
		// your own password is changed on the account page, which asks for the current
		// password and signs out your other sessions
		fail(w, conflictError{"change your own password on the account page"})
		return
	}
	// Keep at least one active administrator.
	if u.Role == "admin" && !u.Disabled && (role != "admin" || disabled) {
		if n, err := s.Store.CountActiveAdmins(ctx, id); err != nil || n == 0 {
			fail(w, conflictError{"at least one active administrator is required"})
			return
		}
	}
	revoke := role != u.Role || (disabled && !u.Disabled)
	if in.Password != "" {
		hash, err := auth.HashPassword(in.Password)
		if err != nil {
			fail(w, validate.Errors{{Field: "password", Code: "weak", Message: err.Error()}})
			return
		}
		if err := s.Store.UpdatePassword(ctx, id, hash); err != nil {
			fail(w, err)
			return
		}
		revoke = true
		var detail any
		// an administrator resets someone's password when the account may be in the
		// wrong hands: the API tokens it created end too, like its sessions
		if n, err := s.Store.DeleteUserAPITokens(ctx, id); err == nil && n > 0 {
			detail = map[string]any{"tokens_revoked": n}
		}
		s.audit(r, "user.password_reset", u.Email, detail)
	}
	if err := s.Store.KeepingAdmin(ctx, func(tx *store.Store) error {
		return tx.UpdateUser(ctx, id, name, role, disabled)
	}); err != nil {
		fail(w, lastAdminErr(err))
		return
	}
	if revoke {
		_ = s.Store.DeleteUserSessions(ctx, id) // new role / disabled / new password take effect immediately
	}
	s.audit(r, "user.update", u.Email, map[string]any{"role": map[string]string{"before": u.Role, "after": role}, "disabled": disabled})
	nu, _ := s.Store.UserByID(ctx, id)
	writeJSON(w, 200, nu)
}

func (s *Server) deleteUser(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	ctx := r.Context()
	if id == userFrom(r).ID {
		fail(w, conflictError{"you cannot delete your own account"})
		return
	}
	u, err := s.Store.UserByID(ctx, id)
	if err != nil {
		fail(w, err)
		return
	}
	if u.Role == "admin" && !u.Disabled {
		if n, err := s.Store.CountActiveAdmins(ctx, id); err != nil || n == 0 {
			fail(w, conflictError{"at least one active administrator is required"})
			return
		}
	}
	if err := s.Store.KeepingAdmin(ctx, func(tx *store.Store) error {
		return tx.DeleteUser(ctx, id) // sessions cascade
	}); err != nil {
		fail(w, lastAdminErr(err))
		return
	}
	s.audit(r, "user.delete", u.Email, nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

func (s *Server) resetUserMFA(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	u, err := s.Store.UserByID(r.Context(), id)
	if err != nil {
		fail(w, err)
		return
	}
	if err := s.Store.SetMFA(r.Context(), id, false, nil); err != nil {
		fail(w, err)
		return
	}
	_ = s.Store.DeleteUserSessions(r.Context(), id)
	s.audit(r, "user.mfa_reset", u.Email, nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// advancedAllowed: only administrators may change a proxy host's advanced nginx
// configuration (it is raw gateway configuration).
func advancedAllowed(r *http.Request, before, after string) error {
	if strings.TrimSpace(before) == strings.TrimSpace(after) || store.RoleAtLeast(userFrom(r).Role, "admin") {
		return nil
	}
	return validate.Errors{{Field: "advanced_config", Code: "forbidden", Message: fmt.Sprintf("only administrators can change the advanced configuration")}}
}

// locationsAdvancedAllowed: the same for the advanced configuration of custom locations
// (compared by path; other location fields may be changed by operators).
func locationsAdvancedAllowed(r *http.Request, before, after []store.Location) error {
	if store.RoleAtLeast(userFrom(r).Role, "admin") {
		return nil
	}
	was := map[string]string{}
	for _, l := range before {
		was[strings.TrimSpace(l.Path)] = strings.TrimSpace(l.AdvancedConfig)
	}
	var errs validate.Errors
	for i, l := range after {
		if snip := strings.TrimSpace(l.AdvancedConfig); snip != "" && snip != was[strings.TrimSpace(l.Path)] {
			errs = append(errs, validate.FieldError{Field: fmt.Sprintf("locations[%d].advanced_config", i), Code: "forbidden",
				Message: "only administrators can change the advanced configuration"})
		}
	}
	return errs.OrNil()
}

// lastAdminErr turns store.ErrLastAdmin into a 409 like the up-front check.
func lastAdminErr(err error) error {
	if errors.Is(err, store.ErrLastAdmin) {
		return conflictError{err.Error()}
	}
	return err
}
