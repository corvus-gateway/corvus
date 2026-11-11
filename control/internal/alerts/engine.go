package alerts

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sort"
	"strings"
	"sync"
	"time"

	"corvus/control/internal/live"
	"corvus/control/internal/seal"
	"corvus/control/internal/store"
	"corvus/control/internal/traffic"
	"corvus/control/internal/underattack"
)

// Sources are the signals the engine evaluates.
type Sources struct {
	Traffic *traffic.Collector
	RedisUp func(ctx context.Context) bool
	Rooms   func(ctx context.Context) []live.Stats
	// UnderAttack returns the automatic under-attack activations and deactivations
	// since the last call (underattack.Service.Drain).
	UnderAttack func() []underattack.Transition
}

type Engine struct {
	Store    *store.Store
	Key      []byte // data key (channel targets are sealed)
	Src      Sources
	Interval time.Duration

	mu          sync.Mutex
	state       map[string]*active
	resolved    map[string]time.Time // recently resolved alerts (re-firing within the cooldown is quiet)
	lastVersion int64
	started     time.Time

	sem chan struct{}  // bounds concurrent delivery attempts (not held while waiting to retry)
	wg  sync.WaitGroup // deliveries in flight
}

// maxDeliveries bounds concurrent delivery attempts; failedRetry is how long an alert
// whose notification could not be delivered anywhere waits before the next try,
// counted from the end of the failed delivery (longer when a receiver asked for it
// with Retry-After).
const (
	maxDeliveries = 8
	failedRetry   = 2 * time.Minute
)

