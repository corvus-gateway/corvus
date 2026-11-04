#!/usr/bin/env bash
# Corvus throughput / latency benchmark (GitHub issue #43).
#
# Measures requests/s and latency (p50/p90/p99, wrk) plus the gateway container's CPU% and
# memory (docker stats) for a plain nginx reverse proxy (baseline) and for Corvus with
# increasing feature sets. Everything runs on this machine with --network host.
#
# Usage (Linux with Docker; needs curl, openssl, python3, wrk, taskset, timeout, ss and
# passwordless sudo or root for adding /etc/hosts entries if the bench names do not resolve):
#
#   docker build -f deploy/Dockerfile -t corvus:bench .
#   CORVUS_IMAGE=corvus:bench DURATION=20s CONNECTIONS=64 deploy/bench/bench.sh
#
# Environment:
#   CORVUS_IMAGE   image to test (default corvus:bench)
#   DURATION       measured run per scenario in seconds, "20" or "20s" (default 20s)
#   WARMUP         warm-up run before each scenario (default 5s)
#   CONNECTIONS    concurrent wrk connections (default 64)
#   OUT_DIR        results directory (default ./bench-out): bench-results.json, summary.md,
#                  wrk-<scenario>.txt (raw wrk output), stats-<scenario>.txt (docker stats samples)
#   WRK_CPUS, BACKEND_CPUS, GW_CPUS   CPU sets (taskset / --cpuset-cpus syntax), see below
#   SCENARIOS      space-separated subset to run (default: all)
#
# CPU layout (pinned so results stay comparable over time):
#   4–7 CPUs: wrk on CPU 0, backend on CPU 1, gateway (nginx baseline or Corvus) on CPUs 2..n-1
#   ≥8 CPUs:  wrk on CPUs 0-1, backend on CPUs 2-3, gateway on CPUs 4..n-1
#   <4 CPUs:  no pinning (numbers are then only a rough indication)
#
# Ports used on the host (must be free): 80, 443, 9443, 9180, 9181 (Corvus), 6390 (embedded
# Valkey, CORVUS_REDIS_PORT), 18080 (nginx baseline), 18081 (backend).
#
# Every network call and wrk run is bounded by a timeout. A scenario that fails or times out is
# recorded as skipped (with the reason) and the next one runs. The script removes its
# containers and volume on exit; it does not publish anything.
set -euo pipefail

CORVUS_IMAGE=${CORVUS_IMAGE:-corvus:bench}
DURATION=${DURATION:-20s}
WARMUP=${WARMUP:-5s}
CONNECTIONS=${CONNECTIONS:-64}
OUT_DIR=${OUT_DIR:-./bench-out}
ALL_SCENARIOS="nginx-baseline corvus-default corvus-protected corvus-https-keepalive corvus-https-newconn corvus-queue corvus-challenge-pass"
SCENARIOS=${SCENARIOS:-$ALL_SCENARIOS}
# nginx 1.28 (stable), multi-arch index digest
NGINX_IMAGE=nginx:1.28-alpine@sha256:a8b39bd9cf0f83869a2162827a0caf6137ddf759d50a171451b335cecc87d236

BACKEND_PORT=18081
BASELINE_PORT=18080
REDIS_PORT=6390
A=https://127.0.0.1:9443
H_PLAIN=bench-plain.test
H_PROT=bench-protected.test
H_QUEUE=bench-queue.test
H_CHAL=bench-challenge.test
HOSTS_MARK="# corvus-bench"

C_BACKEND=corvus-bench-backend
C_BASELINE=corvus-bench-nginx
C_CORVUS=corvus-bench-corvus
VOL=corvus-bench-data

log() { echo "[bench] $(date -u +%H:%M:%S) $*" >&2; }
# Inside a scenario (a subshell) die ends only that scenario; the reason is kept for the report.
die() { log "ERROR: $*"; echo "$*" > "$WORK/reason" 2>/dev/null || true; exit 1; }

for t in docker curl openssl python3 wrk taskset timeout ss; do
  command -v "$t" >/dev/null || { echo "[bench] missing tool: $t" >&2; exit 1; }
done

secs() { # "20s" / "20" / "1m" → seconds
  case "$1" in *m) echo $(( ${1%m} * 60 ));; *s) echo "${1%s}";; *) echo "$1";; esac
}
DUR_S=$(secs "$DURATION")
WARM_S=$(secs "$WARMUP")

mkdir -p "$OUT_DIR"
OUT_DIR=$(cd "$OUT_DIR" && pwd)
WORK=$(mktemp -d)
JAR="$WORK/jar"
RESULTS="$WORK/results.jsonl"
: > "$RESULTS"
ADDED_HOSTS=""

