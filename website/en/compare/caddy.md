---
description: "Corvus vs Caddy: Caddy is a fast web server with automatic HTTPS configured in a Caddyfile; Corvus is a reverse proxy managed in a web console with a WAF, bans, rate limits and a waiting room built in. When to pick which."
title: Corvus vs Caddy
keywords: [Caddy alternative, Caddy vs Corvus, Caddy WAF, Caddy web UI, Caddy rate limit, self-hosted reverse proxy]
mentions:
  - { name: Caddy, url: "https://github.com/caddyserver/caddy" }
---

# Corvus vs Caddy

[Caddy](https://caddyserver.com/) is a web server and reverse proxy in Go (Apache-2.0), famous for
turning HTTPS on by default. You describe sites in a short `Caddyfile` or JSON; there is no official
web UI. Corvus covers the reverse-proxy part of Caddy's job and is managed in a web console, with the
security layer built in.

## Choose Caddy if

- You like configuration in a file under version control, and a two-line site config.
- You also need a **web server** (static files, PHP via FastCGI, templates), not only a proxy.
- You want **HTTP/3**, on-demand TLS, or a local CA for internal names.
- You are comfortable building Caddy with modules (`xcaddy`) for extras.

## Choose Corvus if

- You prefer a **web console** to a config file, or several people manage the proxy.
- You want protection **without compiling modules**: Caddy's rate limiting
  ([caddy-ratelimit](https://github.com/mholt/caddy-ratelimit)) and WAF
  ([coraza-caddy](https://github.com/corazawaf/coraza-caddy)) are third-party modules; in Corvus
  exploit blocking, [automatic bans](../security/bans.md), [rate limits](../security/bans.md#rate-limits),
  [security rules](../security/rules.md) and a [browser challenge](../security/challenge.md) are
  built in.
- You want to **see the traffic**: Corvus has a [live traffic monitor and request logs](../operations/traffic.md)
  and [alerts](../operations/alerts.md); Caddy writes logs for other tools to read.
- You need a [waiting room](../access-queue.md) for traffic spikes.

## Key differences

| | Caddy | Corvus |
|---|---|---|
| Configuration | Caddyfile / JSON / admin API | Web console / HTTP API |
| Automatic HTTPS | ✅ (Let's Encrypt, ZeroSSL, local CA) | ✅ (Let's Encrypt) |
| DNS challenge | Module per DNS provider | 7 providers built in |
| HTTP/3 | ✅ | ❌ |
| Static file server, FastCGI | ✅ | ❌ (only [static pages](../proxy/proxy-hosts.md#serve-a-page-instead-of-a-backend)) |
| WAF, bans, rate limits | Third-party modules | ✅ built in |
| Traffic monitor, alerts | ❌ | ✅ |
| License | Apache-2.0 | Apache-2.0 |

Caddy is lighter and has a far larger community. Corvus trades some of that for a console and the
protection layer in one container. [All comparisons](index.md)
