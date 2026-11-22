---
description: "Corvus vs BunkerWeb: BunkerWeb is an nginx-based WAF with ModSecurity and the OWASP Core Rule Set, Kubernetes and Swarm autoconf, and a PRO edition; Corvus is a simpler reverse proxy console with its own WAF, all free. When to pick which."
title: Corvus vs BunkerWeb
keywords: [BunkerWeb alternative, BunkerWeb vs Corvus, open source WAF, ModSecurity, OWASP CRS, self-hosted WAF, BunkerWeb PRO]
mentions:
  - { name: BunkerWeb, url: "https://github.com/bunkerity/bunkerweb" }
---

# Corvus vs BunkerWeb

[BunkerWeb](https://www.bunkerweb.io/) (AGPL-3.0, with a paid PRO edition) is a WAF built on nginx.
It is the closest project to Corvus in purpose, and it has the broader security feature set:
ModSecurity with the OWASP Core Rule Set, bad-behaviour bans, rate and connection limits, many
antibot options (JavaScript, captcha, reCAPTCHA, hCaptcha, Turnstile…), country, ASN and DNSBL
blocking, and threat sharing. It is configured with environment variables, Docker/Swarm labels,
a Kubernetes ingress controller or its web UI.

## Choose BunkerWeb if

- You need **ModSecurity and the OWASP Core Rule Set** with paranoia levels, for example for a
  compliance checklist.
- You deploy with **Kubernetes, Swarm or Docker labels**.
- You want the widest set of switches and are happy to tune CRS false positives.
- You are fine with some features in PRO (among them monitoring and reporting, load balancing,
  custom pages, maintenance mode, backups to S3, OpenID Connect and wildcard certificates).

## Choose Corvus if

- You want a **reverse proxy console first**, with protection that is on by default and tuned for few
  false positives, rather than a WAF you configure.
- You want **everything free**: in Corvus the [traffic monitor](../operations/traffic.md),
  [custom and maintenance pages](../proxy/proxy-hosts.md#serve-a-page-instead-of-a-backend),
  [several backends](../proxy/proxy-hosts.md#several-backends-round-robin),
  [backups](../operations/backup.md), [SSO for the console](../operations/users.md) and
  [wildcard certificates](../proxy/certificates.md) have no paid tier.
- You need a [waiting room](../access-queue.md) for traffic spikes, or
  [automatic "under attack" mode](../security/challenge.md#under-attack-mode).
- You are moving from NPM: Corvus [imports an NPM setup](../getting-started/migrate-npm.md).

## Key differences

| | BunkerWeb | Corvus |
|---|---|---|
| WAF engine | ModSecurity + OWASP CRS | Own signatures (Corvus Sentinel), virtual-patch templates |
| Configuration | Env vars, labels, Kubernetes, web UI | Web console, HTTP API |
| Bans, rate limits, country rules | ✅ | ✅ |
| Bot challenge | ✅ many options, incl. third-party captchas | ✅ proof-of-work or cookie, no third party |
| Waiting room | ❌ | ✅ |
| Monitoring, reports | Basic in free, more in PRO | ✅ all free |
| Kubernetes | ✅ | ❌ |
| License | AGPL-3.0 + PRO | Apache-2.0, no paid edition |

[All comparisons](index.md)