cleanup() {
  log "cleanup"
  timeout 60 docker rm -f "$C_BACKEND" "$C_BASELINE" "$C_CORVUS" >/dev/null 2>&1 || true
  timeout 30 docker volume rm "$VOL" >/dev/null 2>&1 || true
  if [ -n "$ADDED_HOSTS" ]; then
    if [ "$(id -u)" = 0 ]; then sed -i "/$HOSTS_MARK\$/d" /etc/hosts; else sudo -n sed -i "/$HOSTS_MARK\$/d" /etc/hosts || true; fi
  fi
  rm -rf "$WORK"
}
trap cleanup EXIT

# ---------------------------------------------------------------- CPU layout

NPROC=$(nproc)
if [ -z "${GW_CPUS:-}" ]; then
  if [ "$NPROC" -ge 8 ]; then WRK_CPUS=0-1; BACKEND_CPUS=2-3; GW_CPUS=4-$((NPROC - 1))
  elif [ "$NPROC" -ge 4 ]; then WRK_CPUS=0; BACKEND_CPUS=1; GW_CPUS=2-$((NPROC - 1))
  else WRK_CPUS=""; BACKEND_CPUS=""; GW_CPUS=""
  fi
fi
WRK_CPUS=${WRK_CPUS:-}; BACKEND_CPUS=${BACKEND_CPUS:-}
# number of CPUs in a cpuset list such as "0", "2-3" or "0,2-5"
count_cpus() {
  [ -z "$1" ] && { echo "$NPROC"; return; }
  python3 -c '
import sys
n = 0
for part in sys.argv[1].split(","):
    a, _, b = part.partition("-")
    n += (int(b) - int(a) + 1) if b else 1
print(n)' "$1"
}
GW_VCPUS=$(count_cpus "$GW_CPUS")
WRK_THREADS=$(count_cpus "$WRK_CPUS")
BACKEND_WORKERS=$(count_cpus "$BACKEND_CPUS")
cpuset() { [ -n "$1" ] && echo "--cpuset-cpus=$1"; true; }
WRK=(wrk)
[ -n "$WRK_CPUS" ] && WRK=(taskset -c "$WRK_CPUS" wrk)
CPU_MODEL=$(lscpu | sed -n 's/^Model name: *//p' | head -1)
log "nproc=$NPROC cpu=\"$CPU_MODEL\" wrk=${WRK_CPUS:-any} (threads $WRK_THREADS) backend=${BACKEND_CPUS:-any} gateway=${GW_CPUS:-any} ($GW_VCPUS vCPU)"

# ---------------------------------------------------------------- preflight

log "preflight: ports, /etc/hosts, images"
for p in 80 443 9443 9180 9181 $REDIS_PORT $BASELINE_PORT $BACKEND_PORT; do
  if ss -Hltn "sport = :$p" | grep -q .; then die "port $p is in use"; fi
done

for h in $H_PLAIN $H_PROT $H_QUEUE $H_CHAL; do
  if [ "$(getent ahostsv4 "$h" | awk 'NR==1 {print $1}')" != "127.0.0.1" ]; then
    line="127.0.0.1 $h $HOSTS_MARK"
    if [ "$(id -u)" = 0 ]; then echo "$line" >> /etc/hosts
    else echo "$line" | sudo -n tee -a /etc/hosts >/dev/null || die "cannot add $h to /etc/hosts (run as root or add '127.0.0.1 $h' yourself)"
    fi
    ADDED_HOSTS=1
  fi
done

docker image inspect "$CORVUS_IMAGE" >/dev/null 2>&1 || die "image $CORVUS_IMAGE not found (build it first, see the top of this script)"
timeout 300 docker pull -q "$NGINX_IMAGE" >/dev/null || die "cannot pull $NGINX_IMAGE"

# ---------------------------------------------------------------- helpers

CURL=(curl -sk --connect-timeout 5 --max-time 20)
# JSON API with the admin session (cookie jar + CSRF header)
api() { "${CURL[@]}" -b "$JAR" -c "$JAR" -H 'X-Corvus-Request: 1' -H 'Content-Type: application/json' "$@"; }
js()  { python3 -c "import sys,json; d=json.load(sys.stdin); print($1)"; }
# POST to the API and print a field of the response; dies (ends the scenario) on an error
create() { # path body python-expr
  local r
  r=$(api -w '\n%{http_code}' -X POST "$A$1" -d "$2") || die "POST $1: no response"
  case "$(echo "$r" | tail -1)" in 2??) ;; *) die "POST $1 failed: $(echo "$r" | head -c 600 | tr '\n' ' ')";; esac
  echo "$r" | sed '$d' | js "$3"
}
code() { "${CURL[@]}" -o /dev/null -w '%{http_code}' "$@" || true; }

wait_200() { # url [curl args...] — wait (≤ ~60 s) until the URL answers 200 (config deployed)
  local url=$1; shift
  for _ in $(seq 1 60); do
    [ "$(code --max-time 2 "$@" "$url")" = 200 ] && return 0
    sleep 1
  done
  return 1
}

