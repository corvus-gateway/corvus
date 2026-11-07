#!/usr/bin/env bash
# Corvus integration checks against a running dev stack (deploy/dev). Run from a
# machine on the same network as the server, NOT on the gateway itself.
#   CORVUS_HOST=203.0.113.10 CORVUS_ADMIN_FILE=admin.txt ./itest.sh
# admin file: line 1 email, line 2 password. Requires curl, python3, openssl.
set -uo pipefail
H=${CORVUS_HOST:?set CORVUS_HOST}
A=https://$H:9443
G=http://$H:8081
GS=https://$H:8443
J=$(mktemp)
EMAIL=$(sed -n 1p "${CORVUS_ADMIN_FILE:?}")
PASS=$(sed -n 2p "$CORVUS_ADMIN_FILE")
PY=${PYTHON:-python3}
pass=0; failn=0
ok()   { pass=$((pass+1)); echo "  PASS  $*"; }
bad()  { failn=$((failn+1)); echo "  FAIL  $*"; }
chk()  { if [ "$1" = "$2" ]; then ok "$3 ($1)"; else bad "$3 (got $1, want $2)"; fi; }
c()    { curl -sk -b "$J" -c "$J" -H 'X-Corvus-Request: 1' -H 'Content-Type: application/json' "$@"; }
code() { curl -sk -o /dev/null -w '%{http_code}' "$@"; }
js()   { $PY -c "import sys,json; d=json.load(sys.stdin); print($1)"; }

echo "[login]"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/auth/login -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")" 200 "admin login"

# Remove objects created by earlier runs (hosts/certs under *.corvus.test, itest/npm access lists & policies).
cleanup() {
  for id in $(c $A/api/proxy-hosts | js "' '.join(str(h['id']) for h in d if any(x.endswith('.corvus.test') for x in h['domains']))"); do c -o /dev/null -X DELETE $A/api/proxy-hosts/$id; done
  for id in $(c $A/api/certificates | js "' '.join(str(x['id']) for x in d if any(y.endswith('.corvus.test') for y in x['domains']))"); do c -o /dev/null -X DELETE $A/api/certificates/$id; done
  for id in $(c $A/api/access-lists | js "' '.join(str(x['id']) for x in d if x['name'].startswith(('itest','office')))"); do c -o /dev/null -X DELETE $A/api/access-lists/$id; done
  for id in $(c $A/api/alerts/channels | js "' '.join(str(x['id']) for x in d if x['name'].startswith('itest'))"); do c -o /dev/null -X DELETE $A/api/alerts/channels/$id; done
  for id in $(c $A/api/security-rules | js "' '.join(str(x['id']) for x in d if x['name'].startswith('itest') or '(converted from access list)' in x['name'])"); do c -o /dev/null -X DELETE $A/api/security-rules/$id; done
  for id in $(c $A/api/ip-lists | js "' '.join(str(x['id']) for x in d if x['name'].startswith('itest'))"); do c -o /dev/null -X DELETE $A/api/ip-lists/$id; done
  for id in $(c $A/api/rate-limits | js "' '.join(str(x['id']) for x in d if x['name'].startswith('itest'))"); do c -o /dev/null -X DELETE $A/api/rate-limits/$id; done
}
cleanup
AUTOBAN=$(c $A/api/settings/autoban)
c -o /dev/null -X PUT $A/api/settings/autoban -d "$(echo "$AUTOBAN" | js "__import__('json').dumps(dict(d, enabled=False))")"
for id in $(c $A/api/bans | js "' '.join(str(b['id']) for b in d)"); do c -o /dev/null -X DELETE $A/api/bans/$id; done

HOST1=$(c $A/api/proxy-hosts | js "[h['id'] for h in d if '$H' in h['domains']][0]")
put_host() { c -o /dev/null -w '%{http_code}' -X PUT $A/api/proxy-hosts/$HOST1 -d "{\"domains\":[\"$H\"],\"forward_host\":\"demo\",\"forward_port\":80$1}"; }
put_host ',"under_attack":"off"' >/dev/null  # an aborted earlier run may have left it on

echo "[rate limit]"
RL=$(c -X POST $A/api/rate-limits -d '{"name":"itest-strict","requests_per_second":2,"burst":0}' | js "d['rate_limit']['id']")
chk "$(put_host ",\"rate_limit_id\":$RL")" 200 "attach rate limit"
codes=$(for i in $(seq 1 20); do code "$G/rl-$i"; echo; done | sort | uniq -c | tr '\n' ' ')
case "$codes" in *429*) ok "burst of 20 requests limited: $codes";; *) bad "no 429 in: $codes";; esac
ra=$(for i in 1 2 3 4 5 6; do curl -s -D - -o /dev/null "$G/ra-$i"; done | grep -ci '^retry-after')
[ "$ra" -ge 1 ] && ok "Retry-After sent with 429 ($ra)" || bad "no Retry-After header"
c -o /dev/null -X PUT $A/api/rate-limits/$RL -d '{"name":"itest-strict","requests_per_second":2,"burst":0,"dry_run":true}'
codes=$(for i in $(seq 1 15); do code "$G/dry-$i"; echo; done | sort | uniq -c | tr '\n' ' ')
case "$codes" in *429*) bad "dry run still blocks: $codes";; *) ok "dry run never blocks: $codes";; esac

echo "[exploit blocking]"
chk "$(code "$G/.env")" 200 "/.env passes when blocking is off (detect only)"
chk "$(put_host ",\"block_exploits\":true")" 200 "enable block exploits"
chk "$(code "$G/.env")" 403 "/.env blocked"
chk "$(code "$G/?q=1%20union%20select%201")" 403 "SQL injection pattern blocked"
chk "$(code "$G/normal/page?q=hello")" 200 "normal request allowed"
chk "$(code "$G/a/b/.%2e/x")" 403 "encoded traversal (.%2e/) blocked"
chk "$(code "$G/a/.%2e/.%2e/etc/x")" 400 "traversal above the root rejected by nginx"
chk "$(code "$G/a?f=%252e%252e%252fetc")" 403 "double-encoded traversal blocked"
chk "$(code "$G/vendor/%70hpunit/src/Util/PHP/eval-stdin.php")" 403 "encoded scanner path (decoded check) blocked"
chk "$(code "$G/index.php?%ADd+allow_url_include%3d1+%ADd+auto_prepend_file%3dphp%3a%2f%2finput")" 403 "PHP-CGI argument injection (CVE-2024-4577) blocked"
chk "$(code "$G/index.php?p=admin&a=%3C%3Fphp+system(1)%3B")" 403 "PHP code in query blocked"
chk "$(code "$G/?page_id=1&pagename=..%2f..%2fpearcmd")" 403 "pearcmd LFI chain blocked"
chk "$(code "$G/@fs/app/config.json?import&raw??")" 403 "Vite /@fs/ blocked"
chk "$(code "$G/search?q=%EA%B5%AD%EC%96%B4")" 200 "Korean query (%AD bytes) not a false positive"
curl -s -H 'X-Middleware-Subrequest: middleware:middleware:middleware' "$G/echo" | grep -qi 'x-middleware-subrequest' && bad "X-Middleware-Subrequest forwarded" || ok "X-Middleware-Subrequest stripped (CVE-2025-29927)"

echo "[client IP]"
chk "$(put_host ",\"client_ip\":\"cloudflare\"")" 200 "client_ip=cloudflare"
curl -s -o /dev/null -H 'CF-Connecting-IP: 6.6.6.6' "$G/cf-spoof"
sleep 3
ip=$(c "$A/api/traffic/logs?q=/cf-spoof&limit=1" | js "d['items'][0]['ip']")
[ "$ip" != "6.6.6.6" ] && ok "spoofed CF-Connecting-IP from non-Cloudflare address ignored (ip=$ip)" || bad "spoofed header trusted"
MYNET=$(echo "$ip" | cut -d. -f1-3).0/24
c -o /dev/null -X PUT $A/api/settings/network -d "{\"trusted_proxies\":[\"$MYNET\"],\"real_ip_header\":\"X-Forwarded-For\"}"
chk "$(put_host ",\"client_ip\":\"trusted_proxies\"")" 200 "client_ip=trusted_proxies ($MYNET)"
curl -s -o /dev/null -H 'X-Forwarded-For: 7.7.7.7' "$G/xff-trusted"
sleep 3
chk "$(c "$A/api/traffic/logs?q=/xff-trusted&limit=1" | js "d['items'][0]['ip']")" 7.7.7.7 "X-Forwarded-For honoured from trusted proxy"
put_host ',"client_ip":"direct"' >/dev/null

echo "[basic auth (access list) + security rules]"
ACL=$(c -X POST $A/api/access-lists -d '{"name":"itest-office","users":[{"username":"bob","password":"bob-pass-123"}]}' | js "d['access_list']['id']")
chk "$(put_host ",\"access_list_id\":$ACL")" 200 "attach Basic auth list"
chk "$(code "$G/private")" 401 "no credentials"
chk "$(code -u bob:wrong "$G/private")" 401 "wrong password"
chk "$(code -u bob:bob-pass-123 "$G/private")" 200 "Basic auth accepted"
SKIP=$(c -X POST $A/api/security-rules -d "{\"name\":\"itest-office-skip\",\"action\":\"allow\",\"skip_basic_auth\":true,\"expression\":{\"groups\":[[{\"field\":\"ip.src\",\"operator\":\"in\",\"value\":[\"$ip\"]}]]}}" | js "d['rule']['id']")
chk "$(code "$G/private")" 200 "allow rule with skip Basic auth (office network, no password)"
c -o /dev/null -X DELETE $A/api/security-rules/$SKIP
put_host '' >/dev/null
chk "$(c -o /dev/null -w '%{http_code}' -X DELETE $A/api/access-lists/$ACL)" 200 "delete access list"

