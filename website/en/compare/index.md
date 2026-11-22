---
description: "An honest comparison of self-hosted reverse proxies and WAFs: Nginx Proxy Manager, NPMplus, Caddy, Traefik, Zoraxy, BunkerWeb, SafeLine, CrowdSec, Pangolin and Corvus. Licenses, web UI, built-in security, and which one fits which setup."
title: Corvus compared with other reverse proxies and WAFs
keywords: [reverse proxy comparison, Nginx Proxy Manager alternative, self-hosted WAF, Caddy vs Traefik, BunkerWeb alternative, SafeLine alternative, open source WAF, homelab reverse proxy]
mentions:
  - { name: Nginx Proxy Manager, url: "https://github.com/NginxProxyManager/nginx-proxy-manager" }
  - { name: NPMplus, url: "https://github.com/ZoeyVid/NPMplus" }
  - { name: Caddy, url: "https://github.com/caddyserver/caddy" }
  - { name: Traefik, url: "https://github.com/traefik/traefik" }
  - { name: Zoraxy, url: "https://github.com/tobychui/zoraxy" }
  - { name: BunkerWeb, url: "https://github.com/bunkerity/bunkerweb" }
  - { name: SafeLine, url: "https://github.com/chaitin/SafeLine" }
  - { name: CrowdSec, url: "https://github.com/crowdsecurity/crowdsec" }
  - { name: Pangolin, url: "https://github.com/fosrl/pangolin" }
---

# Corvus compared with other reverse proxies and WAFs

There are many good self-hosted reverse proxies, and they make different trade-offs. This page is
written by the Corvus maintainer, so read it with that in mind; we try to say where the others are
the better choice. Facts were checked against each project's own documentation and repository in
November 2026. Projects change, so check their docs for anything that matters to you, and
[tell us](https://github.com/corvus-gateway/corvus/discussions) if something here is out of date.

## Quick answer

| If you want… | Look at |
|---|---|
| A web console **and** a WAF, bans, rate limits, bot challenge, waiting room and traffic monitor in one container, all free | **Corvus** |
| The simplest web console for proxying and certificates, nothing more | [Nginx Proxy Manager](../getting-started/compare-npm.md) |
| NPM's console with HTTP/3, GoAccess and CrowdSec | [NPMplus](npmplus.md) |
| A config file, automatic HTTPS and HTTP/3, no UI | [Caddy](caddy.md) |
| Routes created from Docker labels or Kubernetes resources | [Traefik](traefik.md) |
| A single-binary console with an uptime monitor, home-lab style | [Zoraxy](zoraxy.md) |
| ModSecurity with the OWASP Core Rule Set, Kubernetes/Swarm autoconf | [BunkerWeb](bunkerweb.md) |
| A semantic-analysis WAF engine | [SafeLine](safeline.md) |
| Community blocklists next to the proxy you already run | [CrowdSec](crowdsec.md) |
| Exposing services **without a public IP or open ports**, with SSO in front | [Pangolin](pangolin.md) |

## Side by side

✅ built in · 🧩 with a plugin, add-on or custom config · 💲 paid edition only · ❌ not available

| | Corvus | NPM | NPMplus | Caddy | Traefik | Zoraxy | BunkerWeb | SafeLine | Pangolin |
|---|---|---|---|---|---|---|---|---|---|
| License | Apache-2.0 | MIT | AGPL-3.0 | Apache-2.0 | MIT | AGPL-3.0 | AGPL-3.0 + PRO | GPL-3.0 repo, engine binary-only | AGPL-3.0 + commercial files |
| Paid edition | none | none | none | none | Traefik Hub | none | PRO | Lite / Pro | Enterprise |
| Managed in a web UI | ✅ | ✅ | ✅ | ❌ | read-only dashboard | ✅ | ✅ | ✅ | ✅ |
| Docker labels / Kubernetes | ❌ | ❌ | ❌ | 🧩 | ✅ | ❌ | ✅ | 🧩 | ❌ |
| Let's Encrypt, auto-renewal | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| HTTP/3 | ❌ | ❌ | ✅ | ✅ | ✅ | ❓ | ❓ | ❓ | ❓ |
| WAF (attack signatures) | ✅ own engine | ❌ | 🧩 CrowdSec AppSec | 🧩 Coraza | 💲 / 🧩 | ❌ | ✅ ModSecurity + CRS | ✅ semantic engine | ❌ |
| Automatic IP bans | ✅ | ❌ | 🧩 CrowdSec | ❌ | 🧩 | ❌ | ✅ | ❓ | 🧩 CrowdSec |
| Rate limits | ✅ | 🧩 custom config | 🧩 custom config | 🧩 | ✅ | ✅ | ✅ | ✅ | ❓ |
| Country blocking | ✅ | ❌ | 🧩 | 🧩 | 🧩 | ✅ | ✅ | ❓ | ✅ |
| Bot challenge without a CAPTCHA service | ✅ | ❌ | ❌ | ❌ | 🧩 | ❌ | ✅ | ✅ | ❌ |
| Waiting room for traffic spikes | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ (edition unclear) | ❌ |
| Request log / traffic view in the UI | ✅ | ❌ | ✅ GoAccess | ❌ | ❌ | ❓ | ✅ | ✅ | ✅ access logs |
| SSO in front of your apps | ❌ | ❌ | ❌ | 🧩 forward_auth + IdP | 🧩 ForwardAuth + IdP | ❓ | 💲 | ❓ | ✅ |
| Tunnel, no open ports needed | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |

❓ = not clearly documented. CrowdSec is not a reverse proxy, so it is not in the table; it adds
detection and community blocklists to the proxy you run ([details](crowdsec.md)).

## Where Corvus is the wrong choice

- **You configure everything as code** (Docker labels, Kubernetes, GitOps). Corvus is managed in a
  console or its [HTTP API](../reference/api.md); Traefik or Caddy fit better.
- **You need HTTP/3**, or single sign-on in front of your apps. Corvus has neither today.
- **You need the OWASP Core Rule Set** with paranoia levels for compliance. Corvus uses its own,
  smaller signature set tuned to be on by default.
- **Your server has no public IP.** Corvus needs ports 80 and 443 reachable; use a tunnel such as
  Pangolin or Cloudflare Tunnel in front, or instead.
- **You want a long track record.** Corvus is young (1.x) and has one maintainer; NPM, Caddy and
  Traefik have years of production use and large communities.

## Where Corvus stands out

- **Everything is free.** No feature is held back for a paid edition.
- **Protection is on from the first host**: exploit blocking, automatic bans and request-body
  inspection need no setup, rules and challenges are there when you need them.
- **The parts usually bolted on are one console**: WAF, bans, rate limits, bot challenge, an automatic
  "under attack" mode, a waiting room, live traffic and request logs, alerts.
- **Easy to try from NPM**: [import](../getting-started/migrate-npm.md) your NPM setup without
  changing NPM.