# Samples `docker stats` for a container until the marker file disappears.
sample_stats() { # container outfile marker
  local line
  while [ -e "$3" ]; do
    line=$(timeout 10 docker stats --no-stream --format '{{.CPUPerc}} {{.MemUsage}}' "$1" 2>/dev/null || true)
    # keep only samples taken entirely inside the measured run
    if [ -e "$3" ] && [ -n "$line" ]; then echo "$line" >> "$2"; fi
  done
  true
}

# CPU seconds (utime+stime) per process name inside a container: "name seconds" lines.
proc_cpu() { # container
  local hz pid comm
  hz=$(getconf CLK_TCK)
  { timeout 10 docker top "$1" -eo pid,comm 2>/dev/null || true; } | tail -n +2 | while read -r pid comm; do
    [ -r "/proc/$pid/stat" ] || continue
    awk -v c="$comm" -v hz="$hz" '{ sub(/^.*\) /, ""); print c, ($12 + $13) / hz }' "/proc/$pid/stat" || true
  done
}

# Warm-up, measured wrk run with docker stats sampling, records the result.
# run_wrk name container note [wrk args...]
run_wrk() {
  local name=$1 ctr=$2 note=$3; shift 3
  log "$name: warm-up ${WARM_S}s"
  timeout $((WARM_S + 30)) "${WRK[@]}" -t"$WRK_THREADS" -c"$CONNECTIONS" -d"${WARM_S}s" "$@" > /dev/null \
    || die "wrk warm-up failed or timed out (exit $?)"
  sleep 1
  log "$name: measuring ${DUR_S}s, $CONNECTIONS connections"
  local marker="$WORK/sampling" stats="$OUT_DIR/stats-$name.txt" spid t0 t1 rc=0
  : > "$stats"; touch "$marker"
  sample_stats "$ctr" "$stats" "$marker" < /dev/null > /dev/null 2>&1 &
  spid=$!
  proc_cpu "$ctr" > "$WORK/cpu0"; t0=$(date +%s.%N)
  timeout $((DUR_S + 30)) "${WRK[@]}" -t"$WRK_THREADS" -c"$CONNECTIONS" -d"${DUR_S}s" --latency "$@" > "$OUT_DIR/wrk-$name.txt" || rc=$?
  t1=$(date +%s.%N); proc_cpu "$ctr" > "$WORK/cpu1"
  rm -f "$marker"; timeout 15 tail --pid="$spid" -f /dev/null || kill "$spid" 2>/dev/null || true
  [ "$rc" = 0 ] || die "wrk failed or timed out (exit $rc)"
  cat "$OUT_DIR/wrk-$name.txt" >&2
  log "$name: recording result"
  python3 - "$name" "$OUT_DIR/wrk-$name.txt" "$stats" "$note" "$WORK/cpu0" "$WORK/cpu1" "$t0" "$t1" >> "$RESULTS" <<'PY'
import json, re, sys
from collections import defaultdict
name, wrk_file, stats_file, note, cpu0, cpu1, t0, t1 = sys.argv[1:9]
def proc(path):
    d = defaultdict(float)
    for line in open(path):
        p = line.rsplit(None, 1)
        if len(p) == 2:
            d[p[0]] += float(p[1])
    return d
b, a = proc(cpu0), proc(cpu1)
wall = float(t1) - float(t0)
by_proc = {k: round(100 * (a[k] - b.get(k, 0)) / wall, 1) for k in a}
by_proc = dict(sorted(((k, v) for k, v in by_proc.items() if v >= 0.5), key=lambda kv: -kv[1]))
txt = open(wrk_file).read()
def dur_ms(v):
    m = re.match(r"([\d.]+)(us|ms|s|m)$", v)
    n, u = float(m.group(1)), m.group(2)
    return n * {"us": 0.001, "ms": 1, "s": 1000, "m": 60000}[u]
lat = dict((p, dur_ms(v)) for p, v in re.findall(r"^\s+(50|75|90|99)%\s+(\S+)", txt, re.M))
rps = float(re.search(r"Requests/sec:\s+([\d.]+)", txt).group(1))
total = int(re.search(r"(\d+) requests in", txt).group(1))
non2xx = int((re.search(r"Non-2xx or 3xx responses: (\d+)", txt) or [0, 0])[1])
sock = re.search(r"Socket errors: connect (\d+), read (\d+), write (\d+), timeout (\d+)", txt)
errors = sum(int(x) for x in sock.groups()) if sock else 0
cpu, mem = [], []
units = {"B": 1 / 2**20, "KiB": 1 / 1024, "MiB": 1, "GiB": 1024, "kB": 1e3 / 2**20, "MB": 1e6 / 2**20, "GB": 1e9 / 2**20}
for line in open(stats_file):
    m = re.match(r"([\d.]+)%\s+([\d.]+)(\w+)\s*/", line)
    if m:
        cpu.append(float(m.group(1)))
        mem.append(float(m.group(2)) * units.get(m.group(3), 1))
print(json.dumps({
    "scenario": name, "rps": round(rps, 1), "requests": total,
    "p50_ms": round(lat.get("50", 0), 3), "p90_ms": round(lat.get("90", 0), 3), "p99_ms": round(lat.get("99", 0), 3),
    "non_2xx": non2xx, "socket_errors": errors,
    "cpu_mean_pct": round(sum(cpu) / len(cpu), 1) if cpu else None,
    "mem_peak_mb": round(max(mem), 1) if mem else None, "stats_samples": len(cpu),
    "cpu_by_process_pct": by_proc,
    "note": note,
}))
PY
  sleep 2
}

