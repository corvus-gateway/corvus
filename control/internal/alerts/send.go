package alerts

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"

	"corvus/control/internal/netcfg"
)

// Target is the (sealed) destination configuration of a channel.
type Target struct {
	URL      string `json:"url,omitempty"`
	Username string `json:"username,omitempty"` // mattermost display name override
	Channel  string `json:"channel,omitempty"`  // mattermost channel override
	BotToken string `json:"bot_token,omitempty"`
	ChatID   string `json:"chat_id,omitempty"`
	Secret   string `json:"secret,omitempty"` // generic webhook HMAC key
}

var Kinds = []string{"slack", "mattermost", "discord", "teams", "telegram", "webhook"}

func ValidKind(k string) bool {
	for _, v := range Kinds {
		if v == k {
			return true
		}
	}
	return false
}

// CheckTarget validates the destination for a channel kind.
func CheckTarget(kind string, t Target) error {
	if kind == "telegram" {
		if t.BotToken == "" || t.ChatID == "" {
			return errors.New("bot_token and chat_id are required")
		}
		if strings.ContainsAny(t.BotToken, "/?#@ ") {
			return errors.New("invalid bot token")
		}
		return nil
	}
	u, err := url.Parse(strings.TrimSpace(t.URL))
	if err != nil || u.Host == "" || (u.Scheme != "https" && u.Scheme != "http") {
		return errors.New("a valid http(s) webhook URL is required")
	}
	if u.User != nil {
		return errors.New("credentials in the URL are not allowed")
	}
	if ip := net.ParseIP(u.Hostname()); (ip != nil && blockedIP(ip)) || strings.EqualFold(u.Hostname(), "localhost") {
		return errors.New("loopback and link-local destinations are not allowed")
	}
	return nil
}

// Hint returns a masked, display-safe representation of the target.
func Hint(kind string, t Target) string {
	if kind == "telegram" {
		return "chat " + t.ChatID
	}
	u, err := url.Parse(t.URL)
	if err != nil {
		return ""
	}
	tail := u.Path
	if len(tail) > 4 {
		tail = tail[len(tail)-4:]
	}
	return u.Host + "/…/" + strings.TrimLeft(tail, "/")
}

// Message is a rendered notification.
type Message struct {
	Rule     string            `json:"rule"`
	Severity string            `json:"severity"`
	Status   string            `json:"status"` // firing | resolved | test
	Title    string            `json:"title"`
	Text     string            `json:"text"`
	Fields   [][2]string       `json:"-"`
	Labels   map[string]string `json:"labels,omitempty"`
	Link     string            `json:"link,omitempty"`
	Time     time.Time         `json:"time"`
}

func color(m Message) (string, int) {
	switch {
	case m.Status == "resolved":
		return "#16a34a", 0x16a34a
	case m.Status == "test":
		return "#4f46e5", 0x4f46e5
	case m.Severity == "critical":
		return "#dc2626", 0xdc2626
	case m.Severity == "warning":
		return "#d97706", 0xd97706
	}
	return "#2563eb", 0x2563eb
}

func icon(m Message) string {
	switch {
	case m.Status == "resolved":
		return "✅"
	case m.Status == "test":
		return "🔔"
	case m.Severity == "critical":
		return "🔴"
	case m.Severity == "warning":
		return "🟠"
	}
	return "🔵"
}

func esc(s string) string { // Telegram HTML escaping
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;").Replace(s)
}

