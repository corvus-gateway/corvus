package autoban

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"
)

// StatusKeyHeader carries the per-start key (CORVUS_STATUS_KEY) the gateway requires
// on its internal status endpoints (/bans, /bad-behaviour, /under-attack), so they are
// not reachable through a proxied target that points at the loopback status port.
const StatusKeyHeader = "X-Corvus-Status-Key"

// MaxPushBytes is the largest body the gateway accepts on its status port for /bans
// (client_max_body_size in nginx.conf). An IPv4 entry takes about 25 bytes of JSON, so
// this holds roughly 300,000 bans.
const MaxPushBytes = 8 << 20

// FailureWarnAfter consecutive failed pushes are logged as a warning (single failures
// happen while the gateway restarts and are logged at debug level only).
const FailureWarnAfter = 3

// failureRewarn repeats the warning while pushes keep failing.
const failureRewarn = time.Hour

// PushStatus describes the synchronisation of a list corvus-control pushes to the
// gateway (ban list, under-attack activations).
type PushStatus struct {
	LastSuccess         *time.Time `json:"last_success,omitempty"`
	LastAttempt         *time.Time `json:"last_attempt,omitempty"`
	LastError           string     `json:"last_error,omitempty"`
	ConsecutiveFailures int        `json:"consecutive_failures"`
	Bytes               int        `json:"bytes"`   // size of the last payload sent
	Entries             int        `json:"entries"` // entries in the last payload sent
}

// PushTracker records the outcome of pushes and logs a warning when they keep failing
// (and once when they work again), so a gateway that stopped enforcing bans is noticed.
type PushTracker struct {
	Name string // e.g. "ban list"

	mu       sync.Mutex
	st       PushStatus
	warnedAt time.Time
}

// Record notes a push attempt of a payload with the given size and entry count.
func (p *PushTracker) Record(now time.Time, bytes, entries int, err error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	t := now
	p.st.LastAttempt = &t
	p.st.Bytes, p.st.Entries = bytes, entries
	if err == nil {
		if p.st.ConsecutiveFailures >= FailureWarnAfter {
			slog.Info("gateway "+p.Name+" push works again", "failures", p.st.ConsecutiveFailures)
		}
		p.st.LastSuccess = &t
		p.st.LastError = ""
		p.st.ConsecutiveFailures = 0
		p.warnedAt = time.Time{}
		return
	}
	p.st.ConsecutiveFailures++
	p.st.LastError = err.Error()
	if p.st.ConsecutiveFailures < FailureWarnAfter {
		slog.Debug("gateway "+p.Name+" push failed", "err", err)
		return
	}
	if p.warnedAt.IsZero() || now.Sub(p.warnedAt) >= failureRewarn {
		p.warnedAt = now
		var since any = "never"
		if p.st.LastSuccess != nil {
			since = p.st.LastSuccess.Format(time.RFC3339)
		}
		slog.Warn("gateway "+p.Name+" push keeps failing: the gateway is not up to date",
			"failures", p.st.ConsecutiveFailures, "last_success", since, "err", err)
	}
}

// Status returns a copy of the current state.
func (p *PushTracker) Status() PushStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.st
}

// PayloadHash identifies a payload (without its timestamp) to skip unchanged pushes.
func PayloadHash(v any) string {
	b, _ := json.Marshal(v)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

// PutJSON sends body to the gateway status port and turns a non-200 answer into an
// error that includes the gateway's message (e.g. "no ban store").
func PutJSON(req *http.Request, c *http.Client) error {
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == 200 {
		return nil
	}
	msg, _ := io.ReadAll(io.LimitReader(resp.Body, 200))
	detail := strings.TrimSpace(string(msg))
	if resp.StatusCode == http.StatusRequestEntityTooLarge {
		return fmt.Errorf("gateway answered 413: the list (%d bytes) exceeds the gateway's body limit", req.ContentLength)
	}
	if detail != "" && !strings.Contains(detail, "<") {
		return fmt.Errorf("gateway answered %d: %s", resp.StatusCode, detail)
	}
	return fmt.Errorf("gateway answered %d", resp.StatusCode)
}