skip() { # name reason
  log "$1: SKIPPED — $2"
  python3 -c 'import json,sys; print(json.dumps({"scenario": sys.argv[1], "skipped": True, "note": sys.argv[2]}))' "$1" "$2" >> "$RESULTS"
}

want() { case " $SCENARIOS " in *" $1 "*) return 0;; *) return 1;; esac; }

# Runs a scenario function in a subshell with errexit; on failure records it as skipped.
# (errexit must not be inherited from a condition context, hence the explicit set +e/-e.)
scenario() { # name function
  want "$1" || return 0
  log "=== $1"
  rm -f "$WORK/reason"
  set +e
  ( set -e; "$2" )
  local rc=$?
  set -e
  if [ "$rc" != 0 ]; then
    skip "$1" "$(cat "$WORK/reason" 2>/dev/null || echo "failed (exit $rc)")"
  fi
}

# ---------------------------------------------------------------- backend

log "starting backend (nginx, 127.0.0.1:$BACKEND_PORT)"
cat > "$WORK/backend.conf" <<EOF
worker_processes $BACKEND_WORKERS;
error_log /dev/stderr warn;
events { worker_connections 16384; }
http {
    access_log off;
    keepalive_requests 1000000;
    server {
        listen 127.0.0.1:$BACKEND_PORT reuseport backlog=4096;
        location / { default_type text/plain; return 200 "ok\n"; }
    }
}
EOF
timeout 60 docker run -d --name "$C_BACKEND" --network host $(cpuset "$BACKEND_CPUS") \
  -v "$WORK/backend.conf:/etc/nginx/nginx.conf:ro" "$NGINX_IMAGE" >/dev/null
wait_200 "http://127.0.0.1:$BACKEND_PORT/" || die "backend did not start"

# ---------------------------------------------------------------- 1. nginx baseline

BASELINE_WORKERS=""
s_baseline() {
  cat > "$WORK/baseline.conf" <<EOF
worker_processes auto;
error_log /dev/stderr warn;
events { worker_connections 8192; }
http {
    log_format main '\$remote_addr - \$remote_user [\$time_local] "\$request" \$status \$body_bytes_sent "\$http_referer" "\$http_user_agent" \$request_time';
    access_log /tmp/access.log main buffer=64k flush=1s;
    keepalive_timeout 65s;
    keepalive_requests 1000;
    upstream backend {
        server 127.0.0.1:$BACKEND_PORT;
        keepalive 128;
    }
    server {
        listen $BASELINE_PORT;
        location / {
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_set_header Host \$host;
            proxy_set_header X-Real-IP \$remote_addr;
            proxy_set_header X-Forwarded-For \$remote_addr;
            proxy_set_header X-Forwarded-Proto \$scheme;
            proxy_pass http://backend;
        }
    }
}
EOF
  log "nginx-baseline: starting nginx on CPUs ${GW_CPUS:-any}"
  timeout 60 docker run -d --name "$C_BASELINE" --network host $(cpuset "$GW_CPUS") \
    -v "$WORK/baseline.conf:/etc/nginx/nginx.conf:ro" "$NGINX_IMAGE" >/dev/null || die "cannot start nginx"
  wait_200 "http://127.0.0.1:$BASELINE_PORT/" || die "nginx baseline did not answer"
  local workers
  workers=$(timeout 10 docker top "$C_BASELINE" | grep -c "nginx: worker" || true)
  echo "$workers" > "$WORK/baseline_workers"
  run_wrk nginx-baseline "$C_BASELINE" "nginx 1.28, keep-alive upstream, buffered access log, $workers workers" \
    "http://127.0.0.1:$BASELINE_PORT/"
}
scenario nginx-baseline s_baseline
timeout 60 docker rm -f "$C_BASELINE" >/dev/null 2>&1 || true
BASELINE_WORKERS=$(cat "$WORK/baseline_workers" 2>/dev/null || true)

# ---------------------------------------------------------------- Corvus

