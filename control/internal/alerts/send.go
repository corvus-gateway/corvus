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
	dest, body, err := payload(kind, t, m)
	if err != nil {
		return 0, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(body))
	if err != nil {
		return 0, err
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
		return 0, transportError(err, req.URL, t)
	}
	defer resp.Body.Close()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4<<10)) // lets the connection be reused
	if resp.StatusCode < 200 || resp.StatusCode > 299 {
		// The response body is not reported: it may echo request details.
		return resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return resp.StatusCode, nil
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
