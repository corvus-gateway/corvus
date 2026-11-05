# Performance

Measured with the [benchmark workflow](https://github.com/corvus-gateway/corvus/blob/main/docs/benchmark.md)
on a GitHub-hosted `ubuntu-24.04` runner (4 vCPU AMD EPYC 9V45): Corvus pinned to **2 vCPU**, the
load generator and the backend on the other two, 64 keep-alive connections, `GET /` with a 3-byte
response.

| Scenario | req/s | p50 | p99 | Memory |
|---|---:|---:|---:|---:|
| nginx 1.28 reverse proxy (baseline) | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus, default host | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus, 30 rules + 1,000-CIDR list + exploit blocking + headers + rate limit | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus, HTTPS (ECDSA P-256), keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus, HTTPS, new TLS handshake per request | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus, access queue (admitted visitors) | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus, challenge rule (visitors with a pass) | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

Corvus reaches about three quarters of plain nginx's throughput; the full protection set costs about
28 % on top of that. The runner is shared and the responses are tiny, so the numbers are meant for
comparing scenarios and versions, not as capacity guarantees. Memory includes the control plane and
the built-in access-queue store.
