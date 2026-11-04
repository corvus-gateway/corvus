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
- Free Let's Encrypt certificates (HTTP-01, or DNS-01 via Cloudflare) with automatic renewal, or upload your own
- Real client IP behind Cloudflare or trusted proxies; optional password protection per host
- Every change is validated with `openresty -t`, applied atomically and can be rolled back

**Corvus Sentinel — security engine**
- Security rules: match IP, IP list, country, path, query, method or header → block, allow or log, globally or per host
- Request body inspection for JSON, form and multipart bodies (log or block)
- One-click **virtual patches** for recent CVEs (React2Shell, wp2shell, PHP-CGI, Vite, n8n …)
- Managed signature updates, accepted only when signed and newer (no tampering, no downgrade)
- Automatic IP bans for repeat offenders, with escalation and exemptions
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

### Migrating from Nginx Proxy Manager

Mount NPM's `data` and `letsencrypt` directories read-only and point Corvus at them:

```yml
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

Then open *Import from NPM* in the console. Hosts, certificates and access lists are carried
over. Stop NPM afterwards so the two do not compete for ports 80/443 and certificate renewals.

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