echo "[password protection in the host form]"
chk "$(put_host ',"basic_auth":{"enabled":true,"users":[{"username":"carol","password":"carol-pass-1"}]}')" 200 "enable password protection on the host"
chk "$(code "$G/pw")" 401 "password required"
chk "$(code -u carol:carol-pass-1 "$G/pw")" 200 "correct password accepted"
chk "$(c $A/api/proxy-hosts/$HOST1 | js "d['basic_auth_users']")" "['carol']" "host shows its users (no hashes)"
chk "$(put_host ',"basic_auth":{"enabled":true,"users":[{"username":"carol","password":""},{"username":"dave","password":"dave-pass-12"}]}')" 200 "add a user, keep the existing password"
chk "$(code -u carol:carol-pass-1 "$G/pw")" 200 "existing password kept"
chk "$(code -u dave:dave-pass-12 "$G/pw")" 200 "new user accepted"
chk "$(put_host ',"basic_auth":{"enabled":true,"users":[{"username":"erin","password":""}]}')" 422 "new user without a password rejected"
chk "$(put_host ',"basic_auth":{"enabled":false,"users":[]}')" 200 "disable password protection"
chk "$(code "$G/pw")" 200 "open again"
chk "$(c $A/api/access-lists | js "any(x['name']=='host-$HOST1' for x in d)")" False "host's own password list removed"
put_host '' >/dev/null

echo "[security rules]"
rule() { c -X POST $A/api/security-rules -d "$1" | js "d['rule']['id']"; }
cond() { echo "{\"name\":\"$1\",\"action\":\"$2\"$3,\"expression\":{\"groups\":[[$4]]}}"; }
chk "$(put_host ",\"block_exploits\":true")" 200 "host with attack-pattern blocking"
B1=$(rule "$(cond itest-block-path block '' '{"field":"http.path","operator":"starts_with","value":["/itest-blocked"]}')")
chk "$(code "$G/itest-blocked/x")" 403 "block: path starts_with"
curl -s "$G/itest-blocked/x" | grep -q "Request ID" && ok "block page with request ID" || bad "block page"
chk "$(code "$G/itest-open")" 200 "unmatched request passes"
c -o /dev/null -X PUT $A/api/security-rules/$B1 -d "$(cond itest-block-path block ',"status_code":410' '{"field":"http.path","operator":"starts_with","value":["/itest-blocked"]}')"
chk "$(code "$G/itest-blocked/x")" 410 "custom block status"
B2=$(rule "$(cond itest-ua block '' '{"field":"http.user_agent","operator":"matches","value":["(?i)sqlmap|nikto"]}')")
chk "$(code -A 'sqlmap/1.8' "$G/ua")" 403 "block: user agent regex"
chk "$(code -A 'Mozilla/5.0' "$G/ua")" 200 "normal user agent passes"
B3=$(rule "$(cond itest-header block '' '{"field":"http.header","name":"X-Itest-Evil","operator":"exists"}')")
chk "$(code -H 'X-Itest-Evil: 1' "$G/h")" 403 "block: header exists"
chk "$(code "$G/h")" 200 "header absent passes"
# ordering: an allow rule below the block rule has no effect until moved above it
MINE="{\"field\":\"ip.src\",\"operator\":\"in\",\"value\":[\"$ip\"]}"
AL=$(rule "{\"name\":\"itest-allow-mine\",\"action\":\"allow\",\"expression\":{\"groups\":[[$MINE,{\"field\":\"http.path\",\"operator\":\"starts_with\",\"value\":[\"/itest-blocked/ok\"]}],[$MINE,{\"field\":\"http.path\",\"operator\":\"starts_with\",\"value\":[\"/itest-x\"]}]]}}")
chk "$(code "$G/itest-blocked/ok")" 410 "allow rule below block rule: still blocked"
IDS=$(c $A/api/security-rules | js "','.join([str($AL)]+[str(r['id']) for r in d if r['id']!=$AL])")
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/security-rules/order -d "{\"ids\":[$IDS]}")" 200 "reorder (allow first)"
chk "$(code "$G/itest-blocked/ok")" 200 "allow rule above block rule wins"
chk "$(code "$G/itest-x/?q=%3Cscript%3E")" 200 "allow skips attack-pattern blocking"
chk "$(code "$G/other/?q=%3Cscript%3E")" 403 "attack pattern still blocked elsewhere"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/security-rules/order -d "{\"ids\":[$AL]}")" 422 "partial order rejected"
# IP lists
L=$(c -X POST $A/api/ip-lists -d "{\"name\":\"itest-list\",\"items\":[\"$ip\",\"10.9.0.0/16\"]}" | js "d['ip_list']['id']")
B4=$(rule "$(cond itest-list block '' "{\"field\":\"ip.src\",\"operator\":\"in\",\"value\":[\"list:$L\"]},{\"field\":\"http.path\",\"operator\":\"eq\",\"value\":[\"/itest-list\"]}")")
chk "$(code "$G/itest-list")" 403 "block: client IP in IP list"
c -o /dev/null -X PUT $A/api/ip-lists/$L -d '{"name":"itest-list","items":["10.9.0.0/16"]}'
chk "$(code "$G/itest-list")" 200 "IP list update applies"
chk "$(c -o /dev/null -w '%{http_code}' -X DELETE $A/api/ip-lists/$L)" 409 "IP list in use cannot be deleted"
# country
chk "$(c $A/api/security-rules/geoip | js "d['available'] and d['ranges']>100000")" True "country database available"
B5=$(rule "$(cond itest-geo block '' '{"field":"ip.country","operator":"in","value":["XX"]},{"field":"http.path","operator":"eq","value":["/itest-geo"]}')")
chk "$(code "$G/itest-geo")" 403 "block: country (private address = XX)"
c -o /dev/null -X PUT $A/api/security-rules/$B5 -d "$(cond itest-geo block '' '{"field":"ip.country","operator":"in","value":["US"]},{"field":"http.path","operator":"eq","value":["/itest-geo"]}')"
chk "$(code "$G/itest-geo")" 200 "other country passes"
# log, 444, scope
LG=$(rule "$(cond itest-log log '' '{"field":"http.path","operator":"eq","value":["/itest-log"]}')")
chk "$(code "$G/itest-log")" 200 "log action does not block"
B6=$(rule "$(cond itest-close block ',"status_code":444' '{"field":"http.path","operator":"eq","value":["/itest-close"]}')")
chk "$(code "$G/itest-close")" 000 "444 closes the connection"
SH=$(c -X POST $A/api/proxy-hosts -d '{"domains":["scope.corvus.test"],"forward_host":"demo","forward_port":80}' | js "d['proxy_host']['id']")
B7=$(rule "{\"name\":\"itest-scope\",\"action\":\"block\",\"scope\":\"hosts\",\"host_ids\":[$SH],\"expression\":{\"groups\":[[{\"field\":\"http.path\",\"operator\":\"eq\",\"value\":[\"/itest-scope\"]}]]}}")
chk "$(code -H 'Host: scope.corvus.test' "$G/itest-scope")" 403 "host-scoped rule applies to its host"
chk "$(code "$G/itest-scope")" 200 "host-scoped rule ignores other hosts"
c -o /dev/null -X DELETE $A/api/proxy-hosts/$SH
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/deployments)" 201 "deploy still works after the scoped host is deleted"
# rate limit skipped by allow
RL2=$(c -X POST $A/api/rate-limits -d '{"name":"itest-rl2","requests_per_second":2,"burst":0}' | js "d['rate_limit']['id']")
put_host ",\"block_exploits\":true,\"rate_limit_id\":$RL2" >/dev/null
codes=$(for i in $(seq 1 15); do code "$G/itest-x/$i"; echo; done | sort | uniq -c | tr '\n' ' ')
case "$codes" in *429*) bad "allowed requests rate limited: $codes";; *) ok "allow skips rate limiting: $codes";; esac
codes=$(for i in $(seq 1 15); do code "$G/rl-other-$i"; echo; done | sort | uniq -c | tr '\n' ' ')
case "$codes" in *429*) ok "others still rate limited: $codes";; *) bad "rate limit gone: $codes";; esac
put_host '' >/dev/null
c -o /dev/null -X DELETE $A/api/rate-limits/$RL2
# validation, events, hits
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/security-rules -d "$(cond itest-bad block '' '{"field":"http.path","operator":"matches","value":["(a)\\1"]}')")" 422 "invalid regex rejected"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/security-rules -d "$(cond itest-bad block '' '{"field":"ip.src","operator":"in","value":["10.0.0.0/40"]}')")" 422 "invalid CIDR rejected"
sleep 3
chk "$(c "$A/api/security-rules/events?limit=200" | js "sorted({e['action'] for e in d if e['rule'].startswith('itest')})")" "['allow', 'block', 'log']" "rule events recorded (block/allow/log)"
chk "$(c $A/api/security-rules | js "[r['hits_24h']>0 for r in d if r['id']==$B1][0]")" True "hit counter"
chk "$(c "$A/api/traffic/events?limit=200" | js "'blocked_rule' in {e['kind'] for e in d}")" True "blocked_rule traffic event"
for id in $(c $A/api/security-rules | js "' '.join(str(r['id']) for r in d if r['name'].startswith('itest'))"); do c -o /dev/null -X DELETE $A/api/security-rules/$id; done
c -o /dev/null -X DELETE $A/api/ip-lists/$L
put_host '' >/dev/null
chk "$(c -o /dev/null -w '%{http_code}' -X DELETE $A/api/rate-limits/$RL)" 200 "delete rate limit"

