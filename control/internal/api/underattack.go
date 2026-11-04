package api

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"corvus/control/internal/underattack"
)

// GET /api/under-attack — hosts whose under-attack mode is on or auto, and whether it
// is active now (auto: since / until / reason of the current activation).
func (s *Server) listUnderAttack(w http.ResponseWriter, r *http.Request) {
	if s.UnderAttack == nil {
		writeJSON(w, 200, []underattack.Status{})
		return
	}
	list, err := s.UnderAttack.Statuses(r.Context(), time.Now())
	if err != nil {
		fail(w, err)
		return
	}
	writeJSON(w, 200, list)
}

// POST /api/proxy-hosts/{id}/under-attack/deactivate — ends an automatic activation
// now; the host is not activated automatically again for one activation period.
func (s *Server) deactivateUnderAttack(w http.ResponseWriter, r *http.Request) {
	id, ok := pathID(w, r)
	if !ok {
		return
	}
	if s.UnderAttack == nil {
		writeErr(w, http.StatusConflict, "not_active", underattack.ErrNotActive.Error())
		return
	}
	err := s.UnderAttack.Deactivate(r.Context(), id, time.Now())
	if errors.Is(err, underattack.ErrNotActive) {
		writeErr(w, http.StatusConflict, "not_active", err.Error())
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	s.audit(r, "under_attack.deactivate", fmt.Sprintf("proxy_host:%d", id), nil)
	writeJSON(w, 200, map[string]string{"status": "ok"})
}

// underAttackChanged re-evaluates and pushes after a proxy host change (mode switched
// off, host deleted or disabled: its activation ends without waiting for the next round).
func (s *Server) underAttackChanged() {
	if s.UnderAttack != nil {
		s.UnderAttack.Trigger()
	}
}
