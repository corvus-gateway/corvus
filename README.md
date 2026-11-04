<div align="center">

**English** · [한국어](docs/readme/README_ko.md) · [简体中文](docs/readme/README_zh.md) · [日本語](docs/readme/README_ja.md) · [Русский](docs/readme/README_ru.md) · [Français](docs/readme/README_fr.md) · [Deutsch](docs/readme/README_de.md) · [Español](docs/readme/README_es.md)

<br>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="assets/brand/corvus-logo-dark.svg">
  <img src="assets/brand/corvus-logo.svg" alt="Corvus" width="420">
</picture>

### Active Web Protection Gateway

<a href="https://github.com/corvus-gateway/corvus/releases/latest"><img src="https://img.shields.io/github/v/release/corvus-gateway/corvus?label=version&color=2ea44f&style=for-the-badge"></a>
<a href="LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg?style=for-the-badge"></a>
<a href="https://github.com/corvus-gateway/corvus/pkgs/container/gateway"><img src="https://img.shields.io/badge/ghcr.io-corvus--gateway%2Fgateway-24292f.svg?style=for-the-badge&logo=docker&logoColor=white"></a>
<a href="https://github.com/corvus-gateway/corvus/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/corvus-gateway/corvus/ci.yml?branch=main&label=CI&style=for-the-badge"></a>

<img src="https://img.shields.io/badge/OpenResty-1.31-00b140?style=flat-square">
<img src="https://img.shields.io/badge/Go-1.26-00add8?style=flat-square&logo=go&logoColor=white">
<img src="https://img.shields.io/badge/React-18-61dafb?style=flat-square&logo=react&logoColor=black">
<img src="https://img.shields.io/badge/i18n-8_languages-8250df?style=flat-square">
<img src="https://img.shields.io/badge/platform-amd64_|_arm64-555?style=flat-square&logo=docker&logoColor=white">
<a href="https://scorecard.dev/viewer/?uri=github.com/corvus-gateway/corvus"><img src="https://api.scorecard.dev/projects/github.com/corvus-gateway/corvus/badge?style=flat-square"></a>
<a href="https://www.bestpractices.dev/projects/16315"><img src="https://www.bestpractices.dev/projects/16315/badge"></a>
<a href="https://www.bestpractices.dev/projects/16315/baseline-1"><img src="https://www.bestpractices.dev/projects/16315/baseline"></a>

</div>

Corvus is a self-hosted gateway that sits in front of your web services. It is as easy to run as
Nginx Proxy Manager — domains, forwarding and free SSL in a few clicks — and it also **watches,
protects and controls** the traffic that passes through it: live monitoring, attack blocking,
rate limiting and an access queue for traffic spikes. Everything runs on your own server; no
traffic is sent to a third-party cloud.

