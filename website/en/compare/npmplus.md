---
description: "Corvus vs NPMplus: NPMplus is an NPM fork with HTTP/3, GoAccess and optional CrowdSec; Corvus is a separate project with its own WAF, automatic bans, rate limits, challenge and waiting room in the console. When to pick which."
title: Corvus vs NPMplus
keywords: [NPMplus alternative, NPMplus vs Corvus, Nginx Proxy Manager fork, NPM CrowdSec, NPM WAF]
mentions:
  - { name: NPMplus, url: "https://github.com/ZoeyVid/NPMplus" }
  - { name: Nginx Proxy Manager, url: "https://github.com/NginxProxyManager/nginx-proxy-manager" }
---

# Corvus vs NPMplus

[NPMplus](https://github.com/ZoeyVid/NPMplus) (AGPL-3.0) is a fork of Nginx Proxy Manager. It keeps
NPM's console and adds HTTP/3, GoAccess statistics and an optional CrowdSec integration. Corvus is
not a fork: it has its own console with the same workflow and builds the protection in.

## Choose NPMplus if

- You want to stay as close to **NPM's interface** as possible.
- You need **HTTP/3**.
- You want **CrowdSec**: its community blocklist and its AppSec component with the OWASP Core Rule
  Set. Note that in NPMplus the CRS check runs out-of-band: a matching request is not blocked
  itself, but an IP that triggers several rules gets banned.

## Choose Corvus if

- You want protection that works **without running CrowdSec** next to the proxy: matching requests
  are [blocked when they arrive](../security/index.md), repeat attackers are
  [banned automatically](../security/bans.md).
- You want **rate limits and country rules in the console** rather than custom nginx config:
  [rate limits](../security/bans.md#rate-limits), [security rules](../security/rules.md) on IP,
  country, path, header and User-Agent, with a log-only mode.
- You want a [browser challenge and automatic "under attack" mode](../security/challenge.md), a
  [waiting room](../access-queue.md), and [alerts](../operations/alerts.md).
- You prefer a permissive license (Apache-2.0).

## Key differences

| | NPMplus | Corvus |
|---|---|---|
| Console | NPM's | Own, same workflow |
| Import from NPM | It is NPM (migration of the data folder) | ✅ [Import from NPM](../getting-started/migrate-npm.md) |
| HTTP/3 | ✅ | ❌ |
| WAF | Optional CrowdSec AppSec (OWASP CRS, out-of-band) | ✅ built in, blocks inline |
| Automatic bans | With CrowdSec | ✅ built in |
| Rate limits, country rules | Custom nginx config | ✅ in the console |
| Statistics | GoAccess | Live traffic, request logs, visitor statistics, GoAccess-ready export |
| Challenge, waiting room, alerts | ❌ | ✅ |
| License | AGPL-3.0 | Apache-2.0 |

[Corvus vs NPM](../getting-started/compare-npm.md) · [All comparisons](index.md)