echo "[request body inspection]"
chk "$(put_host ',"body_inspection":"block"')" 200 "enable body inspection (block)"
post() { curl -s -o /dev/null -w '%{http_code}' -X POST "$@"; }
chk "$(post -H 'Next-Action: x' -F '0={"then":"$1:__proto__:then","status":"resolved_model"}' -F '1="$@0"' "$G/rsc")" 403 "React2Shell multipart payload blocked"
chk "$(post -H 'Content-Type: application/json' --data-binary '{"a":"$@0:__proto__:then"}' "$G/api")" 403 "JSON unicode-escape evasion decoded and blocked"
chk "$(post -H 'Content-Type: application/json' --data-binary '{"user":{"__proto__":{"isAdmin":true}}}' "$G/api")" 403 "prototype pollution blocked"
chk "$(post --data-binary 'a=O%253A8%253A%2522stdClass%2522%253A0%253A%257B%257D' "$G/form")" 403 "double-encoded PHP object injection blocked"
chk "$(post -F 'PHP_SESSION_UPLOAD_PROGRESS=x' -F 'f=@/dev/null' "$G/upload.php")" 403 "PHP upload-progress field (php2shell vector) blocked"
chk "$(post -H 'Content-Type: application/json' -H 'Content-Encoding: gzip' --data-binary 'xxxx' "$G/api")" 403 "compressed request body refused in block mode"
chk "$(post -H 'Content-Type: application/json' --data-binary '{"handle":{"__class":"GuzzleHttp\\Psr7\\FnStream"}}' "$G/api")" 403 "Craft CMS __class / gadget class blocked"
chk "$(post -H 'Content-Type: application/json' --data-binary '{"name":"hello","items":[1,2],"note":"한글 본문 constructor 테스트"}' "$G/api")" 200 "normal JSON passes (no false positive)"
printf '{"__proto__":1}' > /tmp/corvus-itest-file.json
chk "$(post -F 'title=report' -F 'file=@/tmp/corvus-itest-file.json' "$G/upload")" 200 "uploaded file contents are not inspected"
chk "$(curl -s -o /dev/null -w '%{http_code}' "$G/plain-get")" 200 "requests without a body are unaffected"
chk "$(put_host ',"body_inspection":"block","body_max_kb":16')" 200 "body limit 16 KiB"
{ printf '{"pad":"'; head -c 40000 /dev/zero | tr '\0' 'a'; printf '","__proto__":{}}'; } > /tmp/corvus-itest-big.json
chk "$(post -H 'Content-Type: application/json' --data-binary @/tmp/corvus-itest-big.json "$G/api")" 200 "body above the limit is passed (not inspected)"
chk "$(put_host ',"body_inspection":"log"')" 200 "body inspection: log only"
chk "$(post -H 'Content-Type: application/json' --data-binary '{"x":{"__proto__":{}}}' "$G/api-log")" 200 "log mode does not block"
BR=$(c -X POST $A/api/security-rules -d '{"name":"itest-body-rule","action":"block","expression":{"groups":[[{"field":"http.body","operator":"contains","value":["evil-marker"],"ci":true}]]}}' | js "d['rule']['id']")
chk "$(post -H 'Content-Type: application/json' --data-binary '{"q":"EVIL-MARKER"}' "$G/api")" 403 "security rule on request body"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/security-rules -d '{"name":"itest-bad","action":"block","expression":{"groups":[[{"field":"http.body","operator":"eq","value":["x"]}]]}}')" 422 "body field only accepts contains/matches operators"
sleep 3
chk "$(c "$A/api/security-rules/events?limit=200" | js "sorted({(e['rule'], e['action']) for e in d if e['rule'].startswith('body:')} & {('body:react2shell','block'),('body:proto-pollution','log')})")" "[('body:proto-pollution', 'log'), ('body:react2shell', 'block')]" "body events recorded (block and log)"
c -o /dev/null -X DELETE $A/api/security-rules/$BR
put_host '' >/dev/null

echo "[IP bans]"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/bans -d '{"ip":"10.0.0.0/8"}')" 422 "networks are rejected for bans (use security rules)"
BID=$(c -X POST $A/api/bans -d "{\"ip\":\"$ip\",\"duration_s\":600,\"reason\":\"itest\"}" | js "d['id']")
sleep 2
chk "$(code "$G/ban-check")" 403 "manually banned IP refused by the gateway"
curl -s "$G/ban-check" | grep -q "Request ID" && ok "ban uses the block page" || bad "ban page"
chk "$(c -o /dev/null -w '%{http_code}' -X DELETE $A/api/bans/$BID)" 200 "unban"
sleep 2
chk "$(code "$G/ban-check")" 200 "unbanned IP allowed again"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/autoban -d '{"enabled":true,"threshold":3,"window_s":60,"duration_s":120,"include_scans":false,"scan_threshold":200,"exempt":[]}')" 200 "auto ban: threshold 3 in 1 min"
for i in 1 2 3 4 5; do code "$G/.git/config?n=$i" >/dev/null; done
banned=""
for i in $(seq 1 12); do sleep 2; if [ "$(code "$G/after-attack")" = 403 ]; then banned=1; break; fi; done
[ -n "$banned" ] && ok "repeated attacks → automatic ban" || bad "no automatic ban"
chk "$(c $A/api/bans | js "[b['source'] for b in d if b['ip']=='$ip']")" "['auto']" "automatic ban listed"
ABID=$(c $A/api/bans | js "[b['id'] for b in d if b['ip']=='$ip'][0]")
c -o /dev/null -X PUT $A/api/settings/autoban -d '{"enabled":false,"threshold":20,"window_s":600,"duration_s":900,"include_scans":false,"scan_threshold":200,"exempt":[]}'
c -o /dev/null -X DELETE $A/api/bans/$ABID
sleep 2
chk "$(code "$G/after-unban")" 200 "automatic ban lifted"

echo "[bad behaviour]"
# the demo origin answers 200 everywhere: a rule answering 404 stands in for missing paths
BBR=$(rule "$(cond itest-bb-404 block ',"status_code":404' '{"field":"http.path","operator":"starts_with","value":["/itest-bb-missing"]}')")
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/autoban -d '{"enabled":true,"threshold":20,"window_s":600,"duration_s":120,"include_scans":false,"scan_threshold":200,"exempt":[],"bad_behaviour":true,"bad_behaviour_codes":[404],"bad_behaviour_threshold":5,"bad_behaviour_window_s":60}')" 200 "bad behaviour: five 404s in 60 s"
sleep 3
chk "$(code "$G/bb-before")" 200 "client allowed before misbehaving"
codes=$(for i in 1 2 3 4 5 6; do code "$G/itest-bb-missing/$i"; echo; done | tr '\n' ' ')
chk "$codes" "404 404 404 404 404 403 " "request after five 404s refused immediately"
chk "$(code "$G/bb-after")" 403 "banned by the gateway without waiting for corvus-control"
listed=""; r=""
for i in $(seq 1 15); do sleep 2; r=$(c $A/api/bans | js "[b['reason'] for b in d if b['ip']=='$ip' and b['source']=='auto']"); case "$r" in *"bad behaviour: 5×404 on "*) listed=1; break;; esac; done
[ -n "$listed" ] && ok "bad-behaviour ban recorded by corvus-control ($r)" || bad "bad-behaviour ban not listed (got $r)"
sleep 2
chk "$(code "$G/bb-after-push")" 403 "still banned after the authoritative push"
c -o /dev/null -X PUT $A/api/settings/autoban -d '{"enabled":false,"threshold":20,"window_s":600,"duration_s":900,"include_scans":false,"scan_threshold":200,"exempt":[],"bad_behaviour":false}'
for id in $(c $A/api/bans | js "' '.join(str(b['id']) for b in d if b['ip']=='$ip')"); do c -o /dev/null -X DELETE $A/api/bans/$id; done
c -o /dev/null -X DELETE $A/api/security-rules/$BBR
sleep 2
chk "$(code "$G/bb-after-unban")" 200 "bad-behaviour ban lifted"