type active struct {
	Rule     string    `json:"rule"`
	Key      string    `json:"key"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	Text     string    `json:"text"`
	Since    time.Time `json:"since"`
	lastSent time.Time // last successful notification (zero: not delivered yet)
	retryAt  time.Time // after a failed notification, no new attempt before this
	inflight bool      // a notification is being delivered
	failLog  bool      // a failed notification is in the history (one per episode)
	msg      Message
	missed   int // evaluation rounds in a row without the condition
}

// outgoing is a notification to deliver; st is the alert whose state it updates.
// logFail: record the delivery in the history also when it fails.
type outgoing struct {
	m       Message
	st      *active
	at      time.Time
	logFail bool
}

// finding is one detected condition in an evaluation round.
type finding struct {
	rule, key string
	oneShot   bool // events (deploy failure) are not "resolved"
	title     string
	text      string
	fields    [][2]string
}

func (e *Engine) init() {
	if e.state == nil {
		e.state = map[string]*active{}
	}
	if e.resolved == nil {
		e.resolved = map[string]time.Time{}
	}
	if e.sem == nil {
		e.sem = make(chan struct{}, maxDeliveries)
	}
}

// Run evaluates rules periodically until ctx is done.
func (e *Engine) Run(ctx context.Context) {
	e.mu.Lock()
	e.init()
	e.started = time.Now()
	e.lastVersion, _ = e.Store.MaxVersionID(ctx) // only failures after start-up
	e.mu.Unlock()
	iv := e.Interval
	if iv == 0 {
		iv = 30 * time.Second
	}
	t := time.NewTicker(iv)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			e.Evaluate(ctx, time.Now())
		}
	}
}

// Active returns the currently firing alerts.
func (e *Engine) Active() []active {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := []active{}
	for _, a := range e.state {
		out = append(out, *a)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Since.After(out[j].Since) })
	return out
}

// Evaluate runs one detection round and sends notifications.
func (e *Engine) Evaluate(ctx context.Context, now time.Time) {
	set := Load(ctx, e.Store)
	tr := texts[set.Language]
	found := e.detect(ctx, set, tr, now)

	e.mu.Lock()
	e.init()
	seen := map[string]bool{}
	var send []outgoing
	cooldown := time.Duration(set.CooldownS) * time.Second
	for _, f := range found {
		r := set.Rules[f.rule]
		id := f.rule + "|" + f.key
		seen[id] = true
		m := Message{Rule: f.rule, Severity: r.Severity, Status: "firing", Title: f.title, Text: f.text, Fields: f.fields,
			Labels: map[string]string{"rule": f.rule, "key": f.key}, Link: link(set.PublicURL, f.rule), Time: now}
		a, ok := e.state[id]
		if ok {
			a.missed = 0
		}
		switch {
		case !ok && !f.oneShot && now.Sub(e.resolved[id]) < cooldown:
			// back within the cooldown of its last resolution (a flapping condition):
			// track it again without a new notification
			e.state[id] = &active{Rule: f.rule, Key: f.key, Severity: r.Severity, Title: f.title, Text: f.text, Since: now, lastSent: e.resolved[id], msg: m}
			delete(e.resolved, id)
		case !ok:
			o := outgoing{m: m, at: now, logFail: true}
			if !f.oneShot {
				o.st = &active{Rule: f.rule, Key: f.key, Severity: r.Severity, Title: f.title, Text: f.text, Since: now, inflight: true, msg: m}
				e.state[id] = o.st
			}
			send = append(send, o)
		case !a.inflight && !now.Before(a.retryAt) && (a.lastSent.IsZero() || now.Sub(a.lastSent) >= cooldown):
			// due: the cooldown since the last delivered notification has passed, or
			// the first notification failed and is tried again
			if !a.lastSent.IsZero() {
				m.Title = tr["ongoing"] + " · " + m.Title
			}
			a.inflight, a.msg, a.Title, a.Text = true, m, f.title, f.text
			send = append(send, outgoing{m: m, st: a, at: now, logFail: !a.failLog})
		default:
			a.msg, a.Title, a.Text = m, f.title, f.text
		}
	}
	for id, a := range e.state {
		if seen[id] {
			continue
		}
		// one round without the condition can be noise: resolve after two in a row
		if a.missed++; a.missed < 2 {
			continue
		}
		delete(e.state, id)
		e.resolved[id] = now
		// (not for an alert whose firing notification never got through)
		if set.SendResolved && (!a.lastSent.IsZero() || a.inflight) {
			m := a.msg
			m.Status, m.Time = "resolved", now
			m.Title = tr["resolved"] + " · " + a.Title
			m.Text = fmt.Sprintf(tr["resolvedText"], humanDuration(now.Sub(a.Since), set.Language))
			send = append(send, outgoing{m: m, at: now, logFail: true})
		}
	}
	for id, t := range e.resolved {
		if now.Sub(t) >= cooldown {
			delete(e.resolved, id)
		}
	}
	e.mu.Unlock()

	// Deliveries (with their retries) run in the background so a slow or failing
	// receiver does not hold up the next evaluation round.
	for _, o := range send {
		e.wg.Add(1)
		go func() {
			defer e.wg.Done()
			start := clockNow()
			delivered, retryAfter := e.dispatch(ctx, o.m, o.logFail)
			if o.st == nil {
				return
			}
			e.mu.Lock()
			o.st.inflight = false
			if delivered {
				o.st.lastSent, o.st.retryAt, o.st.failLog = o.at, time.Time{}, false
			} else {
				// try again soon, but not every round: counted from the end of this
				// delivery (with its retries), at least what a receiver asked for
				done := o.at.Add(clockNow().Sub(start))
				o.st.retryAt = done.Add(max(failedRetry, retryAfter))
				o.st.failLog = o.st.failLog || o.logFail
			}
			e.mu.Unlock()
		}()
	}
}

// wait blocks until the deliveries in flight are done (tests).
func (e *Engine) wait() { e.wg.Wait() }

func link(base, rule string) string {
	if base == "" {
		return ""
	}
	switch rule {
	case "cert_expiry", "acme_failed":
		return base + "/certificates"
	case "deploy_failed":
		return base + "/deployments"
	case "login_failed":
		return base + "/audit"
	case "waitroom_queue", "redis_down":
		return base + "/waitrooms"
	case "under_attack":
		return base + "/proxy-hosts"
	}
	return base + "/traffic"
}

func (e *Engine) detect(ctx context.Context, set Settings, tr map[string]string, now time.Time) []finding {
	var out []finding
	on := func(id string) (Rule, bool) { r, ok := set.Rules[id]; return r, ok && r.Enabled }
	win := func(r Rule) time.Duration {
		if r.WindowS <= 0 {
			return 5 * time.Minute
		}
		return time.Duration(r.WindowS) * time.Second
	}
	mins := func(d time.Duration) string { return fmt.Sprint(int(d.Minutes())) }

	if c := e.Src.Traffic; c != nil {
		hosts := c.HostNames()
		if r, ok := on("error_rate"); ok {
			for _, h := range hosts {
				s := c.SummaryFor(win(r), h, now)
				if s.Requests >= int64(r.MinRequests) && s.Requests > 0 {
					pct := float64(s.S5xx) / float64(s.Requests) * 100
					if pct >= r.Threshold {
						out = append(out, finding{rule: "error_rate", key: h, title: fmt.Sprintf(tr["error_rate"], h),
							text:   fmt.Sprintf(tr["error_rateText"], mins(win(r)), pct, s.S5xx, s.Requests),
							fields: [][2]string{{tr["host"], h}, {"5xx", fmt.Sprintf("%.1f%%", pct)}, {tr["threshold"], fmt.Sprintf("%g%%", r.Threshold)}}})
					}
				}
			}
		}
		if r, ok := on("latency_p95"); ok {
			for _, h := range hosts {
				s := c.SummaryFor(win(r), h, now)
				if s.Requests >= int64(r.MinRequests) && s.Requests > 0 && s.P95 >= r.Threshold {
					out = append(out, finding{rule: "latency_p95", key: h, title: fmt.Sprintf(tr["latency_p95"], h),
						text:   fmt.Sprintf(tr["latency_p95Text"], mins(win(r)), s.P95, s.P50),
						fields: [][2]string{{tr["host"], h}, {"p95", fmt.Sprintf("%g ms", s.P95)}, {tr["threshold"], fmt.Sprintf("%g ms", r.Threshold)}}})
				}
			}
		}
		if r, ok := on("traffic_spike"); ok && now.Sub(c.Since()) > 15*time.Minute {
			cur := c.SummaryFor(time.Minute, "", now).RPS
			base := c.SummaryFor(time.Hour, "", now).AvgRPS
			if cur >= float64(r.MinRequests) && base > 0 && cur >= r.Threshold*base {
				out = append(out, finding{rule: "traffic_spike", key: "global", title: tr["traffic_spike"],
					text:   fmt.Sprintf(tr["traffic_spikeText"], cur, base, cur/base),
					fields: [][2]string{{"RPS", fmt.Sprintf("%.1f", cur)}, {tr["baseline"], fmt.Sprintf("%.1f", base)}}})
			}
		}
		if r, ok := on("rate_limited"); ok {
			for h, n := range c.EventCounts(now.Add(-win(r)), "rate_limited") {
				if float64(n) >= r.Threshold {
					out = append(out, finding{rule: "rate_limited", key: h, title: fmt.Sprintf(tr["rate_limited"], h),
						text: fmt.Sprintf(tr["rate_limitedText"], mins(win(r)), n), fields: [][2]string{{tr["host"], h}, {tr["count"], fmt.Sprint(n)}}})
				}
			}
		}
		if r, ok := on("exploit"); ok {
			since := now.Add(-win(r))
			blocked := c.EventCounts(since, "blocked_exploit")
			for h, n := range c.EventCounts(since, "blocked_exploit", "exploit_detected") {
				if float64(n) >= r.Threshold {
					out = append(out, finding{rule: "exploit", key: h, title: fmt.Sprintf(tr["exploit"], h),
						text:   fmt.Sprintf(tr["exploitText"], mins(win(r)), n, blocked[h]),
						fields: [][2]string{{tr["host"], h}, {tr["count"], fmt.Sprint(n)}, {tr["blocked"], fmt.Sprint(blocked[h])}}})
				}
			}
		}
	}

	if c := e.Src.Traffic; c != nil {
		out = append(out, securityFindings(c, on, win, mins, tr, now)...)
	}

	if e.Src.UnderAttack != nil {
		events := e.Src.UnderAttack() // drained every round, also while the rule is off
		if _, ok := on("under_attack"); ok {
			out = append(out, underAttackFindings(events, tr, set.Language)...)
		}
	}

	if e.Src.Rooms != nil {
		rooms := e.Src.Rooms(ctx)
		if r, ok := on("waitroom_queue"); ok {
			th := r.Threshold
			if th < 1 {
				th = 1
			}
			for _, s := range rooms {
				if s.Available && float64(s.Waiting) >= th {
					out = append(out, finding{rule: "waitroom_queue", key: s.Name, title: fmt.Sprintf(tr["waitroom_queue"], s.Name),
						text:   fmt.Sprintf(tr["waitroom_queueText"], s.Waiting, s.Active, s.Capacity),
						fields: [][2]string{{tr["room"], s.Name}, {tr["waiting"], fmt.Sprint(s.Waiting)}, {tr["active"], fmt.Sprintf("%d / %d", s.Active, s.Capacity)}}})
				}
			}
		}
		if _, ok := on("redis_down"); ok && len(rooms) > 0 && e.Src.RedisUp != nil && !e.Src.RedisUp(ctx) {
			out = append(out, finding{rule: "redis_down", key: "redis", title: tr["redis_down"], text: tr["redis_downText"]})
		}
	}

	if certs, err := e.Store.ListCertificates(ctx); err == nil {
		r1, expOn := on("cert_expiry")
		_, acmeOn := on("acme_failed")
		for _, c := range certs {
			name := c.Name
			if len(c.Domains) > 0 {
				name = strings.Join(c.Domains, ", ")
			}
			key := fmt.Sprint(c.ID)
			days := time.Until(c.NotAfter).Hours() / 24
			// also after failed renewals (status "error" once expired); not while pending
			// a Let's Encrypt certificate whose first issuance failed has no expiry date yet
			// (not_after is still its creation time): that is acme_failed, not cert_expiry
			neverIssued := c.Kind == "acme" && !c.NotAfter.After(c.CreatedAt.Add(time.Hour))
			if expOn && c.Status != "pending" && !c.NotAfter.IsZero() && !neverIssued && days <= r1.Threshold {
				out = append(out, finding{rule: "cert_expiry", key: key, title: fmt.Sprintf(tr["cert_expiry"], name),
					text:   fmt.Sprintf(tr["cert_expiryText"], days, c.NotAfter.Format("2006-01-02"), map[bool]string{true: tr["autoRenewOn"], false: tr["autoRenewOff"]}[c.AutoRenew]),
					fields: [][2]string{{tr["certificate"], name}, {tr["expires"], c.NotAfter.Format("2006-01-02")}}})
			}
			if acmeOn && c.Kind == "acme" && c.LastError != "" && c.Status != "pending" {
				out = append(out, finding{rule: "acme_failed", key: key, title: fmt.Sprintf(tr["acme_failed"], name),
					text: trim(c.LastError, 400) + hintSuffix(set.Language, c.LastErrorHint), fields: [][2]string{{tr["certificate"], name}, {tr["status"], c.Status}}})
			}
		}
	}

	if _, ok := on("deploy_failed"); ok {
		e.mu.Lock()
		after := e.lastVersion
		e.mu.Unlock()
		if vs, err := e.Store.FailedVersionsSince(ctx, after); err == nil {
			// advance only past what was read: a failure recorded meanwhile is not skipped
			last := after
			for _, v := range vs {
				if v.ID > last {
					last = v.ID
				}
				who := v.Email
				if who == "" {
					who = "system"
				}
				out = append(out, finding{rule: "deploy_failed", key: fmt.Sprint(v.ID), oneShot: true, title: fmt.Sprintf(tr["deploy_failed"], v.ID),
					text: trim(v.TestOutput, 500), fields: [][2]string{{tr["change"], v.Message}, {tr["by"], who}}})
			}
			e.mu.Lock()
			e.lastVersion = last
			e.mu.Unlock()
		}
	}

	if r, ok := on("login_failed"); ok {
		since := now.Add(-win(r))
		n1, _ := e.Store.CountAudit(ctx, "auth.login_failed", since)
		n2, _ := e.Store.CountAudit(ctx, "auth.mfa_failed", since)
		if float64(n1+n2) >= r.Threshold && r.Threshold > 0 {
			out = append(out, finding{rule: "login_failed", key: "admin", title: tr["login_failed"],
				text: fmt.Sprintf(tr["login_failedText"], mins(win(r)), n1+n2), fields: [][2]string{{tr["count"], fmt.Sprint(n1 + n2)}}})
		}
	}
	return out
}

func trim(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "…"
}

func humanDuration(d time.Duration, lang string) string {
	m := int(d.Round(time.Minute).Minutes())
	if lang == "en" {
		if m < 1 {
			return "under a minute"
		}
		return fmt.Sprintf("%d min", m)
	}
	if m < 1 {
		return "1분 미만"
	}
	return fmt.Sprintf("%d분", m)
}

// ---------------------------------------------------------------- delivery

func (e *Engine) targets(ctx context.Context) ([]store.AlertChannel, error) {
	return e.Store.ListAlertChannels(ctx)
}

func (e *Engine) open(c store.AlertChannel) (Target, error) {
	var t Target
	raw, err := seal.Open(e.Key, c.Target)
	if err != nil {
		return t, err
	}
	return t, json.Unmarshal([]byte(raw), &t)
}

// dispatch delivers m to every matching channel in parallel (with retries) and
// records the results (a failed delivery only with logFail). It reports whether the
// notification got through: to at least one channel, or there was no channel to
// deliver to; and otherwise the longest Retry-After a receiver answered with.
func (e *Engine) dispatch(ctx context.Context, m Message, logFail bool) (bool, time.Duration) {
	chans, err := e.targets(ctx)
	if err != nil {
		slog.Error("alerts: list channels", "err", err)
		return false, 0
	}
	rec := &store.AlertRecord{Rule: m.Rule, Severity: m.Severity, Status: m.Status, Key: m.Labels["key"], Title: m.Title, Message: m.Text}
	var to []store.AlertChannel
	for _, c := range chans {
		if !c.Enabled || !atLeast(m.Severity, c.MinSeverity) && m.Status != "resolved" {
			continue
		}
		if len(c.Events) > 0 && !contains(c.Events, m.Rule) {
			continue
		}
		to = append(to, c)
	}
	delivered := len(to) == 0
	var retryAfter time.Duration
	if len(to) > 0 {
		rec.Results = make([]store.DeliveryResult, len(to))
		after := make([]time.Duration, len(to))
		var wg sync.WaitGroup
		for i, c := range to {
			wg.Add(1)
			go func() {
				defer wg.Done()
				rec.Results[i], after[i] = e.deliver(ctx, c, m, e.sem)
			}()
		}
		wg.Wait()
		for i, r := range rec.Results {
			delivered = delivered || r.OK
			retryAfter = max(retryAfter, after[i])
		}
	}
	if delivered || logFail {
		if err := e.Store.AddAlert(ctx, rec); err != nil {
			slog.Error("alerts: record history", "err", err)
		}
	}
	slog.Info("alert", "rule", m.Rule, "status", m.Status, "severity", m.Severity, "title", m.Title, "channels", len(rec.Results), "delivered", delivered)
	return delivered, retryAfter
}

// deliver sends m to one channel. With a semaphore, failed attempts are repeated (see
// sendRetry), each attempt holding a slot; it also returns the Retry-After of a final
// 429 answer.
func (e *Engine) deliver(ctx context.Context, c store.AlertChannel, m Message, sem chan struct{}) (store.DeliveryResult, time.Duration) {
	res := store.DeliveryResult{ChannelID: c.ID, Channel: c.Name}
	var after time.Duration
	t, err := e.open(c)
	if err == nil && sem != nil {
		d := sendRetry(ctx, c.Kind, t, m, sem)
		err, after = d.err, d.retryAfter
	} else if err == nil {
		sctx, cancel := context.WithTimeout(ctx, 15*time.Second)
		_, err = Send(sctx, c.Kind, t, m)
		cancel()
	}
	if err != nil {
		res.Error = trim(err.Error(), 300)
		slog.Warn("alert delivery failed", "channel", c.Name, "kind", c.Kind, "err", res.Error)
	} else {
		res.OK = true
	}
	return res, after
}

// Test sends a test message to one channel and records it in the history.
func (e *Engine) Test(ctx context.Context, c store.AlertChannel) store.DeliveryResult {
	set := Load(ctx, e.Store)
	tr := texts[set.Language]
	m := Message{Rule: "test", Severity: "info", Status: "test", Title: tr["test"], Text: tr["testText"], Time: time.Now(),
		Fields: [][2]string{{tr["channel"], c.Name}, {"kind", c.Kind}}, Link: set.PublicURL}
	res, _ := e.deliver(ctx, c, m, nil) // one attempt: the console waits for the answer
	_ = e.Store.AddAlert(ctx, &store.AlertRecord{Rule: "test", Severity: "info", Status: "test", Title: m.Title, Message: m.Text, Results: []store.DeliveryResult{res}})
	return res
}

func contains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}