CORVUS_VERSION=""; CORVUS_WORKERS=""
s_corvus_setup() {
  log "corvus: starting $CORVUS_IMAGE on CPUs ${GW_CPUS:-any}"
  timeout 30 docker volume create "$VOL" >/dev/null
  timeout 60 docker run -d --name "$C_CORVUS" --network host $(cpuset "$GW_CPUS") \
    -e CORVUS_REDIS_PORT=$REDIS_PORT -e CORVUS_LOOPBACK=container \
    -v "$VOL:/data" "$CORVUS_IMAGE" >/dev/null || die "cannot start Corvus"
  log "corvus: waiting for healthy (≤ 120 s)"
  local st=""
  for _ in $(seq 1 60); do
    st=$(timeout 10 docker inspect -f '{{.State.Health.Status}}' "$C_CORVUS" 2>/dev/null || true)
    [ "$st" = healthy ] && break
    [ "$(timeout 10 docker inspect -f '{{.State.Running}}' "$C_CORVUS" 2>/dev/null)" = true ] || break
    sleep 2
  done
  if [ "$st" != healthy ]; then
    timeout 10 docker logs "$C_CORVUS" 2>&1 | grep -v setup_token | tail -30 >&2 || true
    die "Corvus not healthy ($st)"
  fi
  log "corvus: creating the administrator"
  local tok pw
  tok=$(timeout 10 docker logs "$C_CORVUS" 2>&1 | sed -n 's/.*setup_token=\([^ ]*\).*/\1/p' | tail -1)
  [ -n "$tok" ] || die "no setup token in the Corvus log"
  pw="Bench-$(openssl rand -hex 12)"
  create /api/setup "{\"token\":\"$tok\",\"email\":\"bench@corvus.test\",\"password\":\"$pw\"}" "d['id']" >/dev/null
  "${CURL[@]}" "$A/api/status" | js "d['version']" > "$WORK/corvus_version"
  timeout 10 docker top "$C_CORVUS" | grep -c "nginx: worker" > "$WORK/corvus_workers" || true

  log "corvus: uploading a self-signed ECDSA P-256 certificate"
  openssl req -x509 -newkey ec -pkeyopt ec_paramgen_curve:prime256v1 -nodes -days 30 \
    -subj "/CN=$H_PLAIN" -addext "subjectAltName=DNS:$H_PLAIN" \
    -keyout "$WORK/key.pem" -out "$WORK/cert.pem" 2>/dev/null
  local body cert
  body=$(python3 -c 'import json,sys; print(json.dumps({"name":"bench","certificate":open(sys.argv[1]).read(),"private_key":open(sys.argv[2]).read()}))' "$WORK/cert.pem" "$WORK/key.pem")
  cert=$(create /api/certificates "$body" "d['id']")
  # default host (also used for the HTTPS scenarios): certificate attached, HTTPS not forced
  log "corvus: creating proxy host $H_PLAIN"
  create /api/proxy-hosts "$(host_body $H_PLAIN ",\"certificate_id\":$cert")" "d['proxy_host']['id']" >/dev/null
  wait_200 "http://$H_PLAIN/" || die "Corvus host $H_PLAIN not reachable"
  touch "$WORK/corvus_ready"
}
host_body() { # domain extra-json-fields
  echo "{\"domains\":[\"$1\"],\"forward_scheme\":\"http\",\"forward_host\":\"127.0.0.1\",\"forward_port\":$BACKEND_PORT$2}"
}

s_default() {
  run_wrk corvus-default "$C_CORVUS" "defaults (detect-only exploit patterns, access log)" "http://$H_PLAIN/"
}

