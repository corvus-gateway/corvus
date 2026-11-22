---
description: "Corvus vs Zoraxy: both are self-hosted reverse proxies with a web console; Zoraxy is a single Go binary with an uptime monitor and plugins, Corvus adds a WAF, automatic bans, a bot challenge and a waiting room. When to pick which."
title: Corvus vs Zoraxy
keywords: [Zoraxy alternative, Zoraxy vs Corvus, homelab reverse proxy, reverse proxy web UI, Zoraxy WAF]
mentions:
  - { name: Zoraxy, url: "https://github.com/tobychui/zoraxy" }
---

# Corvus vs Zoraxy

[Zoraxy](https://zoraxy.aroz.org/) (AGPL-3.0) is a home-lab reverse proxy in Go: one binary, a web
console, ACME certificates, TCP/UDP streams, IP and country black/whitelists, rate limiting, an uptime
monitor and a plugin system. Corvus has the same kind of console and puts more weight on blocking
attacks.

## Choose Zoraxy if

- You want a **single binary** without Docker.
- You like its **uptime monitor** and extras for a home network, and want to extend it with plugins.
- Your services are mostly private and you mainly need routing and certificates.

## Choose Corvus if

- Your services face the internet and you want a **WAF**: Zoraxy does not block attack payloads today
  (its README lists an attack-prevention module as looking for a maintainer). Corvus blocks
  [exploit paths and payloads](../security/index.md) by default and
  [bans repeat attackers](../security/bans.md).
- You want a [browser challenge and "under attack" mode](../security/challenge.md), a
  [waiting room](../access-queue.md), and [alerts](../operations/alerts.md).
- You want [request logs and security events](../operations/traffic.md) per host.
- You are moving from NPM: Corvus [imports an NPM setup](../getting-started/migrate-npm.md).

## Key differences

| | Zoraxy | Corvus |
|---|---|---|
| Runs as | Single binary or container | Container |
| Console | ✅ | ✅ |
| Let's Encrypt, DNS challenge | ✅ | ✅ |
| IP / country lists, rate limits | ✅ | ✅ |
| WAF (attack payloads) | ❌ | ✅ |
| Automatic bans, challenge, waiting room | ❌ | ✅ |
| Uptime monitor | ✅ | ❌ (connection check and upstream error warnings) |
| Plugins | ✅ | ❌ |
| License | AGPL-3.0 | Apache-2.0 |

[All comparisons](index.md)