echo "[challenge]"
field() { sed -n "s/.*name=\"$1\" value=\"\([^\"]*\)\".*/\1/p" | head -1; }
status() { head -1 | cut -d' ' -f2; }
verify() { curl -s -o /dev/null -D - -c "$1" -b "$1" --data-urlencode "t=$2" --data-urlencode "c=$3" --data-urlencode "r=$4" "$G/__corvus/challenge/verify"; }
CHC=$(rule "$(cond itest-ch-cookie challenge ',"challenge_mode":"cookie"' '{"field":"http.path","operator":"starts_with","value":["/itest-ch-cookie"]}')")
CHB=$(rule "$(cond itest-ch-block block '' '{"field":"http.path","operator":"eq","value":["/itest-ch-cookie/blocked"]}')")
chk "$(c $A/api/security-rules | js "[(r['action'], r['challenge_mode'], r['challenge_ttl_s'], r['challenge_bind']) for r in d if r['id']==$CHC][0]")" "('challenge', 'cookie', 86400, 'ip')" "challenge rule stored with defaults"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/security-rules -d "$(cond itest-bad challenge ',"challenge_difficulty":30' '{"field":"http.path","operator":"eq","value":["/x"]}')")" 422 "challenge difficulty above 24 rejected"
CJ=$(mktemp)
R=$(curl -s -D - -c "$CJ" -b "$CJ" "$G/itest-ch-cookie/page?x=1")
chk "$(echo "$R" | status)" 403 "cookie challenge: first request gets the challenge page"
echo "$R" | grep -qi '^cache-control: no-store' && ok "challenge page is not cacheable" || bad "challenge page without Cache-Control: no-store"
echo "$R" | grep -qi '^x-corvus-challenge: cookie' && ok "challenge mode header" || bad "no X-Corvus-Challenge header"
TOK=$(echo "$R" | field t)
chk "$(echo "$R" | field r)" "/itest-ch-cookie/page?x=1" "return path in the challenge form"
chk "$(code -X POST "$G/itest-ch-cookie/form")" 403 "POST without a pass is challenged too"
chk "$(code "$G/__corvus/challenge/verify")" 405 "verify endpoint accepts POST only"
chk "$(verify "$CJ" "${TOK}x" "" /itest-ch-cookie/page | status)" 403 "tampered challenge token refused"
V=$(verify "$CJ" "$TOK" "" "/itest-ch-cookie/page?x=1")
chk "$(echo "$V" | status)" 303 "cookie challenge: answer accepted (303)"
echo "$V" | grep -qi '^location: .*/itest-ch-cookie/page?x=1' && ok "redirected back to the original URL" || bad "redirect target: $(echo "$V" | grep -i '^location')"
SC=$(echo "$V" | grep -i '^set-cookie: __corvus_ch=')
case "$SC" in *HttpOnly*SameSite=Lax*) ok "pass cookie is HttpOnly, SameSite=Lax";; *) bad "pass cookie flags: $SC";; esac
chk "$(curl -s -o /dev/null -w '%{http_code}' -b "$CJ" "$G/itest-ch-cookie/page?x=1")" 200 "cookie challenge: request with the pass reaches the backend"
chk "$(curl -s -o /dev/null -w '%{http_code}' -b "$CJ" "$G/itest-ch-cookie/blocked")" 403 "block rules still apply with a pass"
curl -s -b "$CJ" "$G/itest-ch-cookie/blocked" | grep -q 'name="t"' && bad "block rule answered with a challenge" || ok "blocked with the block page"
chk "$(code "$G/itest-ch-cookie/page")" 403 "without the cookie: challenged again"
V=$(verify "$CJ" "$TOK" "" "//evil.example/x")
echo "$V" | grep -i '^location:' | grep -qi 'evil' && bad "open redirect: $(echo "$V" | grep -i '^location')" || ok "network-path return target replaced by /"
# js mode: compute the proof of work here (difficulty 12 ≈ 4k hashes)
CHJ=$(rule "$(cond itest-ch-js challenge ',"challenge_mode":"js","challenge_difficulty":12,"challenge_ttl_s":300' '{"field":"http.path","operator":"starts_with","value":["/itest-ch-js"]}')")
JJ=$(mktemp)
P=$(curl -s -c "$JJ" -b "$JJ" "$G/itest-ch-js/x")
echo "$P" | grep -q 'data-mode="js" data-bits="12"' && ok "js challenge page with difficulty 12" || bad "js challenge page"
chk "$(curl -s -o /dev/null -w '%{http_code}' -b "$CJ" "$G/itest-ch-js/x")" 403 "a cookie-check pass does not satisfy a js challenge"
TOK=$(echo "$P" | field t)
chk "$(verify "$JJ" "$TOK" 0x /itest-ch-js/x | status)" 403 "js challenge: invalid proof of work refused"
CNT=$($PY -c '
import hashlib, sys
nonce, bits, i = sys.argv[1].split(".")[8], int(sys.argv[2]), 0
while int.from_bytes(hashlib.sha256((nonce + str(i)).encode()).digest(), "big") >> (256 - bits):
    i += 1
print(i)' "$TOK" 12)
chk "$(verify "$JJ" "$TOK" "$CNT" /itest-ch-js/x | status)" 303 "js challenge: proof of work accepted (counter $CNT)"
chk "$(curl -s -o /dev/null -w '%{http_code}' -b "$JJ" "$G/itest-ch-js/x")" 200 "js challenge: pass reaches the backend"
chk "$(curl -s -o /dev/null -w '%{http_code}' -b "$JJ" "$G/itest-ch-cookie/page")" 200 "a js pass also satisfies a cookie challenge"
# challenge pages never count as bad behaviour
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/autoban -d '{"enabled":true,"threshold":20,"window_s":600,"duration_s":120,"include_scans":false,"scan_threshold":200,"exempt":[],"bad_behaviour":true,"bad_behaviour_codes":[403],"bad_behaviour_threshold":3,"bad_behaviour_window_s":60}')" 200 "bad behaviour: three 403s in 60 s"
sleep 3
codes=$(for i in 1 2 3 4 5; do code "$G/itest-ch-cookie/bb-$i"; echo; done | tr '\n' ' ')
chk "$codes" "403 403 403 403 403 " "challenge pages served repeatedly"
chk "$(code "$G/ch-not-banned")" 200 "challenge pages do not count toward bad-behaviour bans"
c -o /dev/null -X PUT $A/api/settings/autoban -d '{"enabled":false,"threshold":20,"window_s":600,"duration_s":900,"include_scans":false,"scan_threshold":200,"exempt":[],"bad_behaviour":false}'
for id in $(c $A/api/bans | js "' '.join(str(b['id']) for b in d if b['ip']=='$ip')"); do c -o /dev/null -X DELETE $A/api/bans/$id; done
sleep 3
chk "$(c "$A/api/security-rules/events?limit=500" | js "sorted({e['action'] for e in d if e['rule_id'] in ($CHC, $CHJ)})")" "['challenge', 'challenge_fail', 'challenge_pass']" "challenge events recorded (issued / passed / failed)"
chk "$(c "$A/api/traffic/events?limit=500" | js "sorted({e['kind'] for e in d if e['kind'].startswith('challenge')})")" "['challenge_failed', 'challenge_issued', 'challenge_passed']" "challenge traffic events"
rm -f "$CJ" "$JJ"
for id in $CHC $CHB $CHJ; do c -o /dev/null -X DELETE $A/api/security-rules/$id; done

echo "[under attack]"
chk "$(put_host ',"under_attack":"sometimes"')" 422 "invalid under attack mode rejected"
chk "$(put_host ',"under_attack":"on","under_attack_config":{"mode":"cookie","ttl_s":600}')" 200 "under attack mode on (cookie check)"
chk "$(c $A/api/proxy-hosts/$HOST1 | js "(d['under_attack'], d['under_attack_config']['mode'], d['under_attack_config']['difficulty'], d['under_attack_config']['ttl_s'], d['under_attack_config']['bind'], d['under_attack_config']['rps'], d['under_attack_config']['duration_s'])")" "('on', 'cookie', 16, 600, 'ip', 200, 900)" "under attack settings stored with defaults"
chk "$(put_host '')" 200 "host saved without under attack fields"
chk "$(c $A/api/proxy-hosts/$HOST1 | js "d['under_attack']")" on "omitted under attack fields keep the mode"
chk "$(c $A/api/under-attack | js "[(s['mode'], s['active']) for s in d if s['host_id']==$HOST1]")" "[('on', True)]" "host listed as under attack"
UAA=$(rule "$(cond itest-ua-allow allow '' '{"field":"http.path","operator":"starts_with","value":["/itest-ua-allowed"]}')")
UAB=$(rule "$(cond itest-ua-block block '' '{"field":"http.path","operator":"starts_with","value":["/itest-ua-blocked"]}')")
UJ=$(mktemp)
R=$(curl -s -D - -c "$UJ" -b "$UJ" "$G/itest-ua/page?x=1")
chk "$(echo "$R" | status)" 403 "under attack: request without a pass gets the challenge"
echo "$R" | grep -qi '^x-corvus-challenge: cookie' && ok "under attack: cookie challenge page" || bad "under attack: no X-Corvus-Challenge header"
TOK=$(echo "$R" | field t)
chk "$(echo "$TOK" | cut -d. -f3)" 0 "under attack: challenge token names virtual rule 0"
chk "$(code "$G/")" 403 "under attack: site root challenged too"
chk "$(code "$G/itest-ua-allowed/x")" 200 "under attack: requests an allow rule matched are not challenged"
chk "$(code "$G/itest-ua-blocked/x")" 403 "under attack: block rule still answers"
curl -s "$G/itest-ua-blocked/x" | grep -q 'name="t"' && bad "block rule answered with a challenge" || ok "under attack: block rules come first (block page)"
A1=$(curl -s -D - -o /dev/null "$G/.well-known/acme-challenge/itest-ua" | tr -d '\r')
chk "$(echo "$A1" | status)" 404 "under attack: ACME path reaches the ACME responder"
echo "$A1" | grep -qi '^x-corvus-challenge:' && bad "ACME path challenged" || ok "under attack: ACME path not challenged"
V=$(verify "$UJ" "$TOK" "" "/itest-ua/page?x=1")
chk "$(echo "$V" | status)" 303 "under attack: answer accepted (303)"
echo "$V" | grep -qi '^location: .*/itest-ua/page?x=1' && ok "under attack: redirected back" || bad "under attack redirect: $(echo "$V" | grep -i '^location')"
chk "$(curl -s -o /dev/null -w '%{http_code}' -b "$UJ" "$G/itest-ua/page?x=1")" 200 "under attack: request with the pass reaches the backend"
chk "$(curl -s -o /dev/null -w '%{http_code}' -b "$UJ" "$G/itest-ua/other")" 200 "under attack: the pass covers the whole host"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/proxy-hosts/$HOST1/under-attack/deactivate)" 409 "deactivate: nothing automatic to end"
sleep 3
chk "$(c "$A/api/traffic/events?limit=500" | js "sorted({e['kind'] for e in d if (e['kind']=='challenge_issued' and e['path'].startswith('/itest-ua')) or (e['kind']=='challenge_passed' and e['path']=='/__corvus/challenge/verify')})")" "['challenge_issued', 'challenge_passed']" "under attack challenges recorded as challenge events (passes are logged on the verify path)"
chk "$(put_host ',"under_attack":"auto"')" 200 "under attack mode auto"
chk "$(c $A/api/under-attack | js "[(s['mode'], s['active']) for s in d if s['host_id']==$HOST1]")" "[('auto', False)]" "auto: not active without an attack"
chk "$(code "$G/itest-ua/auto")" 200 "auto: not active, no challenge"
chk "$(put_host ',"under_attack":"off"')" 200 "under attack mode off"
chk "$(code "$G/itest-ua/page")" 200 "under attack off: no challenge"
chk "$(c $A/api/under-attack | js "[s for s in d if s['host_id']==$HOST1]")" "[]" "host no longer listed"
rm -f "$UJ"
for id in $UAA $UAB; do c -o /dev/null -X DELETE $A/api/security-rules/$id; done

