---
description: "Corvus and CrowdSec: CrowdSec is a detection engine with community blocklists that needs a bouncer in your proxy; Corvus is a reverse proxy with bans and a WAF built in. How they differ and how to use a blocklist in Corvus."
title: Corvus and CrowdSec
keywords: [CrowdSec alternative, CrowdSec vs Corvus, fail2ban alternative, community blocklist, automatic IP ban, reverse proxy bouncer]
mentions:
  - { name: CrowdSec, url: "https://github.com/crowdsecurity/crowdsec" }
---

# Corvus and CrowdSec

[CrowdSec](https://www.crowdsec.net/) (MIT) is not a reverse proxy. It reads logs (and, with its
AppSec component, HTTP requests), detects attacks and decides which IPs to ban; separate
**bouncers** in nginx, Traefik, a firewall and so on enforce those decisions. Its best-known feature
is the **community blocklist**, built from reports by many installations.

Corvus does detection and enforcement **inside the proxy**: it blocks attack requests when they
arrive and [bans IPs that keep attacking](../security/bans.md), without log parsing or a separate
service.

## Use CrowdSec if

- You want the **community blocklist**: IPs reported by other users are blocked before they reach
  you.
- You want one engine for **many services**: SSH, mail, VPN and web, on several servers.
- You already run a proxy that has a bouncer and don't want to change it.

## Use Corvus if

- You want **bans and a WAF without another service** to install, configure and update.
- You want to see the **why** of each block in the same console:
  [security events and request logs](../operations/traffic.md), with [alerts](../operations/alerts.md).
- You also want rate limits, a [browser challenge](../security/challenge.md) and a
  [waiting room](../access-queue.md) for your web services.

## Blocklists in Corvus

Corvus can load a blocklist from a URL into an [IP list](../security/ip-lists.md#lists-from-a-url)
and refresh it in the background; [large lists](../security/ip-lists.md#large-blocklists) with
millions of entries are stored compactly and shared by all workers. Use the list in a
[security rule](../security/rules.md) to block or challenge.

Keep CrowdSec or fail2ban for SSH and other non-web services: Corvus only sees the traffic that
passes through it.

[All comparisons](index.md)
