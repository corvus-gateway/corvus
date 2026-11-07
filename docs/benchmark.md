# Benchmark

How much throughput and latency each Corvus feature costs, compared with a plain nginx reverse
proxy on the same machine. The benchmark is a manually triggered workflow
([`.github/workflows/bench.yml`](../.github/workflows/bench.yml)) around a script that also runs
by hand ([`deploy/bench/bench.sh`](../deploy/bench/bench.sh)).

## Method

All parts run on one machine with `--network host` (no docker-proxy in the path):

- **Backend**: `nginx:1.28-alpine` (pinned by digest) answering `200 ok\n` from memory, no access
  log, so it is never the bottleneck.
- **Load generator**: [wrk](https://github.com/wg/wrk) (`--latency`), `GET /`, 64 connections by
  default, a 5 s warm-up and then a 20 s measured run per scenario.
- **Gateway**: the nginx baseline or the Corvus image under test. CPU% and memory of the gateway
  container are sampled with `docker stats --no-stream` during the measured run (mean CPU%, peak
  memory). For Corvus this includes the control plane (which ingests the access log) and the
  embedded Valkey.

CPU layout (pinned with `taskset` / `--cpuset-cpus` so results stay comparable over time):

| CPUs | wrk | backend | gateway |
|---|---|---|---|
| 4–7 | CPU 0 (1 thread) | CPU 1 | CPUs 2..n-1 |
| ≥ 8 | CPUs 0-1 (2 threads) | CPUs 2-3 | CPUs 4..n-1 |
| < 4 | not pinned | not pinned | not pinned |

Override with `WRK_CPUS`, `BACKEND_CPUS` and `GW_CPUS`. "req/s per gateway vCPU" divides by the
number of gateway CPUs.

### Scenarios

| Scenario | What is measured |
|---|---|
| `nginx-baseline` | nginx 1.28 as a plain reverse proxy: keep-alive upstream pool, buffered access log, `worker_processes auto` |
| `corvus-default` | A Corvus proxy host with default settings over HTTP (exploit patterns detect-only, access log to the console) |
| `corvus-protected` | Another host with 30 block rules scoped to it that do not match the test traffic (path, query, header, User-Agent, referer, method, host, scheme, IP and country conditions; one IP list with 1,000 CIDRs), block exploits on, security headers `basic`, and a rate limit of 100,000 r/s (evaluated, never triggered) |
| `corvus-https-keepalive` | The `corvus-default` host over HTTPS with an uploaded self-signed ECDSA P-256 certificate, keep-alive |
| `corvus-https-newconn` | The same with `Connection: close`: one full TLS handshake per request (wrk does not resume sessions), so req/s = handshakes/s |
| `corvus-queue` | A host behind an access queue (capacity 1,000,000) with requests carrying a valid admission cookie obtained once through the normal flow; `refresh` is set to 1 h, so the run measures the signed-token fast path, not the Redis heartbeat |
| `corvus-challenge-pass` | A host with a cookie-mode challenge rule on every path; requests carry a valid pass cookie obtained once through `/__corvus/challenge/verify` |

A scenario that cannot be prepared is reported as skipped with the reason.

### Reading the numbers

- Like the baseline, Corvus proxies through a keep-alive pool per proxy host (`upstream` block
  with `keepalive`; forward host names are resolved at run time with `resolve`, so a stopped
  upstream never blocks deployment).
- Corvus records every request for the console: proxy hosts buffer the records per worker in
  Lua and send them to the control plane in batches (one datagram per ~100 requests), which
  parses them on the same CPUs; the baseline writes a buffered log file. Corvus also runs two
  Lua phases per request (bans and security rules; access log and bad-behaviour counting).
- The scenarios use one keep-alive client connection per wrk connection and a 3-byte response:
  they measure the per-request overhead, the worst case for a gateway relative to plain nginx.
- The load generator, backend and gateway share one machine. GitHub-hosted runners are shared
  VMs with varying CPU types (Intel and AMD), so absolute numbers change between runs; compare
  scenarios within one run (the "vs nginx" column and ratios between Corvus scenarios), not
  across runs. The CPU model is recorded with every result.

## Running it

From GitHub: *Actions → bench → Run workflow* (inputs `duration`, `connections`, optional `ref`).
The table is written to the job summary; `bench-results.json`, `summary.md` and the raw wrk and
`docker stats` outputs are uploaded as the `bench-results` artifact.

By hand on a Linux machine with Docker (needs `curl`, `openssl`, `python3`, `wrk`, and root or
passwordless sudo to add the `bench-*.test` names to `/etc/hosts`):

```bash
docker build -f deploy/Dockerfile -t corvus:bench .
CORVUS_IMAGE=corvus:bench DURATION=20s CONNECTIONS=64 deploy/bench/bench.sh
```

Ports 80, 443, 9443, 9180, 9181, 6390, 18080 and 18081 must be free. `SCENARIOS="corvus-default
corvus-protected"` runs a subset.

## Latest results

GitHub-hosted `ubuntu-24.04` runner (4 vCPU AMD EPYC 9V45, kernel 7.0 Azure), commit `dc6e1df`,
64 connections, 20 s per scenario after a 5 s warm-up, `GET /` with a 3-byte response. wrk on
CPU 0, backend on CPU 1, gateway on CPUs 2-3 (2 vCPU, 2 workers each).

| Scenario | req/s | req/s per gateway vCPU | p50 | p90 | p99 | CPU % (mean) | Peak MB | vs nginx | Errors |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| nginx-baseline | 97,227 | 48,613 | 0.63 ms | 0.75 ms | 1.16 ms | 199 | 17 | 100 % | 0 |
| corvus-default | 75,473 | 37,736 | 0.79 ms | 1.03 ms | 1.83 ms | 204 | 72 | 78 % | 0 |
| corvus-protected | 54,585 | 27,293 | 1.12 ms | 1.41 ms | 2.29 ms | 207 | 77 | 56 % | 0 |
| corvus-https-keepalive | 59,697 | 29,848 | 1.00 ms | 1.34 ms | 2.18 ms | 205 | 80 | 61 % | 0 |
| corvus-https-newconn | 4,348 (handshakes/s) | 2,174 | 5.88 ms | 9.68 ms | 11.62 ms | 168 | 88 | 4 % | 0 |
| corvus-queue | 57,954 | 28,977 | 1.05 ms | 1.33 ms | 2.22 ms | 206 | 84 | 60 % | 0 |
| corvus-challenge-pass | 62,901 | 31,450 | 0.96 ms | 1.23 ms | 1.94 ms | 206 | 88 | 65 % | 0 |

CPU by process (Corvus scenarios): OpenResty ≈ 190 %, corvus-control ≈ 7–9 % (access-log
ingestion; 1 % in the handshake scenario). The gateway CPUs are saturated in every scenario, so
req/s is the throughput limit of 2 vCPU.

Observations:

- The base request path reaches 78 % of the nginx baseline (17–18 % before
  [#47](https://github.com/corvus-gateway/corvus/issues/47)). The remaining gap is mostly the two
  Lua phases per request: in ablation runs with both handlers emptied, Corvus matches the baseline.
- The full protection set (30 rules with a 1,000-CIDR list, exploit blocking, security headers,
  rate limit) costs about 28 % against `corvus-default`, most of it the rule evaluation; the
  access queue fast path and a challenge pass cost 17–23 %, TLS on keep-alive connections 21 %.
- The percentages depend on the runner: on slower runners (e.g. AMD EPYC 7763) the same build
  reaches 56–64 % of the baseline for `corvus-default` and 40–44 % for `corvus-protected`.
- Full TLS handshakes (ECDSA P-256, TLS 1.3) run at about 2,200 per vCPU per second; wrk does not
  resume sessions, so this is the worst case (the gateway supports resumption through its shared
  session cache).

What changed for #47, measured as share of the nginx baseline within one run each (diagnostic
runs that switch one part off in the live configuration):

| Change | corvus-default before → after |
|---|---|
| Keep-alive upstream pool per proxy host (was: new TCP connection per request) | 18 % → 42 % |
| Access log batched per worker in Lua instead of one syslog datagram per request | 35 % → 59 % |
| One composed log variable, idle hosts skip rule evaluation, numeric IPv4 matching | 66 % → 70 % (same runner type: 74 % → 78 %) |
| Access-log parsing in corvus-control without reflection | 3.5 µs → 0.6 µs per record (Go benchmark) |