// payload builds the request body for a channel kind.
func payload(kind string, t Target, m Message) (string, []byte, error) {
	hex, num := color(m)
	head := icon(m) + " " + m.Title
	switch kind {
	case "slack", "mattermost":
		fields := []map[string]any{}
		for _, f := range m.Fields {
			fields = append(fields, map[string]any{"title": f[0], "value": f[1], "short": true})
		}
		att := map[string]any{"color": hex, "title": head, "text": m.Text, "fields": fields, "footer": "Corvus Sentinel", "ts": m.Time.Unix(), "fallback": head}
		if m.Link != "" {
			att["title_link"] = m.Link
		}
		body := map[string]any{"text": "", "attachments": []any{att}}
		if kind == "mattermost" {
			if t.Username != "" {
				body["username"] = t.Username
			} else {
				body["username"] = "Corvus Sentinel"
			}
			if t.Channel != "" {
				body["channel"] = t.Channel
			}
		}
		b, err := json.Marshal(body)
		return t.URL, b, err
	case "discord":
		fields := []map[string]any{}
		for _, f := range m.Fields {
			fields = append(fields, map[string]any{"name": f[0], "value": f[1], "inline": true})
		}
		emb := map[string]any{"title": head, "description": m.Text, "color": num, "fields": fields, "timestamp": m.Time.UTC().Format(time.RFC3339), "footer": map[string]string{"text": "Corvus Sentinel"}}
		if m.Link != "" {
			emb["url"] = m.Link
		}
		b, err := json.Marshal(map[string]any{"username": "Corvus Sentinel", "embeds": []any{emb}, "allowed_mentions": map[string]any{"parse": []string{}}})
		return t.URL, b, err
	case "teams":
		facts := []map[string]string{}
		for _, f := range m.Fields {
			facts = append(facts, map[string]string{"title": f[0], "value": f[1]})
		}
		bodyItems := []any{
			map[string]any{"type": "TextBlock", "size": "Medium", "weight": "Bolder", "text": head, "wrap": true},
			map[string]any{"type": "TextBlock", "text": m.Text, "wrap": true},
			map[string]any{"type": "FactSet", "facts": facts},
		}
		card := map[string]any{"type": "AdaptiveCard", "version": "1.4", "$schema": "http://adaptivecards.io/schemas/adaptive-card.json", "body": bodyItems}
		if m.Link != "" {
			card["actions"] = []any{map[string]string{"type": "Action.OpenUrl", "title": "Corvus AWPG", "url": m.Link}}
		}
		b, err := json.Marshal(map[string]any{"type": "message", "attachments": []any{map[string]any{"contentType": "application/vnd.microsoft.card.adaptive", "content": card}}})
		return t.URL, b, err
	case "telegram":
		var sb strings.Builder
		sb.WriteString("<b>" + esc(head) + "</b>\n" + esc(m.Text))
		for _, f := range m.Fields {
			sb.WriteString("\n<b>" + esc(f[0]) + ":</b> " + esc(f[1]))
		}
		if m.Link != "" {
			sb.WriteString("\n" + esc(m.Link))
		}
		b, err := json.Marshal(map[string]any{"chat_id": t.ChatID, "text": sb.String(), "parse_mode": "HTML", "disable_web_page_preview": true})
		return "https://api.telegram.org/bot" + t.BotToken + "/sendMessage", b, err
	case "webhook":
		labels := map[string]string{}
		for _, f := range m.Fields {
			labels[f[0]] = f[1]
		}
		b, err := json.Marshal(map[string]any{"source": "corvus-sentinel", "rule": m.Rule, "severity": m.Severity, "status": m.Status,
			"title": m.Title, "text": m.Text, "fields": labels, "labels": m.Labels, "link": m.Link, "time": m.Time.UTC().Format(time.RFC3339)})
		return t.URL, b, err
	}
	return "", nil, fmt.Errorf("unsupported channel kind %q", kind)
}

// blockedIP refuses loopback, link-local (cloud metadata) and unspecified
// destinations so a webhook URL cannot be used to reach the host itself.
// Private networks stay allowed for self-hosted Mattermost etc.
var allowLoopback = false // tests only

func blockedIP(ip net.IP) bool {
	if v4, transition := netcfg.EmbeddedIPv4(ip); transition {
		// NAT64/6to4 addresses are judged by the IPv4 address they carry; Teredo and
		// local-use NAT64 have no reliably readable IPv4 part and are refused.
		return v4 == nil || blockedIP(v4)
	}
	if allowLoopback && ip.IsLoopback() {
		return false
	}
	return ip.IsLoopback() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast()
}

var client = &http.Client{
	Timeout: 10 * time.Second,
	Transport: &http.Transport{
		// No proxy: the dial guard below must see the real destination address.
		Proxy: nil,
		DialContext: (&net.Dialer{Timeout: 5 * time.Second, Control: func(network, address string, _ syscall.RawConn) error {
			host, _, _ := net.SplitHostPort(address)
			if ip := net.ParseIP(host); ip != nil && blockedIP(ip) {
				return fmt.Errorf("destination %s is not allowed", host)
			}
			return nil
		}}).DialContext,
		TLSHandshakeTimeout: 5 * time.Second,
		MaxIdleConns:        10,
	},
	CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
}

// Send delivers m to one channel. It returns the HTTP status (0 on transport errors).
func Send(ctx context.Context, kind string, t Target, m Message) (int, error) {
	a := send(ctx, kind, t, m)
	return a.code, a.err
}

// attempt is the outcome of one delivery attempt.
type attempt struct {
	code       int
	err        error
	retry      bool          // worth retrying: transport error, 408, 429 or 5xx
	retryAfter time.Duration // from a 429 response's Retry-After header
}

// Delivery retries: a failed delivery is attempted again after each of these delays
// (a 429's Retry-After replaces the delay, capped at maxRetryAfter). Retry-After is
// read up to maxRetryAt: the alert's next round waits at least that long.
var (
	retryDelays   = []time.Duration{5 * time.Second, 30 * time.Second}
	maxRetryAfter = 2 * time.Minute
	maxRetryAt    = time.Hour
)

