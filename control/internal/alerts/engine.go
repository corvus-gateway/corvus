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
)

// Sources are the signals the engine evaluates.
type Sources struct {
	Traffic *traffic.Collector
	RedisUp func(ctx context.Context) bool
	Rooms   func(ctx context.Context) []live.Stats
}

type Engine struct {
	Store    *store.Store
	Key      []byte // data key (channel targets are sealed)
	Src      Sources
	Interval time.Duration

	mu          sync.Mutex
	state       map[string]*active
	lastVersion int64
	started     time.Time
}

type active struct {
	Rule     string    `json:"rule"`
	Key      string    `json:"key"`
	Severity string    `json:"severity"`
	Title    string    `json:"title"`
	Text     string    `json:"text"`
	Since    time.Time `json:"since"`
	lastSent time.Time
	msg      Message
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
	var send []Message
	cooldown := time.Duration(set.CooldownS) * time.Second
	for _, f := range found {
		r := set.Rules[f.rule]
		id := f.rule + "|" + f.key
		seen[id] = true
		m := Message{Rule: f.rule, Severity: r.Severity, Status: "firing", Title: f.title, Text: f.text, Fields: f.fields,
			Labels: map[string]string{"rule": f.rule, "key": f.key}, Link: link(set.PublicURL, f.rule), Time: now}
		a, ok := e.state[id]
		switch {
		case !ok:
			if !f.oneShot {
				e.state[id] = &active{Rule: f.rule, Key: f.key, Severity: r.Severity, Title: f.title, Text: f.text, Since: now, lastSent: now, msg: m}
			}
			send = append(send, m)
		case now.Sub(a.lastSent) >= cooldown:
			m.Title = tr["ongoing"] + " · " + m.Title
			a.lastSent, a.msg, a.Title, a.Text = now, m, f.title, f.text
			send = append(send, m)
		default:
			a.msg, a.Title, a.Text = m, f.title, f.text
		}
	}
	for id, a := range e.state {
		if seen[id] {
			continue
		}
		delete(e.state, id)
		if set.SendResolved {
			m := a.msg
			m.Status, m.Time = "resolved", now
			m.Title = tr["resolved"] + " · " + a.Title
			m.Text = fmt.Sprintf(tr["resolvedText"], humanDuration(now.Sub(a.Since), set.Language))
			send = append(send, m)
		}
	}
	e.mu.Unlock()

	for _, m := range send {
		e.dispatch(ctx, m)
	}
}

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
			if expOn && c.Status == "valid" && days <= r1.Threshold {
				out = append(out, finding{rule: "cert_expiry", key: key, title: fmt.Sprintf(tr["cert_expiry"], name),
					text:   fmt.Sprintf(tr["cert_expiryText"], days, c.NotAfter.Format("2006-01-02"), map[bool]string{true: tr["autoRenewOn"], false: tr["autoRenewOff"]}[c.AutoRenew]),
					fields: [][2]string{{tr["certificate"], name}, {tr["expires"], c.NotAfter.Format("2006-01-02")}}})
			}
			if acmeOn && c.Kind == "acme" && c.LastError != "" && c.Status != "pending" {
				out = append(out, finding{rule: "acme_failed", key: key, title: fmt.Sprintf(tr["acme_failed"], name),
					text: trim(c.LastError, 400), fields: [][2]string{{tr["certificate"], name}, {tr["status"], c.Status}}})
			}
		}
	}

	if _, ok := on("deploy_failed"); ok {
		e.mu.Lock()
		after := e.lastVersion
		e.mu.Unlock()
		if vs, err := e.Store.FailedVersionsSince(ctx, after); err == nil {
			for _, v := range vs {
				who := v.Email
				if who == "" {
					who = "system"
				}
				out = append(out, finding{rule: "deploy_failed", key: fmt.Sprint(v.ID), oneShot: true, title: fmt.Sprintf(tr["deploy_failed"], v.ID),
					text: trim(v.TestOutput, 500), fields: [][2]string{{tr["change"], v.Message}, {tr["by"], who}}})
			}
		}
		if id, err := e.Store.MaxVersionID(ctx); err == nil {
			e.mu.Lock()
			e.lastVersion = id
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

func (e *Engine) dispatch(ctx context.Context, m Message) {
	chans, err := e.targets(ctx)
	if err != nil {
		slog.Error("alerts: list channels", "err", err)
		return
	}
	rec := &store.AlertRecord{Rule: m.Rule, Severity: m.Severity, Status: m.Status, Key: m.Labels["key"], Title: m.Title, Message: m.Text}
	for _, c := range chans {
		if !c.Enabled || !atLeast(m.Severity, c.MinSeverity) && m.Status != "resolved" {
			continue
		}
		if len(c.Events) > 0 && !contains(c.Events, m.Rule) {
			continue
		}
		rec.Results = append(rec.Results, e.deliver(ctx, c, m))
	}
	if err := e.Store.AddAlert(ctx, rec); err != nil {
		slog.Error("alerts: record history", "err", err)
	}
	slog.Info("alert", "rule", m.Rule, "status", m.Status, "severity", m.Severity, "title", m.Title, "channels", len(rec.Results))
}

func (e *Engine) deliver(ctx context.Context, c store.AlertChannel, m Message) store.DeliveryResult {
	res := store.DeliveryResult{ChannelID: c.ID, Channel: c.Name}
	t, err := e.open(c)
	if err == nil {
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
	return res
}

// Test sends a test message to one channel and records it in the history.
func (e *Engine) Test(ctx context.Context, c store.AlertChannel) store.DeliveryResult {
	set := Load(ctx, e.Store)
	tr := texts[set.Language]
	m := Message{Rule: "test", Severity: "info", Status: "test", Title: tr["test"], Text: tr["testText"], Time: time.Now(),
		Fields: [][2]string{{tr["channel"], c.Name}, {"kind", c.Kind}}, Link: set.PublicURL}
	res := e.deliver(ctx, c, m)
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
