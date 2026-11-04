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

- Corvus proxies to a variable upstream (`proxy_pass http://$corvus_upstream:port`, resolved at
  request time so a stopped upstream never blocks deployment). nginx keeps no idle upstream
  connections for such a target, so every proxied request opens a new TCP connection to the
  backend, while the baseline reuses a keep-alive pool. Part of the gap to the baseline is this
  connection setup, not the protection features.
- Corvus writes every request to its access log socket, and the control plane parses and stores
  it on the same CPUs; the baseline writes a buffered log file.
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

GitHub-hosted `ubuntu-24.04` runner (4 vCPU AMD EPYC 9V74, kernel 7.0 Azure), commit `850d8b1`,
64 connections, 20 s per scenario after a 5 s warm-up, `GET /` with a 3-byte response. wrk on
CPU 0, backend on CPU 1, gateway on CPUs 2-3 (2 vCPU, 2 workers each).

| Scenario | req/s | req/s per gateway vCPU | p50 | p90 | p99 | CPU % (mean) | Peak MB | vs nginx | Errors |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| nginx-baseline | 81,960 | 40,980 | 0.75 ms | 0.88 ms | 1.43 ms | 200 | 16 | 100 % | 0 |
| corvus-default | 13,669 | 6,835 | 4.55 ms | 5.61 ms | 17.47 ms | 205 | 69 | 17 % | 0 |
| corvus-protected | 11,919 | 5,960 | 5.32 ms | 6.81 ms | 7.83 ms | 204 | 80 | 14 % | 0 |
| corvus-https-keepalive | 12,516 | 6,258 | 5.07 ms | 6.39 ms | 7.69 ms | 204 | 83 | 15 % | 0 |
| corvus-https-newconn | 2,979 (handshakes/s) | 1,490 | 5.44 ms | 15.19 ms | 19.35 ms | 191 | 91 | 4 % | 0 |
| corvus-queue | 12,874 | 6,437 | 4.94 ms | 6.80 ms | 7.93 ms | 204 | 83 | 16 % | 0 |
| corvus-challenge-pass | 12,733 | 6,367 | 4.97 ms | 6.26 ms | 7.46 ms | 205 | 86 | 16 % | 0 |

CPU by process (Corvus scenarios): OpenResty ≈ 173 %, corvus-control ≈ 26 % (access-log
ingestion; 8 % in the handshake scenario, which serves fewer requests). The gateway CPUs are
saturated in every scenario, so req/s is the throughput limit of 2 vCPU.

Observations:

- The features themselves are cheap relative to the base request path: 30 rules with a
  1,000-CIDR list, exploit blocking, security headers and a rate limit cost about 13 % against
  `corvus-default`; the access queue fast path and a challenge pass cost about 6–7 %; TLS on
  keep-alive connections about 8 %.
- The gap to the nginx baseline is mostly the base path: Lua phases on every request
  (security-rule evaluation, bad-behaviour counting), the JSON access log sent to and parsed by
  the control plane on the same CPUs, and a new upstream TCP connection per request (variable
  `proxy_pass`, no keep-alive pool). These are the first places to look for optimisation.
- The baseline is probably limited by the single wrk thread (an earlier run on an Intel runner
  reached 150,000 req/s), so its number — and the "vs nginx" column — are conservative.
- Full TLS handshakes (ECDSA P-256, TLS 1.3) run at about 1,500 per vCPU per second.
