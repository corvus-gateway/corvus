package alerts

// hintTexts holds the actionable sentence appended to an acme_failed alert for each
// certificate error hint code (internal/hints). The console has its own (8 languages).
var hintTexts = map[string]map[string]string{
	"ko": {
		"acme.rate_limited":       "Let's Encrypt 발급 한도에 걸렸습니다. 같은 도메인으로 반복 발급하지 말고 오류에 표시된 시각 이후 다시 시도하세요.",
		"acme.dns_servfail":       "도메인의 네임서버가 _acme-challenge 조회에 SERVFAIL/REFUSED로 응답합니다. DNS 서비스에서 DNSSEC·NS 위임 설정을 확인하세요.",
		"acme.dns_propagation":    "_acme-challenge TXT 레코드가 제한 시간 안에 모든 네임서버에 반영되지 않았습니다. 잠시 후 다시 시도하고, 도메인의 NS가 DNS 인증 정보의 공급자를 가리키는지 확인하세요.",
		"acme.dns_credential":     "DNS 공급자가 TXT 레코드 생성을 거부했습니다. API 토큰의 권한(해당 영역 DNS 편집)과 영역 이름을 확인하세요.",
		"acme.dns_lookup":         "도메인의 권한 네임서버를 찾지 못했습니다. 도메인이 등록·위임되어 있고 Corvus 서버에서 DNS 조회가 되는지 확인하세요.",
		"acme.caa":                "도메인의 CAA 레코드가 Let's Encrypt 발급을 막고 있습니다. CAA에 letsencrypt.org를 추가하거나 레코드를 지우세요.",
		"acme.rejected_name":      "Let's Encrypt가 이 이름에는 발급하지 않습니다. 공개 도메인 이름인지(IP·내부 이름 불가) 확인하세요.",
		"acme.invalid_email":      "Let's Encrypt가 계정 이메일을 거부했습니다. 실제로 수신 가능한 이메일 주소로 바꾸세요.",
		"acme.domain_dns":         "Let's Encrypt가 도메인의 DNS 레코드를 찾지 못했습니다. A/AAAA(또는 CAA) 레코드가 공개 DNS에 있는지 확인하세요.",
		"acme.port80_unreachable": "Let's Encrypt가 80번 포트로 이 서버에 접속하지 못했습니다. 방화벽·포트 포워딩과 도메인의 A/AAAA 레코드를 확인하거나 DNS-01을 사용하세요.",
		"acme.wrong_server":       "Let's Encrypt의 HTTP-01 검증 요청이 Corvus가 아닌 곳에 도착했습니다. 도메인의 A/AAAA 레코드와 앞단 프록시·CDN 설정을 확인하세요.",
		"acme.ca_unreachable":     "Corvus 서버에서 Let's Encrypt에 연결하지 못했습니다. 서버의 외부 접속·DNS·프록시 설정을 확인하세요.",
	},
	"en": {
		"acme.rate_limited":       "Let's Encrypt rate limit reached. Stop retrying the same domains and try again after the time shown in the error.",
		"acme.dns_servfail":       "The domain's nameservers answer SERVFAIL/REFUSED for _acme-challenge. Check DNSSEC and the NS delegation at your DNS provider.",
		"acme.dns_propagation":    "The _acme-challenge TXT record did not reach every nameserver in time. Retry later and check that the domain's NS records point to the DNS credential's provider.",
		"acme.dns_credential":     "The DNS provider refused to create the TXT record. Check the API token's permissions (DNS edit for this zone) and the zone name.",
		"acme.dns_lookup":         "The domain's authoritative nameservers could not be found. Check that the domain is registered and delegated, and that the Corvus server can resolve DNS.",
		"acme.caa":                "A CAA record on the domain forbids Let's Encrypt. Add letsencrypt.org to the CAA record or remove it.",
		"acme.rejected_name":      "Let's Encrypt does not issue for this name. Use a public domain name (no IP addresses or internal names).",
		"acme.invalid_email":      "Let's Encrypt rejected the account email. Use a real, deliverable address.",
		"acme.domain_dns":         "Let's Encrypt could not find DNS records for the domain. Check that its A/AAAA (or CAA) records exist in public DNS.",
		"acme.port80_unreachable": "Let's Encrypt could not reach this server on port 80. Check the firewall, port forwarding and the domain's A/AAAA records, or use DNS-01.",
		"acme.wrong_server":       "Let's Encrypt's HTTP-01 check reached a server other than Corvus. Check the domain's A/AAAA records and any proxy or CDN in front.",
		"acme.ca_unreachable":     "The Corvus server could not reach Let's Encrypt. Check its outbound internet access, DNS and proxy settings.",
	},
}

// hintSuffix returns a new line and the hint sentence for code in the alert language, or "".
func hintSuffix(lang, code string) string {
	if t := hintTexts[lang][code]; t != "" {
		return "\n" + t
	}
	return ""
}
