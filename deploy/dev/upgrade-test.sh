#!/usr/bin/env bash
# Upgrade test: what an older Corvus image created must survive a switch to a newer image
# on the same /data volume — the new image starts, applies its migrations, keeps every
# object and still serves it, and the console login works.
#   deploy/dev/upgrade-test.sh ghcr.io/corvus-gateway/gateway:latest corvus/gateway:ci
# Requires docker, curl, python3 and openssl. Publishes host ports 8081 (HTTP), 8443 (HTTPS),
# 9443 (admin) and 8090 (TCP stream), like the dev stack — set UPG_PORTS="http https admin
# stream" to use others, CORVUS_HOST if the published ports are not on 127.0.0.1. Removes
# every container, volume and network it creates. Throwaway admin credentials per run.
set -uo pipefail
OLD=${1:?usage: upgrade-test.sh OLD_IMAGE NEW_IMAGE}
NEW=${2:?usage: upgrade-test.sh OLD_IMAGE NEW_IMAGE}
DIR=$(cd "$(dirname "$0")" && pwd)
P=${UPG_PREFIX:-itestci-upgrade}
read -r PH PS PA PT <<<"${UPG_PORTS:-8081 8443 9443 8090}"
H=${CORVUS_HOST:-127.0.0.1}
PY=${PYTHON:-python3}
HIP=$($PY -c 'import socket,sys; print(socket.gethostbyname(sys.argv[1]))' "$H")
A=https://$H:$PA
DOM=upgrade.corvus.test
DEMO=$(awk '/image: traefik\/whoami/ {print $2; exit}' "$DIR/docker-compose.yml")
T=$(mktemp -d)
EMAIL=upgrade-admin@corvus.test
PASS=$(openssl rand -hex 18)
J=$T/jar
pass=0; failn=0
ok()   { pass=$((pass+1)); echo "  PASS  $*"; }
bad()  { failn=$((failn+1)); echo "  FAIL  $*"; }
chk()  { if [ "$1" = "$2" ]; then ok "$3 ($1)"; else bad "$3 (got $1, want $2)"; fi; }
c()    { curl -sk -b "$J" -c "$J" -H 'X-Corvus-Request: 1' -H 'Content-Type: application/json' "$@"; }
js()   { $PY -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
die()  { echo "upgrade test: $*"; logs; exit 1; }
logs() { docker logs "$P-gw" 2>&1 | grep -v setup_token | tail -40; }
remove() {
  docker rm -f "$P-gw" "$P-demo" >/dev/null 2>&1
  docker volume rm "$P-data" >/dev/null 2>&1
  docker network rm "$P-net" >/dev/null 2>&1
}
trap 'remove; rm -rf "$T"' EXIT
remove

start() { # image
  docker run -d --name "$P-gw" --network "$P-net" -v "$P-data:/data" \
    -p "$PH:80" -p "$PS:443" -p "$PA:9443" -p "$PT:8090" "$1" >/dev/null || die "cannot start $1"
  local s=""
  for _ in $(seq 1 90); do
    s=$(docker inspect -f '{{.State.Health.Status}}' "$P-gw" 2>/dev/null)
    [ "$s" = healthy ] && break; sleep 2
  done
  [ "$s" = healthy ] || die "$1 did not become healthy ($s)"
  for _ in $(seq 1 30); do [ "$(curl -sk -o /dev/null -w '%{http_code}' "$A/api/status")" = 200 ] && return; sleep 1; done
  die "$1: admin API not answering"
}
stop() { docker stop -t 30 "$P-gw" >/dev/null && docker cp "$P-gw:/data/corvus.db" "$T/$1.db" && docker rm "$P-gw" >/dev/null; }
migrations() { $PY -c 'import sqlite3,sys; print(" ".join(r[0] for r in sqlite3.connect(sys.argv[1]).execute("SELECT version FROM schema_migrations ORDER BY version")))' "$T/$1.db"; }

# Everything the old image was given must be served the same way (run before and after).
served() { # label
  local h="-H Host:$DOM" u="-u ann:ann-pass-1234"
  chk "$(curl -s -o /dev/null -w '%{http_code}' $h "http://$H:$PH/")" 401 "$1: proxy host asks for the access-list password"
  chk "$(curl -s -o /dev/null -w '%{http_code}' $h $u "http://$H:$PH/")" 200 "$1: proxy host answers with the password"
  chk "$(curl -s -o /dev/null -w '%{http_code}' --cacert "$T/cert.pem" --resolve "$DOM:$PS:$HIP" $u "https://$DOM:$PS/")" 200 \
    "$1: HTTPS with the uploaded certificate (verified against it)"
  chk "$(curl -s -o /dev/null -w '%{http_code}' $h $u "http://$H:$PH/upgrade-blocked/x")" 410 "$1: security rule blocks"
  chk "$(curl -s -o /dev/null -w '%{http_code}' $h $u "http://$H:$PH/.env")" 403 "$1: exploit blocking on"
  curl -s -m 5 --http1.0 "http://$H:$PT/stream-check" | grep '^GET /stream-check' >/dev/null \
    && ok "$1: TCP stream forwards to the backend" || bad "$1: TCP stream does not forward"
}

echo "[old image: $OLD]"
docker network create "$P-net" >/dev/null || die "network"
docker run -d --name "$P-demo" --network "$P-net" --network-alias demo "$DEMO" >/dev/null || die "demo backend"
start "$OLD"
OLDV=$(curl -sk "$A/api/status" | js "d['version']")
TOK=$(docker logs "$P-gw" 2>&1 | grep -o 'setup_token=[^ ]*' | tail -1 | cut -d= -f2)
chk "$(c -o /dev/null -w '%{http_code}' -X POST "$A/api/setup" -d "{\"token\":\"$TOK\",\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")" 201 "first-run setup (version $OLDV)"
openssl req -x509 -newkey rsa:2048 -nodes -days 30 -set_serial 4242 -subj "/CN=$DOM" \
  -addext "subjectAltName=DNS:$DOM" -keyout "$T/key.pem" -out "$T/cert.pem" 2>/dev/null || die "openssl"
CID=$(c -X POST "$A/api/certificates" -d "$($PY -c "import json,sys; print(json.dumps({'name':'upgrade-cert','certificate':open(sys.argv[1]).read(),'private_key':open(sys.argv[2]).read()}))" "$T/cert.pem" "$T/key.pem")" | js "d['id']")
ACL=$(c -X POST "$A/api/access-lists" -d '{"name":"upgrade-office","users":[{"username":"ann","password":"ann-pass-1234"}]}' | js "d['access_list']['id']")
HID=$(c -X POST "$A/api/proxy-hosts" -d "{\"domains\":[\"$DOM\"],\"forward_host\":\"demo\",\"forward_port\":80,\"block_exploits\":true,\"certificate_id\":$CID,\"access_list_id\":$ACL}" | js "d['proxy_host']['id']")
RID=$(c -X POST "$A/api/security-rules" -d '{"name":"upgrade-block","action":"block","status_code":410,"expression":{"groups":[[{"field":"http.path","operator":"starts_with","value":["/upgrade-blocked"]}]]}}' | js "d['rule']['id']")
SID=$(c -X POST "$A/api/streams" -d '{"incoming_port":8090,"tcp_forwarding":true,"forward_host":"demo","forward_port":80}' | js "d['stream']['id']")
RET='{"log_mode":"notable","log_days":21,"log_max_mb":512,"stats_days":40,"events_days":45}'
chk "$(c -o /dev/null -w '%{http_code}' -X PUT "$A/api/settings/retention" -d "$RET")" 200 "retention settings saved"
chk "$(c -o /dev/null -w '%{http_code}' -X PUT "$A/api/settings/network" -d '{"trusted_proxies":["10.123.0.0/16"],"real_ip_header":"X-Forwarded-For"}')" 200 "network settings saved"
echo "  created: certificate $CID, access list $ACL, proxy host $HID, security rule $RID, stream $SID"
sleep 3
served old
stop old || die "cannot stop the old image"
BEFORE=$(migrations old)

echo "[new image: $NEW]"
start "$NEW"
NEWV=$(curl -sk "$A/api/status" | js "d['version']")
echo "  version $OLDV -> $NEWV"
chk "$(curl -sk "$A/api/status" | js "d['setup_required']")" False "no new setup required"
rm -f "$J"
chk "$(c -o /dev/null -w '%{http_code}' -X POST "$A/api/auth/login" -d "{\"email\":\"$EMAIL\",\"password\":\"$PASS\"}")" 200 "console login with the old credentials"
chk "$(c "$A/api/auth/me" | js "(d['email'], d['role'])")" "('$EMAIL', 'admin')" "signed in as the admin"
curl -sk "$A/" | grep -i '<div id="root"' >/dev/null && ok "console page served" || bad "console page not served"
chk "$(c "$A/api/certificates" | js "[(x['name'], x['domains']) for x in d if x['id']==$CID]")" "[('upgrade-cert', ['$DOM'])]" "certificate kept"
chk "$(c "$A/api/access-lists" | js "[x['name'] for x in d if x['id']==$ACL]")" "['upgrade-office']" "access list kept"
chk "$(c "$A/api/proxy-hosts/$HID" | js "(d['domains'], d['certificate_id'], d['access_list_id'], d['block_exploits'])")" "(['$DOM'], $CID, $ACL, True)" "proxy host kept"
chk "$(c "$A/api/security-rules" | js "[(r['name'], r['action'], r['status_code']) for r in d if r['id']==$RID]")" "[('upgrade-block', 'block', 410)]" "security rule kept"
chk "$(c "$A/api/streams" | js "[(s['incoming_port'], s['forward_host'], s['forward_port']) for s in d if s['id']==$SID]")" "[(8090, 'demo', 80)]" "stream kept"
chk "$(c "$A/api/settings/retention" | js "(d['settings']['log_days'], d['settings']['stats_days'], d['settings']['events_days'])")" "(21, 40, 45)" "retention settings kept"
chk "$(c "$A/api/settings/network" | js "d['trusted_proxies']")" "['10.123.0.0/16']" "network settings kept"
served new
chk "$(docker logs "$P-gw" 2>&1 | grep -cE 'level=ERROR|GATEWAY DOWN')" 0 "no errors in the new image's log"
stop new || die "cannot stop the new image"
AFTER=$(migrations new)
WANT=$(cd "$DIR/../../control/internal/store/migrations" && ls *.sql | sed 's/\.sql$//' | sort | tr '\n' ' ' | sed 's/ $//')
miss=""; for v in $WANT; do case " $AFTER " in *" $v "*) ;; *) miss="$miss $v";; esac; done
chk "${miss:-none}" none "every migration of this source tree applied (missing)"
echo "  migrations: $(echo "$BEFORE" | wc -w) before, $(echo "$AFTER" | wc -w) after"

echo
echo "upgrade: $pass passed, $failn failed"
[ "$failn" = 0 ]