s_protected() {
  log "corvus-protected: IP list (1000 CIDRs), rate limit, host"
  local body list rl host n=0
  # ~1000 CIDRs that never contain the client (10.x.y.0/24 + a few IPv6 networks)
  body=$(python3 -c '
import json
items = ["10.%d.%d.0/24" % (i // 256, i % 256) for i in range(990)] + ["2001:db8:%x::/48" % i for i in range(10)]
print(json.dumps({"name": "bench-blocklist", "items": items}))')
  list=$(create /api/ip-lists "$body" "d['ip_list']['id']")
  rl=$(create /api/rate-limits '{"name":"bench-rl","requests_per_second":100000,"burst":100000}' "d['rate_limit']['id']")
  host=$(create /api/proxy-hosts "$(host_body $H_PROT ",\"block_exploits\":true,\"security_headers\":\"basic\",\"rate_limit_id\":$rl")" "d['proxy_host']['id']")
  # 30 block rules scoped to this host; none matches the benchmark request (GET / with wrk's
  # headers). String operators other than in/not_in take exactly one value.
  python3 - "$list" "$host" > "$WORK/rules.jsonl" <<'PY'
import json, sys
lst, host = sys.argv[1], int(sys.argv[2])
c = lambda f, op, v=None, **kw: dict(field=f, operator=op, **({"value": v} if v is not None else {}), **kw)
conds = [
    # path (8)
    [c("http.path", "in", ["/wp-admin/", "/wp-login.php"])],
    [c("http.path", "starts_with", ["/admin-internal"])],
    [c("http.path", "eq", ["/xmlrpc.php"])],
    [c("http.path", "ends_with", [".bak"])],
    [c("http.path", "contains", ["/.svn/"])],
    [c("http.path", "matches", ["(?i)^/(?:phpmyadmin|pma|myadmin)"])],
    [c("http.path", "matches", ["(?i)\\.(?:asp|aspx|jsp|cgi)$"])],
    [c("http.path", "starts_with", ["/api/internal"]), c("http.method", "in", ["POST", "PUT", "DELETE"])],
    # query (6)
    [c("http.query", "contains", ["cmd="])],
    [c("http.query", "contains", ["../"])],
    [c("http.query", "matches", ["(?i)(?:^|&)debug=(?:1|true)"])],
    [c("http.query", "matches", ["(?i)base64_decode|eval\\("])],
    [c("http.query", "starts_with", ["__proto__"])],
    [c("http.query", "ends_with", ["%00"])],
    # headers, user agent, referer, method, host, scheme (10)
    [c("http.header", "exists", name="X-Bench-Evil")],
    [c("http.header", "eq", ["1"], name="X-Debug-Mode")],
    [c("http.header", "contains", ["${jndi"], name="X-Api-Version")],
    [c("http.header", "matches", ["(?i)^(?:gopher|dict)://"], name="X-Original-URL")],
    [c("http.user_agent", "matches", ["(?i)sqlmap|nikto|masscan|zgrab|nuclei"])],
    [c("http.user_agent", "contains", ["python-requests/0."])],
    [c("http.referer", "contains", ["casino-spam.example"])],
    [c("http.method", "in", ["TRACE", "TRACK", "CONNECT"])],
    [c("http.host", "eq", ["internal.bench-protected.test"])],
    [c("http.scheme", "in", ["https"]), c("http.path", "starts_with", ["/insecure-only"])],
    # client IP / country (6), one with the ~1000-CIDR list
    [c("ip.src", "in", ["list:" + lst])],
    [c("ip.src", "in", ["192.0.2.0/24", "198.51.100.0/24", "203.0.113.0/24"])],
    [c("ip.src", "in", ["100.64.0.0/10"]), c("http.path", "starts_with", ["/"])],
    [c("ip.country", "in", ["KP", "IR"])],
    [c("ip.src", "not_in", ["127.0.0.0/8", "::1/128"]), c("http.path", "starts_with", ["/localonly"])],
    [c("ip.src", "in", ["169.254.0.0/16", "fe80::/10"])],
]
assert len(conds) == 30, len(conds)
for i, g in enumerate(conds):
    print(json.dumps({"name": "bench-rule-%02d" % i, "action": "block", "scope": "hosts", "host_ids": [host],
                      "expression": {"groups": [g]}}))
PY
  log "corvus-protected: creating 30 security rules"
  while IFS= read -r body; do
    create /api/security-rules "$body" "d['rule']['id']" >/dev/null
    n=$((n + 1))
  done < "$WORK/rules.jsonl"
  log "corvus-protected: $n rules created, checking the host"
  wait_200 "http://$H_PROT/" || die "protected host not reachable"
  [ "$(code "http://$H_PROT/admin-internal/x")" = 403 ] || die "rule not effective (/admin-internal not blocked)"
  [ "$(code "http://$H_PROT/.env")" = 403 ] || die "block exploits not effective (/.env not blocked)"
  "${CURL[@]}" -D - -o /dev/null "http://$H_PROT/" | grep -qi '^x-content-type-options: nosniff' || die "security headers missing"
  run_wrk corvus-protected "$C_CORVUS" "30 block rules (1000-CIDR IP list), block exploits, security headers basic, rate limit 100k r/s" "http://$H_PROT/"
}

s_https_keepalive() {
  wait_200 "https://$H_PLAIN/" || die "HTTPS not reachable"
  local tls
  tls=$(echo | timeout 10 openssl s_client -connect 127.0.0.1:443 -servername "$H_PLAIN" 2>/dev/null | sed -n 's/^New, \(.*\), Cipher is \(.*\)/\1 \2/p' | head -1 || true)
  run_wrk corvus-https-keepalive "$C_CORVUS" "corvus-default host over HTTPS, ECDSA P-256 ($tls), keep-alive" "https://$H_PLAIN/"
}

s_https_newconn() {
  wait_200 "https://$H_PLAIN/" || die "HTTPS not reachable"
  run_wrk corvus-https-newconn "$C_CORVUS" "HTTPS with Connection: close: one full TLS handshake per request (no session resumption)" \
    -H "Connection: close" "https://$H_PLAIN/"
}

s_queue() {
  log "corvus-queue: creating the access queue and host"
  local wr qj r adm
  wr=$(create /api/waitrooms '{"name":"bench-queue","capacity":1000000,"active_ttl":7200,"refresh":3600,"max_new_per_ip":0}' "d['waitroom']['id']")
  create /api/proxy-hosts "$(host_body $H_QUEUE ",\"waitroom_id\":$wr,\"waitroom_path\":\"/\"")" "d['proxy_host']['id']" >/dev/null
  qj="$WORK/qjar"
  # 1st request: signed queue cookie (cookie check, no Redis); 2nd: enters the queue → admitted
  log "corvus-queue: obtaining an admission cookie"
  for _ in $(seq 1 60); do [ "$(code -c "$qj" -b "$qj" "http://$H_QUEUE/")" = 503 ] && break; sleep 1; done
  r=$("${CURL[@]}" -D - -o /dev/null -c "$qj" -b "$qj" "http://$H_QUEUE/" | tr -d '\r' || true)
  adm=$(echo "$r" | sed -n 's/^[Ss]et-[Cc]ookie: \(corvus_a_bench_queue=[^;]*\).*/\1/p' | head -1)
  [ -n "$adm" ] || die "no admission cookie (response: $(echo "$r" | head -1))"
  [ "$(code -H "Cookie: $adm" "http://$H_QUEUE/")" = 200 ] || die "admission cookie not accepted"
  run_wrk corvus-queue "$C_CORVUS" "access queue (capacity 1M), admitted session cookie (signed-token fast path; Redis heartbeat at most hourly)" \
    -H "Cookie: $adm" "http://$H_QUEUE/"
}

s_challenge() {
  log "corvus-challenge-pass: creating the host and the challenge rule"
  local host p tok v pass
  host=$(create /api/proxy-hosts "$(host_body $H_CHAL "")" "d['proxy_host']['id']")
  create /api/security-rules "{\"name\":\"bench-challenge\",\"action\":\"challenge\",\"challenge_mode\":\"cookie\",\"scope\":\"hosts\",\"host_ids\":[$host],\"expression\":{\"groups\":[[{\"field\":\"http.path\",\"operator\":\"starts_with\",\"value\":[\"/\"]}]]}}" "d['rule']['id']" >/dev/null
  log "corvus-challenge-pass: obtaining a pass cookie"
  p=""
  for _ in $(seq 1 60); do
    p=$("${CURL[@]}" "http://$H_CHAL/" || true)
    echo "$p" | grep -q 'name="t"' && break
    sleep 1
  done
  tok=$(echo "$p" | sed -n 's/.*name="t" value="\([^"]*\)".*/\1/p' | head -1)
  [ -n "$tok" ] || die "no challenge page"
  v=$("${CURL[@]}" -o /dev/null -D - --data-urlencode "t=$tok" --data-urlencode "c=" --data-urlencode "r=/" "http://$H_CHAL/__corvus/challenge/verify" | tr -d '\r' || true)
  pass=$(echo "$v" | sed -n 's/^[Ss]et-[Cc]ookie: \(__corvus_ch=[^;]*\).*/\1/p' | head -1)
  [ -n "$pass" ] || die "no pass cookie (verify answered: $(echo "$v" | head -1))"
  [ "$(code -H "Cookie: $pass" "http://$H_CHAL/")" = 200 ] || die "pass cookie not accepted"
  [ "$(code "http://$H_CHAL/")" = 403 ] || die "challenge not active without the cookie"
  run_wrk corvus-challenge-pass "$C_CORVUS" "cookie-mode challenge on every path, requests carry a valid pass cookie" \
    -H "Cookie: $pass" "http://$H_CHAL/"
}

CORVUS_SCENARIOS="corvus-default corvus-protected corvus-https-keepalive corvus-https-newconn corvus-queue corvus-challenge-pass"
need_corvus=""
for s in $CORVUS_SCENARIOS; do want "$s" && need_corvus=1; done
if [ -n "$need_corvus" ]; then
  log "=== corvus setup"
  rm -f "$WORK/reason"
  set +e
  ( set -e; s_corvus_setup )
  set -e
  CORVUS_VERSION=$(cat "$WORK/corvus_version" 2>/dev/null || true)
  CORVUS_WORKERS=$(cat "$WORK/corvus_workers" 2>/dev/null || true)
  if [ -e "$WORK/corvus_ready" ]; then
    log "Corvus $CORVUS_VERSION ($CORVUS_WORKERS OpenResty workers)"
    scenario corvus-default s_default
    scenario corvus-protected s_protected
    scenario corvus-https-keepalive s_https_keepalive
    scenario corvus-https-newconn s_https_newconn
    scenario corvus-queue s_queue
    scenario corvus-challenge-pass s_challenge
  else
    reason="Corvus setup failed: $(cat "$WORK/reason" 2>/dev/null || echo unknown)"
    for s in $CORVUS_SCENARIOS; do want "$s" && skip "$s" "$reason"; done
  fi
fi

# ---------------------------------------------------------------- report

log "writing the report"
COMMIT=$(git -C "$(dirname "$0")" rev-parse --short HEAD 2>/dev/null || echo unknown)
WRK_VERSION=$(timeout 5 wrk -v 2>&1 < /dev/null | head -1 | cut -c1-80 || true)
KERNEL=$(uname -r)
timeout 60 python3 - "$RESULTS" "$OUT_DIR" <<PY
import json, sys
rows = [json.loads(l) for l in open(sys.argv[1]) if l.strip()]
out = sys.argv[2]
gw_vcpus = $GW_VCPUS
env = {
    "corvus_image": "$CORVUS_IMAGE", "corvus_version": "$CORVUS_VERSION", "commit": "$COMMIT",
    "nginx_image": "$NGINX_IMAGE", "wrk": "$WRK_VERSION",
    "kernel": "$KERNEL", "nproc": $NPROC, "cpu_model": "$CPU_MODEL",
    "cpus": {"wrk": "${WRK_CPUS:-any}", "backend": "${BACKEND_CPUS:-any}", "gateway": "${GW_CPUS:-any}"},
    "gateway_vcpus": gw_vcpus, "wrk_threads": $WRK_THREADS, "connections": $CONNECTIONS,
    "duration_s": $DUR_S, "warmup_s": $WARM_S,
    "corvus_openresty_workers": "$CORVUS_WORKERS", "baseline_nginx_workers": "$BASELINE_WORKERS",
}
base = next((r["rps"] for r in rows if r["scenario"] == "nginx-baseline" and not r.get("skipped")), None)
for r in rows:
    if r.get("skipped"):
        continue
    r["rps_per_vcpu"] = round(r["rps"] / gw_vcpus, 1)
    r["vs_baseline_pct"] = round(100 * r["rps"] / base, 1) if base else None
    if r["scenario"] == "corvus-https-newconn":
        r["handshakes_per_s"] = r["rps"]
json.dump({"environment": env, "results": rows}, open(out + "/bench-results.json", "w"), indent=2)

f = lambda v, fmt="{:,.0f}": "–" if v is None else fmt.format(v)
lines = [
    "## Corvus benchmark",
    "",
    "Corvus **%s** (commit \`%s\`), %d connections, %d s per scenario after a %d s warm-up." % (env["corvus_version"] or "?", env["commit"], env["connections"], env["duration_s"], env["warmup_s"]),
    "",
    "| Scenario | req/s | req/s per gateway vCPU | p50 | p90 | p99 | CPU % (mean) | Peak MB | vs nginx | Errors |",
    "|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|",
]
for r in rows:
    if r.get("skipped"):
        lines.append("| %s | skipped | | | | | | | | |" % r["scenario"])
        continue
    rps = f(r["rps"]) + (" (handshakes/s)" if "handshakes_per_s" in r else "")
    err = r["non_2xx"] + r["socket_errors"]
    lines.append("| %s | %s | %s | %s ms | %s ms | %s ms | %s | %s | %s | %s |" % (
        r["scenario"], rps, f(r["rps_per_vcpu"]), f(r["p50_ms"], "{:.2f}"), f(r["p90_ms"], "{:.2f}"), f(r["p99_ms"], "{:.2f}"),
        f(r["cpu_mean_pct"], "{:.0f}"), f(r["mem_peak_mb"], "{:.0f}"), f(r["vs_baseline_pct"], "{:.0f} %"), f"{err:,}"))
lines += ["", "Scenario details:", ""]
for r in rows:
    procs = ", ".join("%s %.0f %%" % kv for kv in (r.get("cpu_by_process_pct") or {}).items())
    lines.append("- **%s**: %s%s%s" % (r["scenario"], "SKIPPED: " if r.get("skipped") else "", r.get("note", ""), " — CPU by process: " + procs if procs else ""))
lines += [
    "",
    "Machine: %d CPUs (%s), kernel %s. CPU layout: wrk on CPUs %s (threads: %d), backend on %s, gateway on %s (%d vCPU; Corvus %s OpenResty workers, nginx baseline %s workers). CPU %% is per container (100 %% = one core) and includes Corvus' control plane and embedded Valkey; memory is the peak \`docker stats\` sample. Load generator, backend and gateway share the machine." % (
        env["nproc"], env["cpu_model"], env["kernel"], env["cpus"]["wrk"], env["wrk_threads"], env["cpus"]["backend"], env["cpus"]["gateway"], gw_vcpus,
        env["corvus_openresty_workers"] or "?", env["baseline_nginx_workers"] or "?"),
    "Backend: \`%s\` answering \`200 ok\` from memory. %s." % (env["nginx_image"], env["wrk"]),
]
open(out + "/summary.md", "w").write("\n".join(lines) + "\n")
print("\n".join(lines))
PY
if [ -n "${GITHUB_STEP_SUMMARY:-}" ]; then cat "$OUT_DIR/summary.md" >> "$GITHUB_STEP_SUMMARY"; fi
log "results in $OUT_DIR"
