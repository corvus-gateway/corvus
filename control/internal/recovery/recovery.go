// Package recovery restores console access from inside the container when no
// administrator can sign in (forgotten password, lost authenticator and recovery codes).
// Shell access to the container already controls the data volume, so this only saves
// editing the database by hand.
package recovery

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"corvus/control/internal/auth"
	"corvus/control/internal/store"
)

// Result describes a completed reset.
type Result struct {
	Email         string
	Password      string // the new password, shown once
	MFADisabled   bool
	TokensDeleted int64
}

// ResetAdmin gives an account a new random password, ends its sessions and deletes the
// API tokens it created; disableMFA also turns off its two-step verification. With an
// empty email the only active administrator is chosen.
func ResetAdmin(ctx context.Context, st *store.Store, email string, disableMFA bool) (*Result, error) {
	u, err := pick(ctx, st, strings.TrimSpace(email))
	if err != nil {
		return nil, err
	}
	pw := auth.RandomToken(15) // 20 characters
	hash, err := auth.HashPassword(pw)
	if err != nil {
		return nil, err
	}
	res := &Result{Email: u.Email, Password: pw}
	err = st.WithTx(ctx, func(tx *store.Store) error {
		if err := tx.UpdatePassword(ctx, u.ID, hash); err != nil {
			return err
		}
		if err := tx.DeleteUserSessions(ctx, u.ID); err != nil {
			return err
		}
		n, err := tx.DeleteUserAPITokens(ctx, u.ID)
		if err != nil {
			return err
		}
		res.TokensDeleted = n
		if disableMFA && u.MFAEnabled {
			if err := tx.SetMFA(ctx, u.ID, false, nil); err != nil {
				return err
			}
			res.MFADisabled = true
		}
		return tx.Audit(ctx, nil, "auth.reset_local", u.Email,
			map[string]any{"mfa_disabled": res.MFADisabled, "tokens_revoked": res.TokensDeleted}, "local")
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}

func pick(ctx context.Context, st *store.Store, email string) (*store.User, error) {
	if email != "" {
		u, err := st.UserByEmail(ctx, email)
		if errors.Is(err, store.ErrNotFound) {
			return nil, fmt.Errorf("no account with the e-mail %q", email)
		}
		if err != nil {
			return nil, err
		}
		if u.Disabled {
			return nil, fmt.Errorf("the account %s is disabled; choose an active administrator", u.Email)
		}
		return u, nil
	}
	users, err := st.ListUsers(ctx)
	if err != nil {
		return nil, err
	}
	var admins []store.User
	for _, u := range users {
		if u.Role == "admin" && !u.Disabled {
			admins = append(admins, u)
		}
	}
	switch len(admins) {
	case 0:
		return nil, errors.New("no active administrator; name the account with --email")
	case 1:
		return &admins[0], nil
	}
	names := make([]string, len(admins))
	for i, u := range admins {
		names[i] = u.Email
	}
	return nil, fmt.Errorf("several administrators (%s); choose one with --email", strings.Join(names, ", "))
}
