package alerts

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"corvus/control/internal/hints"
	"corvus/control/internal/store"
)

// acme_failed carries the actionable sentence of the certificate error's hint.
func TestACMEFailedAlertHasHint(t *testing.T) {
	e, _, h, _ := setup(t)
	ctx := context.Background()
	e.init()
	c := &store.Certificate{Name: "wild", Kind: "acme", Domains: []string{"*.example.com"}, NotAfter: time.Now().Add(-time.Hour),
		Status: "error", AutoRenew: true,
		LastError: "issuance failed: error: one or more domains had a problem:\n[*.example.com] propagation: time limit exceeded: last error: authoritative nameservers: NS ns1.example.net. returned SERVFAIL for _acme-challenge.example.com."}
	if err := e.Store.CreateCertificate(ctx, c); err != nil {
		t.Fatal(err)
	}
	evalWait(e, ctx, time.Now())
	h.mu.Lock()
	defer h.mu.Unlock()
	if len(h.msgs) != 1 {
		t.Fatalf("want one alert, got %d", len(h.msgs))
	}
	b, _ := json.Marshal(h.msgs[0])
	if !strings.Contains(string(b), "DNSSEC") {
		t.Errorf("alert lacks the hint sentence: %s", b)
	}
}

func TestHintTextsCoverACMECodes(t *testing.T) {
	for _, code := range []string{hints.ACMERateLimited, hints.ACMEDNSServfail, hints.ACMEDNSPropagation, hints.ACMEDNSCredential,
		hints.ACMEDNSLookup, hints.ACMECAA, hints.ACMERejectedName, hints.ACMEInvalidEmail, hints.ACMEDomainDNS,
		hints.ACMEPort80, hints.ACMEWrongServer, hints.ACMECAUnreachable} {
		for _, lang := range []string{"ko", "en"} {
			if hintSuffix(lang, code) == "" {
				t.Errorf("no %s text for %s", lang, code)
			}
		}
	}
	if hintSuffix("en", "") != "" || hintSuffix("en", "unknown") != "" {
		t.Error("unknown code must add nothing")
	}
}
