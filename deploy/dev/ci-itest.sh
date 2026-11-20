#!/usr/bin/env bash
# Runs the integration checks (itest.sh) against a fresh dev stack, as CI does: fixtures,
# `docker compose up`, first-run setup with throwaway admin credentials, a first proxy host,
# then itest.sh. The image is not built here:
#   docker build -f deploy/Dockerfile -t corvus/gateway:ci .
#   CORVUS_IMAGE=corvus/gateway:ci deploy/dev/ci-itest.sh
# Requires docker (compose v2), curl, python3 and openssl. COMPOSE_PROJECT names the stack
# (default itestci); KEEP_STACK=1 leaves it running afterwards. Exit status is itest.sh's.
set -uo pipefail
cd "$(dirname "$0")"
export CORVUS_IMAGE=${CORVUS_IMAGE:-corvus/gateway:dev}
P=${COMPOSE_PROJECT:-itestci}
export CORVUS_HOST=${CORVUS_HOST:-localhost}
A=https://$CORVUS_HOST:9443
T=$(mktemp -d)
dc() { docker compose -p "$P" "$@"; }
logs() { docker logs "$(dc ps -q corvus)" 2>&1 | grep -v setup_token | tail -${1:-60}; }
finish() { [ -z "${KEEP_STACK:-}" ] && dc down -v --remove-orphans >/dev/null 2>&1; rm -rf "$T"; }
die() { echo "ci-itest: $*"; logs; exit 1; }
trap finish EXIT

[ -s pebble-ca.pem ] && [ -s import/npm/data/database.sqlite ] && [ -s fixtures/www/blocklist-in.txt ] || sh setup-fixtures.sh || die "fixtures"
dc down -v --remove-orphans >/dev/null 2>&1
dc up -d --no-build --quiet-pull --wait --wait-timeout 180 || die "stack did not start"
TOK=""
for _ in $(seq 1 60); do
  TOK=$(docker logs "$(dc ps -q corvus)" 2>&1 | grep -o 'setup_token=[^ ]*' | tail -1 | cut -d= -f2)
  [ -n "$TOK" ] && break; sleep 1
done
[ -n "$TOK" ] || die "no setup token in the log"

# Throwaway administrator for this run only.
printf 'ci-admin@corvus.test\n%s\n' "$(openssl rand -hex 18)" > "$T/admin"
export CORVUS_ADMIN_FILE=$T/admin
TN=$T; command -v cygpath >/dev/null 2>&1 && TN=$(cygpath -m "$T")   # Git Bash: curl is a native program
c() { curl -sk -b "$TN/jar" -c "$TN/jar" -H 'X-Corvus-Request: 1' -H 'Content-Type: application/json' -o /dev/null -w '%{http_code}' "$@"; }
r=$(c -X POST "$A/api/setup" -d "{\"token\":\"$TOK\",\"email\":\"$(sed -n 1p "$T/admin")\",\"password\":\"$(sed -n 2p "$T/admin")\"}")
[ "$r" = 201 ] || die "first-run setup answered $r"
r=$(c -X POST "$A/api/proxy-hosts" -d "{\"domains\":[\"$CORVUS_HOST\"],\"forward_host\":\"demo\",\"forward_port\":80}")
[ "$r" = 201 ] || die "first proxy host answered $r"
sleep 3

# large IP list feed checks: served by the fixtures service, memory read with docker stats
export CORVUS_FIXTURE_FEEDS=https://fixtures:8443 CORVUS_CONTAINER=$(dc ps -q corvus)
bash ./itest.sh; rc=$?
errs=$(docker logs "$(dc ps -q corvus)" 2>&1 | grep -cE 'level=ERROR|GATEWAY DOWN')
echo "gateway log: $errs error line(s)"
[ "$rc" = 0 ] || { echo "--- corvus log (last 80 lines) ---"; logs 80; }
exit "$rc"
