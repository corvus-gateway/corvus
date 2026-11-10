package acme

import (
	"net/url"
	"regexp"
	"sort"
	"strings"
)

// Certificate errors are stored in the certificate (visible to every signed-in user)
// and sent in alerts, so they are cleaned before that: DNS provider errors can quote
// request URLs that carry credentials in the query string, or the credential itself.

var urlInText = regexp.MustCompile(`(?i)https?://[^\s"'<>]+`)

// redactError returns err's message with every credential value replaced by *** and
// the query string and fragment of every URL removed.
func redactError(msg string, cred Credentials) string {
	var secrets []string
	add := func(v string) {
		v = strings.TrimSpace(v)
		if len(v) < 4 { // too short to be a secret; replacing it would mangle the message
			return
		}
		secrets = append(secrets, v, url.QueryEscape(v), url.PathEscape(v))
	}
	add(cred.APIToken)
	add(cred.Email)
	add(cred.APIKey)
	for _, v := range cred.Values {
		add(v)
	}
	// longest first, so a value containing another is replaced whole
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, s := range secrets {
		msg = strings.ReplaceAll(msg, s, "***")
	}
	return urlInText.ReplaceAllStringFunc(msg, func(u string) string {
		if i := strings.IndexAny(u, "?#"); i >= 0 {
			return u[:i] + "?…"
		}
		return u
	})
}