- [Quick Setup](#quick-setup)
- [Full Setup](#full-setup)
- [Screenshots](#screenshots)

## Project Goal

Small teams usually end up with a reverse proxy for TLS, a separate WAF they never tune, and no
idea what is happening when a launch or an attack hits. Corvus puts these in one box with safe
defaults: basic attack detection works the moment a host is added, without writing a single rule,
and every advanced option stays optional. The goal is a gateway you can install in five minutes
and still trust on the day your traffic goes ten times higher.

Corvus is not a fork of Nginx Proxy Manager. It is a new implementation (OpenResty data plane,
Go control plane, React console) that can **import an existing NPM setup**.

## Features

**Proxy & TLS**
- Proxy hosts with wildcards, WebSocket, HTTP/2, forced HTTPS and custom Nginx snippets
- Redirection hosts: send domains to another domain (301/302/303/307/308, optionally keeping the path), with the same TLS and protection
- Free Let's Encrypt certificates (HTTP-01, or DNS-01 via Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap) with automatic renewal, or upload your own (and replace it in place when it is renewed)
- Streams: forward TCP/UDP ports (databases, SSH, MQTT, game servers) to another host, with optional TLS termination for TCP
- Real client IP behind Cloudflare or trusted proxies; optional password protection per host
- Security header presets per host (basic / strict: nosniff, frame and referrer policy, report-only CSP, HttpOnly/SameSite/Secure cookie flags), never overriding what the app already sends
- Gzip compression of text responses (on by default), and per-host upload size limit and upstream timeout
- Every change is validated with `openresty -t`, applied atomically and can be rolled back

**Corvus Sentinel — security engine**
- Security rules: match IP, IP list, country, path, query, method, header or User-Agent list → block, allow, log or challenge, globally or per host
- IP and User-Agent lists kept up to date from a URL (e.g. Tor exit nodes, bad-bot lists; presets included), refreshed in the background and redeployed only when they change
- Browser challenge without third-party services: a JavaScript proof of work (adjustable difficulty) or a lightweight cookie check, with a signed, IP-bound pass cookie
- "Under attack" mode per host: challenge every visitor — switched on by hand, or automatically while requests per second or the share of blocked requests exceed a threshold (with alerts when it turns on and off)
- Request body inspection for JSON, form and multipart bodies (log or block)
- One-click **virtual patches** for recent CVEs (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Managed signature updates, accepted only when signed and newer (no tampering, no downgrade)
- Automatic IP bans for repeat offenders, with escalation and exemptions; optional immediate bans on bad behaviour (too many 4xx responses per IP and site)
- Rate limits with a dry-run mode; exploit-path blocking out of the box

**Access Queue**
- Admit visitors up to a capacity and queue the rest fairly during spikes
- Admission rate, fail-open / fail-closed policy, live active/waiting counts, emergency release
- Waiting page localized in 8 languages

**Monitoring & alerts**
- Live requests/s, status codes, p50/p95/p99 latency, per-host and per-path breakdown
- Request log search and CSV/JSON export, security events, persistent history with retention and log rotation
- Alerts to Slack, Mattermost, Discord, Teams, Telegram or signed webhooks
- Prometheus `/metrics`

**Administration**
- Roles, MFA (TOTP + recovery codes), OIDC single sign-on, API tokens
- Admin-console IP allowlist, audit log, deployment history
- SQLite by default or PostgreSQL; admin console in 8 languages with light and dark themes

**Hardened by default**
- Non-root containers with a read-only root filesystem and no Linux capabilities
- Redis protected by a generated password and a minimal ACL
- OpenResty built from signature-verified sources

## Screenshots

<p align="center">
  <img src="assets/screenshots/dashboard.png" alt="Dashboard" width="900">
</p>

| Traffic monitor | Security rules |
|:---:|:---:|
| <img src="assets/screenshots/traffic.png" alt="Traffic monitor"> | <img src="assets/screenshots/security-rules.png" alt="Security rules"> |
| **Proxy host** | **Access queues** |
| <img src="assets/screenshots/proxy-host.png" alt="Proxy host"> | <img src="assets/screenshots/access-queues.png" alt="Access queues"> |

## Quick Setup

1. [Install Docker](https://docs.docker.com/engine/install/) with the Compose plugin.
2. Create a `docker-compose.yml` file like this:

```yml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    restart: unless-stopped
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
    volumes:
      - ./data:/data
```

This is the whole installation: gateway, admin console and access-queue store run in this one
container. See [Full Setup](#full-setup) for options.

3. Bring up your stack by running

```bash
docker compose up -d
```

4. Read the one-time setup token

```bash
docker compose logs corvus | grep setup_token
```

5. Log in to the admin console at `https://<your-server>:9443`, enter the token and create the
   first administrator. The console uses a self-signed certificate until you configure your own.

> [!TIP]
> Point your domain's DNS at the server, add a proxy host, and request a Let's Encrypt
> certificate in the same form.

### Upgrading

```bash
docker compose pull
docker compose up -d
```

Settings, certificates and history in `./data` are kept; database changes are applied
automatically on start.

## Full Setup

### Ports

| Port | Purpose |
|---|---|
| 80 / 443 | Gateway — proxied traffic |
| 9443 | Admin console and API — allow only trusted networks (firewall, or *Network → Admin console allowed networks*) |

### Data and backups

All state lives in `./data`: the settings database, generated configuration history,
certificates, traffic history and `secrets/`. **Back up `secrets/`** — it holds the keys that
encrypt MFA secrets and DNS tokens in the database. The container runs as the unprivileged user
`10001` and takes ownership of `./data` on first start.

**Backup from the console:** *Data retention → Backup & restore → Download backup* (administrators,
signed-in session only — API tokens cannot download backups) creates
`corvus-backup-<version>-<timestamp>.tar.gz` with:

- `corvus.db` — a consistent snapshot of the SQLite settings database (taken with `VACUUM INTO`
  while Corvus keeps running),
- `secrets/`, `certs/` and `rules/` (installed managed-rule bundle),
- `manifest.json` — format version, Corvus version, creation time, database engine and the
  SHA-256 of every file.

Generated configuration versions (`nginx/`), traffic history and request logs (`history/`), the
country database (`geoip/`) and `run/` are left out: they are regenerated or downloaded again.
With PostgreSQL (`CORVUS_DB_URL`) the archive holds no database, only a `README.txt` — back the
database up with `pg_dump` and keep the dump together with the archive (its encrypted fields need
`secrets/data_key`). **The archive contains private keys and the data key: store it encrypted.**
Every download is recorded in the audit log.

**Restore from the console:** *Restore from a backup* uploads an archive (at most 200 MB). It is
checked — format version, SHA-256 of every file, no absolute paths, `..`, links or unknown entries,
same database engine — and its summary (version, date, file and certificate counts) is shown.
*Restore on next restart* stages it in `./data/restore-pending/`; nothing changes until you restart
the container:

```bash
docker compose restart
```

On start, before the database is opened, Corvus moves the current `corvus.db`, `secrets/`,
`certs/`, `rules/` and `nginx/` to `./data/restore-backup-<timestamp>/` and puts the backup in
their place (the configuration is then re-rendered from the restored database). To roll back, stop
the container and move those files back. If the staged files fail their checks the restore is not
applied, the current state is kept and the staged files are moved to `./data/restore-failed-<timestamp>/`
(see `docker compose logs`). Traffic history is not touched by a restore.

**Restore by hand:** the archive layout is the data directory layout. Before the first start,
extract it into an empty `./data` (the extra `manifest.json` is harmless):

```bash
mkdir data && tar -xzf corvus-backup-….tar.gz -C data
docker compose up -d
```

### Hardened install

[`deploy/docker-compose.hardened.yml`](deploy/docker-compose.hardened.yml) runs the same image
with a read-only root filesystem, no Linux capabilities and no privilege escalation. Create the
data directory for user `10001` first:

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### Configuration

Set these under `environment:` in `docker-compose.yml`.

| Variable | Description |
|---|---|
| `CORVUS_ADMIN_HOSTS` | Extra names/IPs for the admin certificate, e.g. `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | Use PostgreSQL instead of SQLite, e.g. `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | Use an external Redis/Valkey for the access queue (several gateways sharing one queue); with `CORVUS_REDIS_PASSWORD`. Default: built-in |
| `CORVUS_IMPORT_DIR` | Directory with read-only NPM volumes for *Import from NPM* |
| `CORVUS_RULES_URL` | Optional URL of a signed managed-rules bundle |
| `CORVUS_ADMIN_ALLOW_ANY=1` | Recovery switch if you locked yourself out with the admin allowlist |
| `CORVUS_REDIS_PORT` | Port of the built-in access-queue store on 127.0.0.1 (default `6379`); change it with `network_mode: host` if the host already uses 6379 |
| `CORVUS_RESOLVER` | DNS server for forward hosts. Default: Docker's DNS, or the first `nameserver` of `/etc/resolv.conf` with host networking |
| `CORVUS_LOOPBACK=container` | Make `127.0.0.1`/`localhost` forward hosts point at the container itself instead of the Docker host |
| `CORVUS_IPV6` | `auto` (default): also listen on IPv6 when the system has it · `off`: IPv4 only · `on`: always listen on IPv6 and resolve forward hosts to IPv6 addresses too (IPv6-only upstreams) |
| `PUID` / `PGID` | User and group the container runs as and owns `./data` (default `10001`), e.g. your NAS user |
| `CORVUS_ACME_DIRECTORY` | ACME directory URL of another CA instead of Let's Encrypt, e.g. a private step-ca `https://ca.internal/acme/acme/directory` |
| `LEGO_CA_CERTIFICATES` | PEM file(s) of a private CA the ACME client should trust (mount it into the container), e.g. `/data/ca.pem` |

### Forward hosts

A proxy host can forward to:

- **another container** by name, e.g. `xoshop-web` port `80` — Corvus must share a Docker network
  with it (add that network to the `corvus` service);
- **a private or public IP**, e.g. `10.3.2.39` port `8084`;
- **a service on the Docker host itself** as `127.0.0.1` or `localhost`, like in NPM. Inside a
  container these mean the Docker host, so no `host.docker.internal` is needed. The service must
  listen on an address the container can reach (`0.0.0.0` or the Docker bridge, not only
  `127.0.0.1`), and a host firewall must allow connections from Docker networks.

To reach services that listen only on the host's `127.0.0.1`, or to keep an NPM setup that used
`network_mode: host` unchanged, run Corvus with host networking: replace `ports:` and `networks:`
with `network_mode: host` (and set `CORVUS_REDIS_PORT` if 6379 is taken on the host).

### Streams (TCP/UDP)

*Streams* forward a TCP and/or UDP port to a forward host (same rules as above), optionally
terminating TLS for TCP with one of your certificates. With the default bridge network, also
publish each stream port in `docker-compose.yml`, then apply it with `docker compose up -d`:

```yaml
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
      - '5432:5432'       # stream: PostgreSQL (TCP)
      - '1194:1194/udp'   # stream: OpenVPN (UDP)
```

With `network_mode: host` nothing else is needed. Ports Corvus uses itself (80, 443, 9443 and its
internal ports) cannot be used for streams.

### Migrating from Nginx Proxy Manager

*Import from NPM* in the console copies **proxy hosts, redirection hosts, streams, certificates** (Let's
Encrypt and custom, including Cloudflare DNS tokens) and **access lists** (users and IP rules) from
an existing NPM installation. It only reads NPM's files and database — NPM itself is not changed,
so you can go back at any time.

Not imported: 404 hosts, custom locations and advanced
configuration that uses NPM-specific variables. The preview lists everything it skips.

**Check which database your NPM uses.** Open NPM's `docker-compose.yml`:

- no `DB_MYSQL_*` / `DB_POSTGRES_*` variables, and `data/database.sqlite` exists → **SQLite** (NPM's default)
- `DB_MYSQL_HOST`, `DB_MYSQL_USER`, … and usually a separate `db` service → **MySQL / MariaDB**
- `DB_POSTGRES_*` → PostgreSQL is not supported by the import; recreate the hosts in the console

Both need NPM's `data` and `letsencrypt` directories mounted read-only into Corvus (certificate
files live there):

```yml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    restart: unless-stopped
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

Use the host paths of NPM's `./data` and `./letsencrypt` volumes. NPM and Corvus both need ports
80/443, so they can't run at the same time.

#### NPM with SQLite (default)

```bash
cd /path/to/npm && docker compose down        # stop NPM (frees 80/443)
cd /path/to/corvus && docker compose up -d     # start Corvus with the mounts above
```

In the console, open *Import from NPM*, keep **SQLite**, click *Preview*, choose what to import
and click *Import selected*. If the preview reports `permission denied`, make the files readable for
Corvus (it runs as uid 10001): `sudo chmod o+rx /path/to/npm/data && sudo chmod o+r /path/to/npm/data/database.sqlite`.

#### NPM with MySQL / MariaDB

Corvus has to reach NPM's database while it imports, so stop only NPM's proxy service and keep its
database running:

```bash
cd /path/to/npm && docker compose stop app     # the NPM service that owns 80/443 (often "app")
cd /path/to/corvus && docker compose up -d

# connect Corvus to NPM's Docker network for the import
docker network ls | grep -i npm                # e.g. npm_default
docker network connect npm_default $(docker compose ps -q corvus)
```

In *Import from NPM*, choose **MySQL / MariaDB** and enter NPM's database settings: host = the
database service name (usually `db`), port `3306`, and the values of `DB_MYSQL_NAME`,
`DB_MYSQL_USER` and `DB_MYSQL_PASSWORD`. The connection is read-only and the password is not
stored. Then *Preview* → *Import selected*.

When the import is done:

```bash
docker network disconnect npm_default $(docker compose ps -q corvus)
cd /path/to/npm && docker compose down         # stop NPM's database too
```

#### After the import

- Check the imported hosts, then remove `CORVUS_IMPORT_DIR` and the two `/import` mounts and run
  `docker compose up -d` again.
- Certificates keep renewing in Corvus. Hosts that used an unsupported DNS provider are imported
  without automatic renewal — the preview warns about them.
- **Going back:** `docker compose down` in Corvus, then `docker compose up -d` in NPM.

## Performance

Measured with the [benchmark workflow](docs/benchmark.md) on a GitHub-hosted `ubuntu-24.04`
runner (4 vCPU AMD EPYC 9V45): Corvus pinned to **2 vCPU**, wrk and the backend on the other two,
64 keep-alive connections, `GET /` with a 3-byte response.

| Scenario | req/s | p50 | p99 | Memory |
|---|---:|---:|---:|---:|
| nginx 1.28 reverse proxy (baseline) | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus, default host | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus, 30 rules + 1,000-CIDR list + exploit blocking + headers + rate limit | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus, HTTPS (ECDSA P-256), keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus, HTTPS, new TLS handshake per request | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus, access queue (admitted visitors) | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus, challenge rule (visitors with a pass) | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

On the base request path Corvus reaches about three quarters of plain nginx's throughput
(keep-alive upstream pools, access log batched per worker); the full protection set — 30 rules
with a 1,000-CIDR list, exploit blocking, security headers and a rate limit — costs about 28 %
on top of that. **Caveat:** a shared CI runner with the load generator on the same machine
and tiny responses — the numbers are conservative and meant for comparing scenarios and
versions, not as capacity guarantees. Memory includes the control plane and embedded Valkey.

## Development

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only, no OpenResty
```

| Path | Contents |
|---|---|
| `gateway/` | Data plane: OpenResty + Lua (rules, body inspection, access queue) |
| `control/` | Control plane (Go): settings DB, config generation, deploy/rollback, API |
| `web/` | Admin console (React + Vite + TypeScript) |
| `deploy/` | Dockerfile, verified OpenResty build, Compose files, integration tests |
| `tools/` | Load-test and rule-bundle signing tools |

## Security

Please report vulnerabilities privately — see [SECURITY.md](SECURITY.md).

## License

[Apache License 2.0](LICENSE) © 2026 Jikwang Moon.
IP-to-country data by [DB-IP](https://db-ip.com) (CC BY 4.0). See [NOTICE](NOTICE).
