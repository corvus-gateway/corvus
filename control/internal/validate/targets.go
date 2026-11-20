package validate

import (
	"fmt"
	"strconv"

	"corvus/control/internal/store"
)

// MaxTargets is the number of forward targets (backends) a proxy host may have,
// including its forward host.
const MaxTargets = 32

// extraTargets normalizes and validates a proxy host's additional forward targets in
// place (after its forward host): each one as the forward host, no target twice.
func extraTargets(errs *Errors, h *store.ProxyHost) {
	if h.ExtraTargets == nil {
		h.ExtraTargets = []store.Target{}
	}
	if len(h.ExtraTargets)+1 > MaxTargets {
		errs.add("extra_targets", "too_many", "at most %d backends per host", MaxTargets)
		return
	}
	seen := map[string]bool{h.ForwardHost + ":" + strconv.Itoa(h.ForwardPort): true}
	for i := range h.ExtraTargets {
		t := &h.ExtraTargets[i]
		prefix := fmt.Sprintf("extra_targets[%d].", i)
		t.ForwardHost = forwardHost(errs, prefix+"forward_host", t.ForwardHost)
		if t.ForwardPort < 1 || t.ForwardPort > 65535 {
			errs.add(prefix+"forward_port", "invalid", "port must be 1-65535")
		}
		key := t.ForwardHost + ":" + strconv.Itoa(t.ForwardPort)
		if seen[key] {
			errs.add(prefix+"forward_host", "duplicate", "backend %s is listed twice", key)
		}
		seen[key] = true
	}
}