echo "[admin console allowlist]"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/network -d '{"trusted_proxies":[],"admin_allow":["10.255.255.0/24"]}')" 422 "allowlist without my address refused (lock-out guard)"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/network -d "{\"trusted_proxies\":[],\"admin_allow\":[\"$ip/32\"]}")" 200 "allowlist with my address saved"
chk "$(c -o /dev/null -w '%{http_code}' $A/api/auth/me)" 200 "console still reachable from the allowed address"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/network -d '{"trusted_proxies":[],"admin_allow":[]}')" 200 "allowlist cleared"

echo "[virtual-patch templates & managed rules]"
chk "$(c $A/api/rule-templates | js "len(d)>=10 and all(t['title'].get('en') for t in d)")" True "templates listed"
TL=$(c -X POST $A/api/ip-lists -d "{\"name\":\"itest-admins\",\"items\":[\"$ip\"]}" | js "d['ip_list']['id']")
TR=$(c -X POST $A/api/security-rules/from-template -d "{\"template_id\":\"wp2shell\",\"action\":\"block\",\"scope\":\"all\",\"exempt_list_id\":$TL}" | js "d['rule']['id']")
chk "$(c $A/api/security-rules | js "[r['expression']['groups'][0][1]['value'] for r in d if r['id']==$TR][0]")" "['list:$TL']" "template exemption bound to the IP list"
chk "$(code "$G/wp-json/batch/v1")" 200 "exempt admin network passes the virtual patch"
c -o /dev/null -X PUT $A/api/ip-lists/$TL -d '{"name":"itest-admins","items":["10.9.9.9"]}'
chk "$(code "$G/wp-json/batch/v1")" 403 "virtual patch blocks everyone else"
chk "$(code "$G/?rest_route=%2Fbatch%2Fv1")" 403 "virtual patch covers the rest_route variant"
c -o /dev/null -X DELETE $A/api/security-rules/$TR; c -o /dev/null -X DELETE $A/api/ip-lists/$TL
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/security-rules/from-template -d '{"template_id":"nope"}')" 422 "unknown template rejected"
chk "$(c $A/api/managed-rules | js "d['signatures']>=8 and d['templates']>=10")" True "managed rule bundle status"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/managed-rules/upload -d '{"bundle":"{\"serial\":99999999999,\"version\":\"evil\"}","signature":"AAAA"}')" 422 "unsigned bundle rejected"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/managed-rules -d '{"url":"http://insecure.example/bundle.json","auto_update":true}')" 422 "update URL must be https"

echo "[data retention]"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/retention -d '{"log_mode":"notable","log_days":0,"log_max_mb":1024,"stats_days":30,"events_days":30}')" 422 "invalid retention rejected"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/retention -d '{"log_mode":"notable","log_days":14,"log_max_mb":1024,"stats_days":30,"events_days":30}')" 200 "retention saved"
code "$G/.git/retention-check" >/dev/null; sleep 4
TODAY=$(date -u +%Y-%m-%d)
chk "$(c $A/api/traffic/archives | js "any(a['name']=='access-$TODAY.jsonl' for a in d)")" True "today's request log file listed"
found=""; for i in 1 2 3 4 5 6; do c "$A/api/traffic/archives/access-$TODAY.jsonl" | grep 'retention-check' >/dev/null && { found=1; break; }; sleep 2; done
[ -n "$found" ] && ok "request log download contains the notable request" || bad "request log content"
chk "$(c -o /dev/null -w '%{http_code}' "$A/api/traffic/archives/..%2Fcorvus.db")" 404 "archive path traversal refused"
chk "$(c $A/api/settings/retention | js "d['usage']['db_bytes']>0")" True "storage usage reported"

echo "[traffic API]"
sleep 3
s=$(c "$A/api/traffic/summary?range=15m")
echo "$s" | js "('requests' in d and d['requests']>0 and d['p95']>0)" | grep -q True && ok "summary: $(echo "$s" | js "'req=%d 4xx=%d blocked=%d p95=%sms ips=%d' % (d['requests'],d['s4xx'],d['blocked'],d['p95'],d['unique_ips'])")" || bad "summary $s"
chk "$(c "$A/api/traffic/timeseries?range=5m" | js "d['step_s']==10 and len(d['points'])==30")" True "timeseries 5m = 30 x 10s points"
chk "$(c "$A/api/traffic/hosts?range=15m" | js "any(h['host']=='$H' for h in d)")" True "per-host stats"
chk "$(c "$A/api/traffic/top-paths?range=15m&limit=5" | js "len(d)>0")" True "top paths"
chk "$(c "$A/api/traffic/events?limit=200" | js "sorted({e['kind'] for e in d} & {'rate_limited','rate_limit_dry_run','blocked_exploit','exploit_detected'})==['blocked_exploit','exploit_detected','rate_limit_dry_run','rate_limited']")" True "events: rate_limited, dry run, exploit blocked/detected"
chk "$(c "$A/api/traffic/logs?status=403&format=csv&limit=5" | head -1 | cut -d, -f1-5)" "time,host,method,path,status" "CSV export header"

echo "[MFA]"
MS=$(c -X POST $A/api/auth/mfa/setup); SECRET=$(echo "$MS" | js "d['secret']")
echo "$MS" | js "d['qr_png'].startswith('data:image/png;base64,')" | grep -q True && ok "QR code returned" || bad "no QR"
totp() { $PY - "$SECRET" "${1:-0}" <<'PY'
import sys,hmac,hashlib,struct,time,base64
s=base64.b32decode(sys.argv[1]+'='*(-len(sys.argv[1])%8)); c=int(time.time())//30+int(sys.argv[2])
h=hmac.new(s,struct.pack('>Q',c),hashlib.sha1).digest(); o=h[-1]&15
print('%06d'%((struct.unpack('>I',h[o:o+4])[0]&0x7fffffff)%1000000))
PY
}
# start inside a fresh 30 s TOTP window so the enable code and the replay check share one step
[ $(( $(date +%s) % 30 )) -gt 20 ] && sleep $((31 - $(date +%s) % 30))
REC=$(c -X POST $A/api/auth/mfa/enable -d "{\"code\":\"$(totp)\"}" | js "d['recovery_codes'][0]")
[ -n "$REC" ] && ok "MFA enabled, recovery codes issued" || bad "enable failed"
c -o /dev/null -X POST $A/api/auth/logout
R=$(c -X POST $A/api/auth/login -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")
chk "$(echo "$R" | js "d.get('mfa_required')")" True "password alone no longer signs in"
MT=$(echo "$R" | js "d['mfa_token']")
chk "$(c -o /dev/null -w '%{http_code}' $A/api/auth/me)" 401 "no session before second factor"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/auth/login/mfa -d "{\"mfa_token\":\"$MT\",\"code\":\"$(totp)\"}")" 401 "same TOTP code cannot be replayed"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/auth/login/mfa -d "{\"mfa_token\":\"$MT\",\"code\":\"$REC\"}")" 200 "recovery code signs in"
c -o /dev/null -X POST $A/api/auth/logout
MT=$(c -X POST $A/api/auth/login -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" | js "d['mfa_token']")
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/auth/login/mfa -d "{\"mfa_token\":\"$MT\",\"code\":\"$REC\"}")" 401 "recovery code is single use"
echo "  ...waiting for the next TOTP window"; sleep $((31 - $(date +%s) % 30))
MT=$(c -X POST $A/api/auth/login -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}" | js "d['mfa_token']")
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/auth/login/mfa -d "{\"mfa_token\":\"$MT\",\"code\":\"$(totp)\"}")" 200 "fresh TOTP code signs in"
sleep $((31 - $(date +%s) % 30))
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/auth/mfa/disable -d "{\"password\":\"$PASS\",\"code\":\"$(totp)\"}")" 200 "MFA disabled"

echo "[security headers]"
# The demo backend cannot set cookies, so a second host plays the origin: its advanced
# config answers /itest-cookies with Set-Cookie, X-Frame-Options and X-Powered-By, and the
# host under test proxies to it through the gateway itself.
hdr_host() { $PY -c 'import json,sys; print(json.dumps({"domains":[sys.argv[1]],"forward_host":"demo","forward_port":80,"block_exploits":True,"security_headers":sys.argv[2],"advanced_config":sys.argv[3]}))' "$@"; }
ORIGIN_CONF='location = /itest-cookies {
    add_header Set-Cookie "sid=abc; Path=/" always;
    add_header Set-Cookie "pref=1; Path=/; secure; HttpOnly; SameSite=Strict" always;
    add_header X-Frame-Options DENY always;
    add_header X-Powered-By itest always;
    return 200 "ok";
}'
FRONT_CONF='location / {
    proxy_set_header Host hdr-origin.corvus.test;
    proxy_pass http://127.0.0.1:80;
}'
hdrs() { curl -s -D - -o /dev/null -H "Host: $1" "$G$2" | tr -d '\r'; }
has()  { echo "$1" | grep -ciE "$2"; }
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/proxy-hosts -d "$(hdr_host hdr-origin.corvus.test off "$ORIGIN_CONF")")" 201 "origin host (security headers off)"
HID=$(c -X POST $A/api/proxy-hosts -d "$(hdr_host hdr-front.corvus.test strict "$FRONT_CONF")" | js "d['proxy_host']['id']")
R=$(hdrs hdr-origin.corvus.test /itest-cookies)
chk "$(has "$R" '^x-content-type-options:')/$(has "$R" '^x-powered-by: itest')" "0/1" "off: responses unchanged"
R=$(hdrs hdr-front.corvus.test /itest-cookies)
chk "$(has "$R" '^x-content-type-options: nosniff$')/$(has "$R" '^referrer-policy: strict-origin-when-cross-origin$')/$(has "$R" '^permissions-policy:')" "1/1/1" "strict: basic headers added"
chk "$(has "$R" '^x-frame-options:')/$(has "$R" '^x-frame-options: DENY$')" "1/1" "strict: upstream X-Frame-Options kept, not duplicated"
chk "$(has "$R" '^x-powered-by:')" 0 "strict: X-Powered-By hidden"
chk "$(has "$R" "^content-security-policy-report-only: default-src 'self'")/$(has "$R" '^cross-origin-opener-policy: same-origin$')" "1/1" "strict: report-only CSP and COOP"
chk "$(has "$R" '^set-cookie: sid=abc; Path=/; HttpOnly; SameSite=Lax$')" 1 "strict: missing cookie flags added (HTTP: no Secure)"
chk "$(has "$R" '^set-cookie: pref=1; Path=/; secure; HttpOnly; SameSite=Strict$')" 1 "strict: cookie with its own flags untouched"
R=$(curl -s -D - -o /dev/null -H 'Host: hdr-front.corvus.test' "$G/.env" | tr -d '\r')
chk "$(echo "$R" | head -1 | cut -d' ' -f2)/$(has "$R" '^x-content-type-options: nosniff$')" "403/1" "headers on gateway error responses"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/proxy-hosts/$HID -d "$(hdr_host hdr-front.corvus.test basic "$FRONT_CONF")")" 200 "switch to basic"
R=$(hdrs hdr-front.corvus.test /itest-cookies)
chk "$(has "$R" '^x-content-type-options: nosniff$')/$(has "$R" '^content-security-policy-report-only:')/$(has "$R" '^set-cookie: sid=abc; Path=/$')" "1/0/1" "basic: no CSP, cookies untouched"
c -o /dev/null -X DELETE $A/api/proxy-hosts/$HID
for id in $(c $A/api/proxy-hosts | js "' '.join(str(h['id']) for h in d if 'hdr-origin.corvus.test' in h['domains'])"); do c -o /dev/null -X DELETE $A/api/proxy-hosts/$id; done

