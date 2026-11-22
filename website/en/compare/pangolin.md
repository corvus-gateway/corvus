---
description: "Corvus vs Pangolin: Pangolin exposes services through WireGuard tunnels without open ports and adds SSO in front of apps; Corvus is a reverse proxy with a WAF, bans and a waiting room for servers with a public IP. When to pick which, and using both."
title: Corvus vs Pangolin
keywords: [Pangolin alternative, Pangolin vs Corvus, Cloudflare Tunnel alternative, self-hosted tunnel, zero trust reverse proxy, WAF]
mentions:
  - { name: Pangolin, url: "https://github.com/fosrl/pangolin" }
---

# Corvus vs Pangolin

[Pangolin](https://pangolin.net/) is a tunneled reverse proxy and access platform: a small connector
next to your services opens an outbound WireGuard tunnel to a Pangolin server (usually a VPS), so you
can publish services **without a public IP or open ports**, with single sign-on, roles and PIN or
e-mail codes in front. It uses Traefik internally. The code is AGPL-3.0 with some files under a
commercial license (Enterprise edition).

The two solve different problems: Pangolin is about **reaching** and **gating** your services,
Corvus about **protecting** public web traffic.

## Choose Pangolin if

- Your server is behind **CGNAT** or you can't or don't want to open ports.
- You want **identity-based access** (SSO, users, roles) in front of private apps, a self-hosted
  alternative to Cloudflare Tunnel with Access.

## Choose Corvus if

- Your services are **public** (anyone may visit) and you want to stop attacks on them: a
  [WAF](../security/index.md), [automatic bans](../security/bans.md),
  [rate limits](../security/bans.md#rate-limits), a [browser challenge](../security/challenge.md),
  a [waiting room](../access-queue.md).
- Your server has a public IP and ports 80 and 443.

## Using both

Pangolin can carry the traffic to Corvus: point the Pangolin resource at Corvus, and let Corvus apply
protection and route to the apps. Enable the [PROXY protocol or trusted proxies](../proxy/proxy-hosts.md#client-ip)
so Corvus sees the real visitor IP.

## Key differences

| | Pangolin | Corvus |
|---|---|---|
| Needs a public IP / open ports | No (tunnel) | Yes |
| SSO and roles in front of apps | ✅ | ❌ (console only) |
| WAF | ❌ | ✅ |
| Automatic bans | With CrowdSec | ✅ built in |
| Country rules | ✅ | ✅ |
| Challenge, waiting room | ❌ | ✅ |
| License | AGPL-3.0 + commercial files | Apache-2.0 |

[All comparisons](index.md)
