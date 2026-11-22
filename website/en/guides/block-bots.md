---
description: "How to block bad bots, vulnerability scanners and brute-force attempts on a self-hosted website: what they look like in the logs, and five layers from exploit blocking to a browser challenge, without a CAPTCHA service."
keywords: [block bots, vulnerability scanner, brute force, bad bots, self-hosted, reverse proxy, bot protection, CAPTCHA alternative]
---

# Blocking bad bots and vulnerability scanners on a self-hosted site

Look at the access log of any public server and most requests are not people: crawlers, uptime
checkers, AI scrapers, and scanners trying paths your site never had.

## What they look like

| Kind | Typical requests | Harm |
|---|---|---|
| Vulnerability scanners | `/.env`, `/.git/config`, `/wp-login.php`, `/phpmyadmin/`, `/cgi-bin/`, exploit payloads in the query string | Find an unpatched app or a leaked file |
| Brute force | Hundreds of `POST /login` from one or a few IPs | Guess passwords |
| Scrapers | Thousands of page requests, often with a generic or fake User-Agent | Load, copied content |
| Floods | Many requests per second to one URL | Slow or unavailable site |

Good crawlers (search engines, your monitoring) are not the problem; the goal is to stop the rest
without blocking them.

## Five layers, from cheap to strict

1. **Block exploit paths and payloads.** Requests for `/.env` or containing `../../etc/passwd` are
   never legitimate. Block them outright; this alone removes most scanner traffic.
2. **Ban repeat offenders.** An IP that triggers many blocks or many `404`s in a few minutes is a
   scanner. Ban it for a while, longer each time it comes back.
3. **Rate-limit sensitive paths.** A few login attempts per minute per IP is plenty for a person.
4. **Filter by User-Agent, IP list or country** where it fits your audience: known bad-bot
   User-Agents, Tor exit nodes, community blocklists, or countries you never serve. Start in
   log-only mode to see what you would block.
5. **Challenge what is left.** A browser challenge (a short JavaScript computation or a cookie check)
   stops clients that don't run a real browser, without a CAPTCHA service. Use it on login pages, or
   for the whole site while you are under attack.

Keep non-browser clients in mind: apps, API clients, feed readers and monitoring can't solve a
challenge. Exempt their paths or IPs.

## Doing it with Corvus

In [Corvus](../index.md) these layers are settings, not scripts:

| Layer | Where |
|---|---|
| Exploit paths and payloads | On for every new host: Proxy host → *Protection* ([how it works](../security/index.md)) |
| Ban repeat offenders | [Automatic bans](../security/bans.md#automatic-bans), with *Also ban suspected scanners* for 404 floods |
| Rate limits | [Per host](../security/bans.md#rate-limits) |
| User-Agent, IP lists, countries | [Security rules](../security/rules.md) and [lists](../security/ip-lists.md), with presets for bad-bot User-Agents and Tor exit nodes |
| Challenge | [Challenge rule or under attack mode](../security/challenge.md), no third-party service |

The [traffic monitor](../operations/traffic.md) shows each block and its reason, so you can check
for false positives, and [alerts](../operations/alerts.md) tell you about repeated attack sources.

## Without Corvus

With plain nginx you can get far with `map` blocks for paths and User-Agents, `limit_req` for rate
limits and fail2ban reading the access log; CrowdSec adds community blocklists and bouncers for
several proxies. It is more pieces to maintain, but it works. See
[Corvus compared with other reverse proxies](../compare/index.md).