echo "[ACME (Pebble test CA)]"
CID=$(c -X POST $A/api/certificates/acme -d '{"domains":["acme1.corvus.test"],"email":"ops@corvus.test","challenge":"http-01"}' | js "d['id']")
st=pending; for i in $(seq 1 40); do st=$(c $A/api/certificates | js "[x['status'] for x in d if x['id']==$CID][0]"); [ "$st" != pending ] && break; sleep 2; done
chk "$st" valid "ACME certificate issued"
[ "$st" != valid ] && c $A/api/certificates | js "[x['last_error'] for x in d if x['id']==$CID][0]"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/proxy-hosts -d "{\"domains\":[\"acme1.corvus.test\"],\"forward_host\":\"demo\",\"forward_port\":80,\"certificate_id\":$CID}")" 201 "host with ACME certificate"
issuer=$(echo | openssl s_client -connect $H:8443 -servername acme1.corvus.test 2>/dev/null | openssl x509 -noout -issuer 2>/dev/null)
case "$issuer" in *ebble*) ok "HTTPS served with ACME certificate ($issuer)";; *) bad "issuer: $issuer";; esac
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/certificates/acme -d '{"domains":["*.corvus.test"],"email":"ops@corvus.test","challenge":"http-01"}')" 422 "wildcard requires DNS-01"

echo "[host with new certificate request (NPM-style)]"
TPL=$(c $A/api/acme/dns-providers | js "d[0]['credentials_template']")
case "$TPL" in *dns_cloudflare_api_token=*) ok "Cloudflare credentials template served";; *) bad "template: $TPL";; esac
body() { $PY -c "import json,sys; print(json.dumps({'domains':[sys.argv[1]],'forward_host':'demo','forward_port':80,'ssl_forced':True,'certificate_request':json.loads(sys.argv[2])}))" "$1" "$2"; }
R=$(c -X POST $A/api/proxy-hosts -d "$(body dns1.corvus.test "{\"email\":\"ops@corvus.test\",\"dns_challenge\":true,\"dns_provider\":\"cloudflare\",\"dns_credentials\":$($PY -c 'import json,sys; print(json.dumps(sys.argv[1]))' "$TPL")}")")
chk "$(echo "$R" | js "[f['field'] for f in d.get('fields',[])]")" "['certificate_request.dns_credentials']" "unchanged template token rejected"
R=$(c -X POST $A/api/proxy-hosts -d "$(body '*.wild.corvus.test' '{"email":"ops@corvus.test","dns_challenge":false}')")
chk "$(echo "$R" | js "[f['code'] for f in d.get('fields',[])]")" "['wildcard_needs_dns']" "wildcard without DNS challenge rejected"
R=$(c -X POST $A/api/proxy-hosts -d "$(body inline1.corvus.test '{"email":"ops@corvus.test","dns_challenge":false}')")
CID=$(echo "$R" | js "d['certificate']['id']"); chk "$(echo "$R" | js "d['certificate']['status']")" pending "host saved with certificate pending"
chk "$(code -H 'Host: inline1.corvus.test' "$G/")" 200 "serves HTTP while the certificate is pending"
st=pending; for i in $(seq 1 40); do st=$(c $A/api/certificates | js "[x['status'] for x in d if x['id']==$CID][0]"); [ "$st" != pending ] && break; sleep 2; done
chk "$st" valid "certificate issued in the background"
sleep 2
chk "$(code -H 'Host: inline1.corvus.test' "$G/x")" 301 "HTTPS redirect activated after issuance"
chk "$(curl -sk -o /dev/null -w '%{http_code}' --resolve inline1.corvus.test:8443:$H https://inline1.corvus.test:8443/)" 200 "HTTPS with issued certificate"
chk "$(c $A/api/certificates | js "[x['auto_renew'] for x in d if x['id']==$CID][0]")" True "auto renew enabled"
CREQ=$($PY -c 'import json; print(json.dumps({"email":"ops@corvus.test","dns_challenge":True,"dns_provider":"cloudflare","dns_credentials":"# Cloudflare API token" + chr(10) + "dns_cloudflare_api_token=invalid-test-token"}))')
R=$(c -X POST $A/api/proxy-hosts -d "$(body dns2.corvus.test "$CREQ")")
CID=$(echo "$R" | js "d['certificate']['id']")
st=pending; for i in $(seq 1 60); do st=$(c $A/api/certificates | js "[x['status'] for x in d if x['id']==$CID][0]"); [ "$st" != pending ] && break; sleep 2; done
chk "$st" error "invalid Cloudflare token ends in error (not stuck pending)"
echo "  last_error: $(c $A/api/certificates | js "[x['last_error'] for x in d if x['id']==$CID][0][:140]")"
chk "$(code -H 'Host: dns2.corvus.test' "$G/")" 200 "host still serves HTTP after a failed order"

echo "[alerts]"
CH=$(c -X POST $A/api/alerts/channels -d '{"name":"itest-hook","kind":"webhook","url":"http://demo/alert","secret":"s3cret","min_severity":"info"}' | js "d['channel']['id']")
chk "$(c -X POST $A/api/alerts/channels/$CH/test | js "d['ok']")" True "webhook test delivery"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/alerts/channels -d '{"name":"itest-meta","kind":"slack","url":"http://169.254.169.254/x"}')" 422 "metadata/loopback webhook URL rejected"
chk "$(c $A/api/alerts/channels | js "'s3cret' not in json.dumps(d) and 'demo/alert' not in json.dumps(d)")" True "channel secrets never returned by the API"
c -o /dev/null -X POST $A/api/proxy-hosts -d '{"domains":["broken.corvus.test"],"forward_host":"demo","forward_port":9999}'
for i in $(seq 1 80); do curl -s -o /dev/null -H 'Host: broken.corvus.test' "$G/e$i"; done
n=0; for i in $(seq 1 15); do n=$(c $A/api/alerts/active | js "len([a for a in d if a['rule']=='error_rate' and a['key']=='broken.corvus.test'])"); [ "$n" -ge 1 ] && break; sleep 5; done
chk "$n" 1 "5xx spike detected (error_rate alert active)"
chk "$(c "$A/api/alerts/history?limit=20" | js "any(h['rule']=='error_rate' and h['status']=='firing' and h['results'] and h['results'][0]['ok'] for h in d)")" True "alert delivered to the channel"
c -o /dev/null -X DELETE $A/api/alerts/channels/$CH

