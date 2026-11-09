# Architecture

Corvus runs as **one container**. It has two halves: a data plane that handles Internet traffic
and a control plane that stores the configuration and drives the data plane.

```
                 Internet
                    │ 80 / 443 (HTTP, TLS, HTTP/2, TCP/UDP streams)
┌───────────────────▼──────────────────────────────────────────────┐
│ Data plane: OpenResty (nginx + LuaJIT)                           │
│   gateway/lua/corvus/*   bans · security rules · exploit/body    │
│                          checks · challenge · rate limits ·      │
│                          access queue · access log               │
│        │ proxy_pass                    │ queue state             │
│        ▼                               ▼                         │
│   your services              Redis (built in, 127.0.0.1)         │
└───────────────────▲──────────────────────────────────────────────┘
      writes config │ SIGHUP / reload          ▲ access log (Unix socket)
┌───────────────────┴──────────────────────────┴───────────────────┐
│ Control plane: corvus-control (Go, PID 1)                        │
│   api       HTTPS admin console + JSON API (9443)                │
│   compiler  configuration → nginx.conf + policy.json             │
│   deploy    test → switch → reload → verify → commit / rollback  │
│   acme      Let's Encrypt (HTTP-01, DNS-01)                      │
│   traffic   in-memory traffic statistics and events              │
│   autoban · alerts · feeds · sigs (signed rule bundles)          │
│   store     SQLite (default) or PostgreSQL                       │
└──────────────────────────────────────────────────────────────────┘
                    │ /data volume: database, certificates, config versions, secrets
```

## Components

| Path | Language | Role |
|---|---|---|
| `control/cmd/corvus-control` | Go | Entry point. Starts Redis and OpenResty as child processes, supervises them and serves the console |
| `control/internal/api` | Go | Admin console and HTTP API ([docs/API.md](API.md)): sessions, role checks, input validation |
| `control/internal/store` | Go | Configuration database (SQLite or PostgreSQL) with migrations |
| `control/internal/compiler` | Go | Renders the stored configuration into a self-contained nginx configuration directory and a `policy.json` for the Lua code |
| `control/internal/deploy` | Go | Applies a configuration version: `openresty -t`, atomic switch, reload, health check; rolls back on failure |
| `control/internal/acme`, `certs` | Go | Certificate issuance, renewal and storage |
| `control/internal/traffic`, `history`, `alerts`, `autoban` | Go | Collects the access log, keeps statistics, raises alerts and bans repeat attackers |
| `control/internal/sigs`, `feeds`, `geoip` | Go | Signed managed-rule bundles, IP lists and country data |
| `control/internal/seal` | Go | Encrypts secrets stored in the database |
| `gateway/lua/corvus` | Lua (OpenResty) | Per-request checks: bans, rules, body inspection, challenge, access queue, access logging |
| `web/` | TypeScript, React | Admin console, built and embedded into the Go binary |
| `deploy/` | Dockerfile, shell | Image build (OpenResty compiled from verified sources), compose files, integration tests |

## How a configuration change is applied

1. The console sends the change to the API, which validates every field.
2. In one database transaction the change is saved, the configuration is rendered into a new
   version directory, and `openresty -t` checks it.
3. If the check passes, the `current` link is switched atomically, OpenResty reloads, and the
   control plane confirms the new version is serving before committing the transaction.
4. If anything fails, the transaction is rolled back and the running configuration is unchanged.
   The console shows nginx's output.

Earlier versions can be restored from the *Deployments* page.

## How a request is handled

The order of the checks is documented for users in
[How protection works](https://corvusgateway.org/security/). In short: IP bans → security rules →
exploit-path and body checks → under-attack challenge → rate limit, password protection and access
queue → proxy to the service. Each request is logged once to the control plane over a Unix
datagram socket; if the control plane is slow, log lines are dropped instead of delaying traffic.

## Data

Everything persistent is under `/data`: the database, certificates and keys, configuration
versions, `secrets/` (the data key that encrypts stored secrets, the key that signs challenge and
access-queue cookies, and the Redis password) and `history/` (traffic statistics, security events
and request logs with bounded retention).

## Design records

Notable decisions are explained in the pull requests that introduced them; see also
[GOVERNANCE.md](../GOVERNANCE.md).
