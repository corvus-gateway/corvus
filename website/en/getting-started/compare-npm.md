---
description: How Corvus compares to Nginx Proxy Manager (NPM): what works the same, what Corvus adds (WAF, auto-bans, rate limits, access queue, traffic monitor) and where NPM is still ahead.
---

# Corvus vs Nginx Proxy Manager

Corvus is built for people who like how simple [Nginx Proxy Manager](https://nginxproxymanager.com/)
(NPM) is, but also want to see and control the traffic that reaches their services. It is not an NPM
fork. The everyday workflow is the same: add a domain, pick the target, request a certificate. Corvus
adds the protection and monitoring layer in the same console.

## Side by side

| | NPM | Corvus |
|---|---|---|
| Proxy hosts, custom locations, redirections, TCP/UDP streams | ✅ | ✅ |
| Let's Encrypt with auto-renewal | ✅ | ✅ |
| Wildcard certificates (DNS challenge) | ✅ many DNS providers | ✅ 7 providers (Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap), [saved and shared](../proxy/certificates.md) between certificates |
| Password protection (Basic auth) and IP access lists | ✅ Access lists | ✅ Password protection and [security rules](../security/rules.md) |
| Custom nginx configuration per host | ✅ | ✅ |
| Answer for unknown hosts (404, close, redirect) | ✅ Default site | ✅ [Unknown hosts](../operations/pages.md#unknown-hosts) |
| Blocking of common exploit paths | ✅ on/off switch | ✅ on by default, with [categories and logs](../security/index.md) |
| Automatic bans for IPs that keep attacking | ❌ (add CrowdSec or fail2ban) | ✅ [built in](../security/bans.md) |
| Rules on IP, country, path, header, User-Agent | ❌ | ✅ [Security rules](../security/rules.md), with a *Log* mode to test them first |
| Rate limits | ❌ (custom config) | ✅ [per host](../security/bans.md#rate-limits) |
| Browser challenge and automatic "under attack" mode | ❌ | ✅ [built in](../security/challenge.md), no third-party service |
| Access queue (waiting room) for traffic spikes | ❌ | ✅ [built in](../access-queue.md) |
| Live traffic monitor and request logs | ❌ (access log files) | ✅ [per host](../operations/traffic.md) |
| Alerts (Slack, Discord, Teams, Telegram, …) | ❌ | ✅ [built in](../operations/alerts.md) |
| Configuration versions and rollback | ❌ | ✅ every change is tested with `openresty -t` and [versioned](../proxy/proxy-hosts.md#how-changes-are-applied) |
| Users and roles | ✅ | ✅ admin / operator / viewer, [SSO (OIDC)](../operations/users.md), API tokens, audit log |
| Backups | manual (copy the data folder) | ✅ [download and restore](../operations/backup.md) in the console |
| Database | SQLite, MySQL/MariaDB, PostgreSQL | SQLite (default) or PostgreSQL |

Compared with NPM at the time of writing; NPM may have added features since.

## Where NPM is still ahead

- **More DNS providers** for wildcard certificates. If yours is not in the Corvus list, use
  HTTP validation or a [custom certificate](../proxy/certificates.md).
- **A much longer track record** and a large community. Corvus is young (1.x).
- **Smaller footprint.** Corvus also runs a traffic collector and the access-queue store, so it
  uses more memory than NPM (see [Performance](../reference/performance.md)).

## Moving from NPM

**Import from NPM** reads your NPM setup (proxy hosts, redirections, streams, certificates, access
lists) without changing anything in NPM, so you can always go back. See
[Migrate from NPM](migrate-npm.md).