echo "[users & roles]"
OPW="Op-$(date +%s)-pass"; VPW="Vw-$(date +%s)-pass"
OID=$(c -X POST $A/api/users -d "{\"email\":\"op@corvus.test\",\"name\":\"Operator\",\"role\":\"operator\",\"password\":\"$OPW\"}" | js "d['id']")
VID=$(c -X POST $A/api/users -d "{\"email\":\"viewer@corvus.test\",\"name\":\"Viewer\",\"role\":\"viewer\",\"password\":\"$VPW\"}" | js "d['id']")
[ -n "$OID" ] && [ -n "$VID" ] && ok "operator and viewer accounts created" || bad "user creation"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/users -d '{"email":"op@corvus.test","role":"viewer","password":"another-pass-1"}')" 422 "duplicate email rejected"
JO=$(mktemp); JV=$(mktemp)
co() { curl -sk -b "$JO" -c "$JO" -H 'X-Corvus-Request: 1' -H 'Content-Type: application/json' "$@"; }
cv() { curl -sk -b "$JV" -c "$JV" -H 'X-Corvus-Request: 1' -H 'Content-Type: application/json' "$@"; }
co -o /dev/null -X POST $A/api/auth/login -d "{\"email\":\"op@corvus.test\",\"password\":\"$OPW\"}"
cv -o /dev/null -X POST $A/api/auth/login -d "{\"email\":\"viewer@corvus.test\",\"password\":\"$VPW\"}"
chk "$(cv -o /dev/null -w '%{http_code}' $A/api/proxy-hosts)" 200 "viewer can read"
chk "$(cv -o /dev/null -w '%{http_code}' -X POST $A/api/proxy-hosts -d '{"domains":["v.corvus.test"],"forward_host":"demo","forward_port":80}')" 403 "viewer cannot change configuration"
chk "$(cv -o /dev/null -w '%{http_code}' $A/api/users)" 403 "viewer cannot list users"
chk "$(co -o /dev/null -w '%{http_code}' -X POST $A/api/proxy-hosts -d '{"domains":["op.corvus.test"],"forward_host":"demo","forward_port":80}')" 201 "operator can create a proxy host"
chk "$(co -X POST $A/api/proxy-hosts -d '{"domains":["op2.corvus.test"],"forward_host":"demo","forward_port":80,"advanced_config":"client_max_body_size 1m;"}' | js "[f['field'] for f in d.get('fields',[])]")" "['advanced_config']" "operator cannot set advanced configuration"
chk "$(co -o /dev/null -w '%{http_code}' -X PUT $A/api/settings/network -d '{"trusted_proxies":[]}')" 403 "operator cannot change network settings"
chk "$(co -o /dev/null -w '%{http_code}' -X POST $A/api/users -d '{"email":"x@corvus.test","role":"admin","password":"another-pass-1"}')" 403 "operator cannot create users"
MEID=$(c $A/api/auth/me | js "d['id']")
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/users/$MEID -d '{"role":"viewer"}')" 409 "admin cannot demote themselves"
chk "$(c -o /dev/null -w '%{http_code}' -X DELETE $A/api/users/$MEID)" 409 "admin cannot delete themselves"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/users/$VID -d '{"disabled":true}')" 200 "disable viewer"
chk "$(cv -o /dev/null -w '%{http_code}' $A/api/auth/me)" 401 "disabled account's session is revoked"
chk "$(cv -o /dev/null -w '%{http_code}' -X POST $A/api/auth/login -d "{\"email\":\"viewer@corvus.test\",\"password\":\"$VPW\"}")" 401 "disabled account cannot sign in"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/users/$OID -d '{"role":"viewer"}')" 200 "demote operator to viewer"
chk "$(co -o /dev/null -w '%{http_code}' $A/api/auth/me)" 401 "role change revokes sessions"
for id in $(c $A/api/proxy-hosts | js "' '.join(str(h['id']) for h in d if any(x.endswith('.corvus.test') and x.startswith('op') for x in h['domains']))"); do c -o /dev/null -X DELETE $A/api/proxy-hosts/$id; done
chk "$(c -o /dev/null -w '%{http_code}' -X DELETE $A/api/users/$OID)" 200 "delete user"
c -o /dev/null -X DELETE $A/api/users/$VID; rm -f "$JO" "$JV"

echo "[redirection hosts]"
for id in $(c $A/api/redirection-hosts | js "' '.join(str(h['id']) for h in d if any(x.endswith('.corvus.test') for x in h['domains']))"); do c -o /dev/null -X DELETE $A/api/redirection-hosts/$id; done
RD=$(c -X POST $A/api/redirection-hosts -d '{"domains":["redir.corvus.test"],"forward_scheme":"https","forward_domain":"example.org","http_code":308,"preserve_path":true,"enabled":true}')
RDID=$(echo "$RD" | js "d.get('id') or d.get('redirection_host',{}).get('id')")
sleep 2
R=$(curl -s -D - -o /dev/null -H 'Host: redir.corvus.test' "$G/some/path?x=1" | tr -d '\r')
chk "$(echo "$R" | awk 'NR==1{print $2}')" 308 "redirection host answers with its status code"
chk "$(echo "$R" | awk -F': ' 'tolower($1)=="location"{print $2}')" "https://example.org/some/path?x=1" "redirect keeps path and query"
c -o /dev/null -X DELETE $A/api/redirection-hosts/$RDID

echo "[error pages]"
EH=$(c -X POST $A/api/proxy-hosts -d '{"domains":["err.corvus.test"],"forward_scheme":"http","forward_host":"demo","forward_port":1}' | js "d.get('id') or d.get('proxy_host',{}).get('id')")
sleep 2
EB=$(curl -s -w '\n%{http_code}' -H 'Host: err.corvus.test' "$G/")
chk "$(echo "$EB" | tail -1)" 502 "unreachable backend answers 502"
echo "$EB" | grep -q 'Temporarily unavailable' && ok "502 uses the Corvus error page" || bad "502 without the Corvus error page"
echo "$EB" | grep -q 'err.corvus.test' && ok "error page shows the host" || bad "error page without host"
c -o /dev/null -X DELETE $A/api/proxy-hosts/$EH

echo "[upstream pool]"
# A forward host whose name does not resolve (container not created yet) must not block
# deployment (resolved at run time): the host is created and answers with the error page.
NH=$(c -X POST $A/api/proxy-hosts -d '{"domains":["nores.corvus.test"],"forward_scheme":"http","forward_host":"corvus-missing-backend","forward_port":80}' | js "d.get('id') or d.get('proxy_host',{}).get('id')")
[ -n "$NH" ] && [ "$NH" != None ] && ok "host with an unresolvable forward host deploys" || bad "host with an unresolvable forward host rejected"
sleep 2
chk "$(curl -s -o /dev/null -w '%{http_code}' -H 'Host: nores.corvus.test' "$G/")" 502 "unresolvable forward host answers 502"
chk "$(curl -s -o /dev/null -w '%{http_code}' -H "Host: $H" "$G/")" 200 "other hosts unaffected"
c -o /dev/null -X DELETE $A/api/proxy-hosts/$NH
# Keep-alive pool: repeated requests reuse upstream connections (whoami shows the gateway's
# source address and port), so there are fewer distinct ports than requests.
PORTS=$(for i in $(seq 1 20); do curl -s -H "Host: $H" "$G/pool-$i" | sed -n 's/^RemoteAddr: //p'; done | sort -u | wc -l)
[ "$PORTS" -ge 1 ] && [ "$PORTS" -lt 20 ] && ok "upstream connections reused ($PORTS connections for 20 requests)" || bad "no upstream connection reuse ($PORTS connections for 20 requests)"

echo "[custom locations]"
# The host's own forward target is unreachable (demo:1); the locations route to demo:80.
# whoami echoes the request line, so the forward path is visible.
LH=$(c -X POST $A/api/proxy-hosts -d '{"domains":["loc.corvus.test"],"forward_host":"demo","forward_port":1,"block_exploits":true,"locations":[{"path":"/loc-path","forward_host":"demo","forward_port":80,"forward_path":"/path"},{"path":"/keep/","forward_host":"demo","forward_port":80}]}' | js "d.get('id') or d.get('proxy_host',{}).get('id')")
[ -n "$LH" ] && [ "$LH" != None ] && ok "host with custom locations deploys" || bad "host with custom locations rejected"
sleep 2
lget() { curl -s -H 'Host: loc.corvus.test' "$G$1"; }
lcode() { curl -s -o /dev/null -w '%{http_code}' -H 'Host: loc.corvus.test' "$@"; }
lget '/loc-path/x?a=1' | grep -q '^GET /path/x?a=1 HTTP/1.1' && ok "location forwards with its forward path (/loc-path/x -> /path/x)" || bad "forward path not applied: $(lget '/loc-path/x?a=1' | grep '^GET ')"
lget /keep/y | grep -q '^GET /keep/y HTTP/1.1' && ok "location without forward path keeps the URI" || bad "location URI changed: $(lget /keep/y | grep '^GET ')"
chk "$(lcode "$G/other")" 502 "other paths still go to the host's forward target"
chk "$(lcode "$G/loc-path/.env")" 403 "host protections apply inside a location (exploit blocking)"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/proxy-hosts/$LH -d '{"domains":["loc.corvus.test"],"forward_host":"demo","forward_port":1,"basic_auth":{"enabled":true,"users":[{"username":"locu","password":"loc-pass-123"}]}}')" 200 "enable password protection (locations kept when omitted)"
sleep 1
chk "$(lcode "$G/loc-path/x")" 401 "password required inside a location"
chk "$(lcode -u locu:loc-pass-123 "$G/loc-path/x")" 200 "correct password accepted inside a location"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/proxy-hosts/$LH -d '{"domains":["loc.corvus.test"],"forward_host":"demo","forward_port":1,"basic_auth":{"enabled":false,"users":[]},"locations":[{"path":"/","forward_host":"demo","forward_port":80}]}')" 200 "location / replaces the forward target"
sleep 1
chk "$(lcode "$G/anything")" 200 "root location answers from its upstream"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/proxy-hosts/$LH -d '{"domains":["loc.corvus.test"],"forward_host":"demo","forward_port":1,"locations":[{"path":"/__corvus/x","forward_host":"demo","forward_port":80}]}')" 422 "reserved location path rejected"
c -o /dev/null -X DELETE $A/api/proxy-hosts/$LH

echo "[streams]"
# TCP port forwarding: container port 8090 (published by deploy/dev/docker-compose.yml) → demo:80.
for id in $(c $A/api/streams | js "' '.join(str(s['id']) for s in d if s['incoming_port']==8090)"); do c -o /dev/null -X DELETE $A/api/streams/$id; done
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/streams -d '{"incoming_port":9443,"tcp_forwarding":true,"forward_host":"demo","forward_port":80}')" 422 "port used by Corvus (admin) rejected"
SID=$(c -X POST $A/api/streams -d '{"incoming_port":8090,"tcp_forwarding":true,"forward_host":"demo","forward_port":80}' | js "d.get('stream',{}).get('id')")
[ -n "$SID" ] && [ "$SID" != None ] && ok "TCP stream created" || bad "TCP stream not created"
sleep 2
# Stream ports are checked from the Docker host when CORVUS_SSH is set (e.g. "ssh user@host"):
# a NAT in front of the test server may forward only the gateway's HTTP/HTTPS/admin ports.
scurl() { if [ -n "${CORVUS_SSH:-}" ]; then $CORVUS_SSH curl "$@" "http://127.0.0.1:8090/stream-check"; else curl "$@" "http://$H:8090/stream-check"; fi; }
SB=$(scurl -s -m 5 --http1.0)
echo "$SB" | grep -q '^Hostname:' && ok "TCP stream forwards to the demo backend" || bad "no whoami answer through the stream: $SB"
echo "$SB" | grep -q 'GET /stream-check' && ok "stream passes the request through unchanged" || bad "request not passed through: $SB"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/streams -d '{"incoming_port":8090,"tcp_forwarding":true,"forward_host":"demo","forward_port":80}')" 422 "second TCP stream on the same port rejected"
c -o /dev/null -X PUT $A/api/streams/$SID -d '{"incoming_port":8090,"tcp_forwarding":true,"forward_host":"demo","forward_port":80,"enabled":false}'
sleep 2
chk "$(scurl -s -m 5 -o /dev/null -w '%{http_code}')" 000 "disabled stream closes the port"
c -o /dev/null -X DELETE $A/api/streams/$SID

