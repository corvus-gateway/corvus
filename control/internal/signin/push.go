package signin

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"sort"
	"strconv"
	"sync"
	"time"

	"corvus/control/internal/autoban"
	"corvus/control/internal/store"
)

// SessionStore is the part of the store the session push needs.
type SessionStore interface {
	ListSigninUsers(ctx context.Context) ([]store.SigninUser, error)
	ListProxyHosts(ctx context.Context) ([]store.ProxyHost, error)
}

// Sessions pushes what the gateway needs to check login page sessions to its shared memory
// (PUT /signin-sessions on the loopback status port): the session version of every enabled
// user, the PIN version of every host with a login page, and per users-mode host the users
// its groups let in. The gateway accepts a session cookie only while its version matches,
// so a change here (user disabled or deleted, password or PIN changed, two-step reset,
// "end sessions", user removed from a group) ends the affected sessions with the next push.
//
// It pushes at start, after Trigger (every change that affects sessions, and a gateway
// restart, see autoban.Service.OnGatewayRestart) and every PushEvery as a safety net.
type Sessions struct {
	Store   SessionStore
	PushURL string // e.g. http://127.0.0.1:9180/signin-sessions
	// StatusKey is sent with every push (autoban.StatusKeyHeader).
	StatusKey string
	Client    *http.Client

	tracker autoban.PushTracker
	mu      sync.Mutex
	trigger chan struct{}
}

// PushEvery is how often the state is pushed when nothing triggers a push.
const PushEvery = time.Minute

// SessionState is the body of PUT /signin-sessions (see gateway corvus/login.lua).
type SessionState struct {
	Users  map[string]int     `json:"users"`  // user id → session version (enabled users)
	Names  map[string]string  `json:"names"`  // user id → name (X-Corvus-User)
	Hosts  map[string]int     `json:"hosts"`  // proxy host id → PIN version (hosts with a login page)
	Access map[string][]int64 `json:"access"` // users-mode host id → users allowed in
}

// State builds the state from the store.
func (s *Sessions) State(ctx context.Context) (*SessionState, error) {
	users, err := s.Store.ListSigninUsers(ctx)
	if err != nil {
		return nil, err
	}
	hosts, err := s.Store.ListProxyHosts(ctx)
	if err != nil {
		return nil, err
	}
	st := &SessionState{Users: map[string]int{}, Names: map[string]string{}, Hosts: map[string]int{}, Access: map[string][]int64{}}
	byGroup := map[int64][]int64{}
	for _, u := range users {
		if !u.Enabled {
			continue
		}
		id := strconv.FormatInt(u.ID, 10)
		st.Users[id], st.Names[id] = u.SessionVersion, u.Username
		for _, g := range u.GroupIDs {
			byGroup[g] = append(byGroup[g], u.ID)
		}
	}
	for _, h := range hosts {
		if !h.Enabled || !h.Login.Enabled {
			continue
		}
		id := strconv.FormatInt(h.ID, 10)
		st.Hosts[id] = h.Login.PINVersion
		if h.Login.Mode != store.LoginUsers {
			continue
		}
		seen := map[int64]bool{}
		ids := []int64{}
		for _, g := range h.Login.GroupIDs {
			for _, u := range byGroup[g] {
				if !seen[u] {
					seen[u] = true
					ids = append(ids, u)
				}
			}
		}
		sort.Slice(ids, func(i, j int) bool { return ids[i] < ids[j] })
		st.Access[id] = ids
	}
	return st, nil
}

// SyncStatus reports how the session state reaches the gateway (last success, last error).
func (s *Sessions) SyncStatus() autoban.PushStatus {
	return s.tracker.Status()
}

// Push sends the current state to the gateway now.
func (s *Sessions) Push(ctx context.Context) error {
	if s.PushURL == "" {
		return nil
	}
	st, err := s.State(ctx)
	if err != nil {
		return err
	}
	body, _ := json.Marshal(st)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, s.PushURL, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set(autoban.StatusKeyHeader, s.StatusKey)
	c := s.Client
	if c == nil {
		c = &http.Client{Timeout: 5 * time.Second}
	}
	err = autoban.PutJSON(req, c)
	if s.tracker.Name == "" {
		s.tracker.Name = "sign-in session"
	}
	s.tracker.Record(time.Now(), len(body), len(st.Users), err)
	return err
}

// Trigger pushes soon (after a change that affects sessions). Safe on a nil *Sessions.
func (s *Sessions) Trigger() {
	if s == nil {
		return
	}
	select {
	case s.ch() <- struct{}{}:
	default:
	}
}

func (s *Sessions) ch() chan struct{} {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.trigger == nil {
		s.trigger = make(chan struct{}, 1)
	}
	return s.trigger
}

// Run pushes at start, on Trigger and every PushEvery until ctx is done. A failed push
// is retried a few seconds later.
func (s *Sessions) Run(ctx context.Context) {
	ch := s.ch()
	t := time.NewTicker(PushEvery)
	defer t.Stop()
	retry := time.NewTimer(0) // first push at once
	defer retry.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		case <-ch:
		case <-retry.C:
		}
		if err := s.Push(ctx); err != nil {
			slog.Debug("sign-in session push failed", "err", err)
			retry.Reset(3 * time.Second)
		}
	}
}