// The clock of deliveries; tests replace it to simulate time.
var (
	clockNow = time.Now
	sleep    = func(ctx context.Context, d time.Duration) bool { // false: ctx ended first
		timer := time.NewTimer(d)
		defer timer.Stop()
		select {
		case <-ctx.Done():
			return false
		case <-timer.C:
			return true
		}
	}
)

// delivery is the outcome of a delivery with retries.
type delivery struct {
	code       int
	err        error
	retryAfter time.Duration // Retry-After of the last attempt (429)
}

// sendRetry delivers m, retrying transport errors, 408, 429 and 5xx responses;
// other 4xx answers (a wrong URL or token) are final. Each attempt has its own timeout
// and holds a slot of sem (if any); the wait between attempts does not.
func sendRetry(ctx context.Context, kind string, t Target, m Message, sem chan struct{}) delivery {
	var last delivery
	for i := 0; ; i++ {
		if sem != nil {
			select {
			case sem <- struct{}{}:
			case <-ctx.Done():
				if last.err == nil {
					last.err = ctx.Err()
				}
				return last
			}
		}
		actx, cancel := context.WithTimeout(ctx, 15*time.Second)
		a := send(actx, kind, t, m)
		cancel()
		if sem != nil {
			<-sem
		}
		last = delivery{code: a.code, err: a.err, retryAfter: a.retryAfter}
		if a.err == nil || !a.retry || i >= len(retryDelays) {
			return last
		}
		wait := retryDelays[i]
		if a.retryAfter > 0 {
			wait = min(a.retryAfter, maxRetryAfter)
		}
		// the error can quote a remote answer: keep it on one log line
		msg := strings.NewReplacer("\n", " ", "\r", " ").Replace(a.err.Error())
		slog.Info("alert delivery failed, retrying", "kind", kind, "err", msg, "in", wait)
		if !sleep(ctx, wait) {
			return last
		}
	}
}

// parseRetryAfter reads a Retry-After header (seconds or an HTTP date), capped at
// maxRetryAt; 0 when absent or unreadable.
func parseRetryAfter(v string, now time.Time) time.Duration {
	v = strings.TrimSpace(v)
	if v == "" {
		return 0
	}
	if n, err := strconv.Atoi(v); err == nil {
		if n <= 0 {
			return 0
		}
		return time.Duration(min(n, int(maxRetryAt/time.Second))) * time.Second
	}
	if at, err := http.ParseTime(v); err == nil && at.After(now) {
		return min(at.Sub(now), maxRetryAt)
	}
	return 0
}

func send(ctx context.Context, kind string, t Target, m Message) attempt {
	dest, body, err := payload(kind, t, m)
	if err != nil {
		return attempt{err: err}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(body))
	if err != nil {
		return attempt{err: err}
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "corvus-alerts")
	if kind == "webhook" && t.Secret != "" {
		ts := strconv.FormatInt(time.Now().Unix(), 10)
		mac := hmac.New(sha256.New, []byte(t.Secret))
		mac.Write([]byte(ts + "." + string(body)))
		req.Header.Set("X-Corvus-Timestamp", ts)
		req.Header.Set("X-Corvus-Signature", "sha256="+hex.EncodeToString(mac.Sum(nil)))
	}
	resp, err := client.Do(req)
	if err != nil {
		return attempt{err: transportError(err, req.URL, t), retry: true}
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10)) // lets the connection be reused
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The response body is not reported: it may echo request details.
		a := attempt{code: resp.StatusCode, err: fmt.Errorf("HTTP %d", resp.StatusCode)}
		switch {
		case resp.StatusCode == http.StatusTooManyRequests:
			a.retry, a.retryAfter = true, parseRetryAfter(resp.Header.Get("Retry-After"), clockNow())
		case resp.StatusCode == http.StatusRequestTimeout || resp.StatusCode >= 500:
			a.retry = true
		}
		return a
	}
	return attempt{code: resp.StatusCode}
}

// transportError describes a failed request without the destination's path or
// query, where webhook services keep their secrets; only scheme and host remain.
func transportError(err error, dest *url.URL, t Target) error {
	msg := err.Error()
	var ue *url.Error
	if errors.As(err, &ue) {
		msg = fmt.Sprintf("%s %s://%s/…: %v", ue.Op, dest.Scheme, dest.Host, ue.Err)
	}
	for _, s := range []string{t.BotToken, t.Secret, dest.Path, dest.RawPath, dest.RawQuery} {
		if len(s) > 1 {
			msg = strings.ReplaceAll(msg, s, "***")
		}
	}
	return errors.New(msg)
}
