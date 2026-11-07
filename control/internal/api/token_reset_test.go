package api

import (
	"context"
	"fmt"
	"strings"
	"testing"

	"corvus/control/internal/auth"
)

// An administrator resetting another user's password also ends the API tokens that
// user created; other users' tokens are untouched.
func TestPasswordResetRevokesTokens(t *testing.T) {
	e := newTestEnv(t)
	ctx := context.Background()
	op, err := e.s.Store.CreateUser(ctx, "op@example.com", "Op", "x", "operator")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := e.s.Store.CreateAPIToken(ctx, "op-ci", "cvs_opop", auth.TokenHash("cvs_op-token"), "operator", op.ID, nil); err != nil {
		t.Fatal(err)
	}
	adminTok, _ := e.createToken("admin-ci", "viewer", 0)

	// a name change alone keeps the tokens
	if w, _ := e.do("PUT", fmt.Sprintf("/api/users/%d", op.ID), map[string]any{"name": "Operator"}, withCookie(e.cookie), withCSRF()); w.Code != 200 {
		t.Fatalf("rename: %d %s", w.Code, w.Body)
	}
	if _, err := e.s.Store.APITokenByHash(ctx, auth.TokenHash("cvs_op-token")); err != nil {
		t.Fatalf("token gone after rename: %v", err)
	}

	if w, _ := e.do("PUT", fmt.Sprintf("/api/users/%d", op.ID), map[string]any{"password": "Another-long-passw0rd"}, withCookie(e.cookie), withCSRF()); w.Code != 200 {
		t.Fatalf("reset: %d %s", w.Code, w.Body)
	}
	if _, err := e.s.Store.APITokenByHash(ctx, auth.TokenHash("cvs_op-token")); err == nil {
		t.Fatal("operator's token survived the password reset")
	}
	if w, _ := e.do("GET", "/api/auth/me", nil, withBearer(adminTok)); w.Code != 200 {
		t.Fatalf("admin's own token stopped working: %d", w.Code)
	}
	list, _ := e.s.Store.ListAudit(ctx, 10)
	found := false
	for _, a := range list {
		if a.Action == "user.password_reset" && strings.Contains(string(a.Detail), `"tokens_revoked":1`) {
			found = true
		}
	}
	if !found {
		t.Fatalf("audit entry without tokens_revoked: %+v", list)
	}
}