echo "[access queue]"
WR=$(c -X POST $A/api/waitrooms -d '{"name":"itest-queue","capacity":1000,"max_new_per_ip":0}' | js "d['waitroom']['id']")
chk "$(put_host ",\"waitroom_id\":$WR,\"waitroom_path\":\"/\"")" 200 "attach access queue"
WQ=$(mktemp)
r=$(curl -s -D - -o /dev/null -c "$WQ" -b "$WQ" -H 'Accept: application/json' "$G/wq-api" | tr -d '')
chk "$(echo "$r" | head -1 | cut -d' ' -f2)" 429 "cookie check for an API request: 429, not a server error"
echo "$r" | grep -qi '^retry-after: 1' && ok "cookie check sends Retry-After" || bad "cookie check without Retry-After"
chk "$(code -c "$WQ" -b "$WQ" -H 'Accept: application/json' "$G/wq-api")" 200 "with the queue cookie: admitted"
chk "$(code -H 'Accept: text/html' "$G/wq-page")" 302 "cookie check for a page: redirect to itself"
sleep 2
chk "$(c "$A/api/traffic/logs?range=1h&limit=50&q=/wq-api" | js "(429,'cookie-check') in [(l['status'],l.get('wr')) for l in d['items'] if l['path']=='/wq-api']")" True "cookie check logged as 429 cookie-check"
chk "$(c "$A/api/traffic/events?range=1h" | js "any(e['kind'] in ('rate_limited','server_error') and e['path']=='/wq-api' for e in d)")" False "cookie check is neither a rate limit nor a server error event"
put_host ',"waitroom_id":null' >/dev/null
c -o /dev/null -X DELETE $A/api/waitrooms/$WR
rm -f "$WQ"

echo "[ipv6]"
# IPv4 keeps working whatever CORVUS_IPV6 decided; IPv6 is checked only when the server
# has an IPv6 address reachable from here (CORVUS_HOST6, e.g. 2001:db8::10).
chk "$(code "$G/__corvus/healthz" -H 'Host: unknown.corvus.test')" 200 "IPv4 listener serves"
chk "$(code -H "Host: $H" "$G/")" 200 "proxy host serves over IPv4"
if [ -n "${CORVUS_HOST6:-}" ]; then
  chk "$(code -g -H "Host: $H" "http://[$CORVUS_HOST6]:8081/")" 200 "proxy host serves over IPv6"
  chk "$(code -g "http://[$CORVUS_HOST6]:8081/__corvus/healthz")" 200 "default server listens on IPv6"
else
  ok "ipv6 not available (set CORVUS_HOST6 to test IPv6 listeners)"
fi
echo "[upload limits and gzip]"
LIMJ='{"domains":["limits.corvus.test"],"forward_scheme":"http","forward_host":"demo","forward_port":80,"max_upload_mb":1,"upstream_timeout_s":30'
LH=$(c -X POST $A/api/proxy-hosts -d "$LIMJ}" | js "d.get('id') or d.get('proxy_host',{}).get('id')")
chk "$(c $A/api/proxy-hosts/$LH | js "(d.get('max_upload_mb'),d.get('upstream_timeout_s'),d.get('gzip'))")" "(1, 30, True)" "host stores upload size, timeout and gzip (default on)"
sleep 2
UP=$(mktemp)
head -c 2097152 /dev/zero > "$UP"
chk "$(code -H 'Host: limits.corvus.test' -H 'Content-Type: application/octet-stream' --data-binary @"$UP" "$G/upload")" 413 "2 MB upload to a 1 MB host: 413"
head -c 524288 /dev/zero > "$UP"
chk "$(code -H 'Host: limits.corvus.test' -H 'Content-Type: application/octet-stream' --data-binary @"$UP" "$G/upload")" 200 "512 KB upload accepted"
rm -f "$UP"
# whoami echoes the request headers as text/plain: a padded header makes the answer > 1 KB.
PAD=$(head -c 3000 /dev/zero | tr '\0' a)
ce() { curl -s -o /dev/null -D - -H 'Host: limits.corvus.test' -H 'Accept-Encoding: gzip' -H "X-Pad: $PAD" "$G/gz" | tr -d '\r' | awk -F': ' 'tolower($1)=="content-encoding"{print $2}'; }
chk "$(ce)" gzip "text response compressed"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/proxy-hosts/$LH -d "$LIMJ,\"gzip\":false}")" 200 "turn gzip off"
sleep 2
chk "$(ce)" "" "gzip off: response not compressed"
c -o /dev/null -X DELETE $A/api/proxy-hosts/$LH
echo "[certificate replace]"
CT=$(mktemp -d)
for n in 1 2; do
  san="DNS:replace.corvus.test"; [ $n = 2 ] && san="$san,DNS:www.replace.corvus.test"
  MSYS2_ARG_CONV_EXCL="/CN" openssl req -x509 -newkey rsa:2048 -nodes -days 30 -set_serial $((1000+n)) -subj /CN=replace.corvus.test \
    -addext "subjectAltName=$san" -keyout "$CT/k$n.pem" -out "$CT/c$n.pem" 2>/dev/null
done
certjson() { $PY -c "import json,sys; print(json.dumps({'name':'itest-replace','certificate':open(sys.argv[1]).read(),'private_key':open(sys.argv[2]).read()}))" "$CT/c$1.pem" "$CT/k$1.pem"; }
CRID=$(c -X POST $A/api/certificates -d "$(certjson 1)" | js "d['id']")
RH=$(c -X POST $A/api/proxy-hosts -d "{\"domains\":[\"replace.corvus.test\"],\"forward_scheme\":\"http\",\"forward_host\":\"demo\",\"forward_port\":80,\"certificate_id\":$CRID}" | js "d.get('id') or d.get('proxy_host',{}).get('id')")
sleep 2
serial() { echo | openssl s_client -connect "$H:8443" -servername replace.corvus.test 2>/dev/null | openssl x509 -noout -serial 2>/dev/null | cut -d= -f2; }
chk "$(serial)" 03E9 "host serves the uploaded certificate"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/certificates/$CRID -d "$($PY -c "import json,sys; print(json.dumps({'certificate':open(sys.argv[1]).read(),'private_key':open(sys.argv[2]).read()}))" "$CT/c2.pem" "$CT/k1.pem")")" 422 "replacement with a key that does not match: 422"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT $A/api/certificates/$CRID -d "$(certjson 2)")" 200 "replace the certificate in place"
sleep 2
chk "$(serial)" 03EA "host serves the replacement without being edited"
chk "$(c $A/api/certificates | js "[(x['name'],len(x['domains'])) for x in d if x['id']==$CRID][0]")" "('itest-replace', 2)" "same ID, name kept, domains updated"
chk "$(c $A/api/proxy-hosts/$RH | js "d['certificate_id']")" "$CRID" "host still uses the certificate"
c -o /dev/null -X DELETE $A/api/proxy-hosts/$RH
c -o /dev/null -X DELETE $A/api/certificates/$CRID
rm -rf "$CT"

echo "[backup]"
BK=$(mktemp)
chk "$(curl -sk -b "$J" -o "$BK" -w '%{http_code}' "$A/api/backup")" 200 "backup download"
chk "$($PY -c "import tarfile,sys; n=tarfile.open(sys.argv[1]).getnames(); print('manifest.json' in n and any(x.endswith('.db') for x in n) and any(x.startswith('secrets/') for x in n) and not any(x.startswith('nginx/') for x in n))" "$BK")" True "backup contains manifest, database and secrets but no generated config"
rm -f "$BK"

echo "[NPM import]"
P=$(c -X POST $A/api/import/npm/preview -d '{}')
chk "$(echo "$P" | js "(len(d['proxy_hosts']),len(d['certificates']),len(d['access_lists']))")" "(3, 2, 1)" "preview hosts/certs/acls (soft-deleted excluded)"
chk "$(c -o /dev/null -w '%{http_code}' -X POST $A/api/import/npm/preview -d '{"path":"/data"}')" 422 "paths outside /import rejected"
R=$(c -X POST $A/api/import/npm/apply -d '{}')
chk "$(echo "$R" | js "sorted((k,v) for k,v in d['imported'].items() if k not in ('security_rules','redirection_hosts','streams'))")" "[('access_lists', 1), ('certificates', 2), ('proxy_hosts', 3)]" "import applied"
chk "$(code -H 'Host: npm-shop.corvus.test' "$G/")" 301 "imported host forces HTTPS"
chk "$(curl -sk -o /dev/null -w '%{http_code}' --resolve npm-shop.corvus.test:8443:$H https://npm-shop.corvus.test:8443/)" 200 "imported certificate serves HTTPS"
chk "$(code -H 'Host: npm-custom.corvus.test' -u alice:alice-secret-1 "$G/")" 200 "imported access list password works"
chk "$(c -X POST $A/api/import/npm/apply -d '{}' | js "len(d['skipped'])>=3")" True "re-import skips conflicting hosts"

cleanup
c -o /dev/null -X PUT $A/api/settings/autoban -d "$AUTOBAN"
echo
echo "integration: $pass passed, $failn failed"
rm -f "$J"
[ "$failn" = 0 ]
