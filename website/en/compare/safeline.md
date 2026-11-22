---
description: "Corvus vs SafeLine: SafeLine is a self-hosted WAF with a semantic-analysis engine distributed as prebuilt images and paid editions; Corvus is a fully open-source (Apache-2.0) reverse proxy with a WAF built in. When to pick which."
title: Corvus vs SafeLine
keywords: [SafeLine alternative, SafeLine vs Corvus, open source WAF, self-hosted WAF, Chaitin SafeLine, fully open source WAF]
mentions:
  - { name: SafeLine, url: "https://github.com/chaitin/SafeLine" }
---

# Corvus vs SafeLine

[SafeLine](https://waf.chaitin.com/) by Chaitin is a self-hosted WAF and reverse proxy with a web
console. Its detection engine uses **semantic analysis** of requests rather than regular-expression
signatures. It also has rate limiting, an anti-bot challenge, a password challenge, dynamic page
encryption and a waiting room. Commercial editions (Lite, Pro) exist next to the free one.

## About "open source"

The SafeLine repository is GPL-3.0 and contains the management service, SDKs and tools, but the
detection engine and the proxy are shipped as **prebuilt container images**, not as source in the
repository (as of November 2026). If you need to read, audit or build every part yourself, that
matters. Every part of Corvus is in [its repository](https://github.com/corvus-gateway/corvus)
under Apache-2.0, and release images are signed and built from it.

## Choose SafeLine if

- You want its **semantic engine**, which is designed to catch obfuscated payloads that signatures
  miss.
- You want its extras such as dynamic HTML/JS encryption, or its plugins for ingress-nginx, Kong or
  Traefik.

## Choose Corvus if

- You want a **fully open-source** gateway you can audit and build.
- You want **all features free**, with no app or feature limits tied to an edition.
- You want a **complete reverse-proxy console** in the NPM style: [proxy hosts](../proxy/proxy-hosts.md),
  [redirections](../proxy/redirection-hosts.md), [TCP/UDP streams](../proxy/streams.md),
  [certificates with DNS challenge](../proxy/certificates.md), [import from NPM](../getting-started/migrate-npm.md).
- You want [live traffic, request logs, visitor statistics](../operations/traffic.md) and
  [alerts](../operations/alerts.md) to Slack, Discord, Teams, Telegram and more.

## Key differences

| | SafeLine | Corvus |
|---|---|---|
| Source code | Management in the repo, engine as prebuilt images | Everything in the repo |
| License | GPL-3.0 repo, paid Lite/Pro editions | Apache-2.0, no paid edition |
| WAF engine | Semantic analysis | Signatures and virtual patches |
| Rate limits, bot challenge, waiting room | ✅ (check which edition) | ✅ |
| Redirection hosts, TCP/UDP streams, NPM import | Not its focus | ✅ |
| Runs as | Several containers | One container |

[All comparisons](index.md)
