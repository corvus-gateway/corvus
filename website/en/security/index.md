# How protection works

Corvus Sentinel, the security engine, checks every request before it reaches your service. You don't
have to configure anything to start: **Block known exploit paths** is on for every new host, and
repeat attackers are banned automatically.

## What runs when

Each request goes through these checks in order. The first one that blocks wins.

| # | Check | Configured in |
|---|---|---|
| 1 | **IP bans**: banned addresses are refused at once | [Bans & rate limits](bans.md) |
| 2 | **Security rules**: block, allow, log or challenge, top to bottom | [Security rules](rules.md) |
| 3 | **Exploit-path blocking** and **request body inspection** | Proxy host → *Protection* |
| 4 | **Under attack mode**: every visitor must pass a challenge | [Challenge & under attack](challenge.md) |
| 5 | **Rate limit**, **password protection** and the **access queue** | Proxy host → *Protection*, [Access queue](../access-queue.md) |

An **allow** rule skips the remaining rules, exploit blocking and rate limits for that request, but
not the access queue.

Blocked visitors see the [block page](../operations/pages.md) with their IP and the time, so a user who
reports a false block can be found in the [request logs](../operations/traffic.md).

## Where to see what happened

- **Traffic monitor → Events**: blocks, rate limits, challenges, access-queue waits
- **Security rules → Rule events**: which rule matched which request
- **IP bans**: who is banned, why and until when
- [Alerts](../operations/alerts.md): repeated attack sources, exploit attempts, scanners

## Managed signatures and virtual patches

The attack patterns are updated with Corvus releases and, optionally, from a signed rule bundle
(`CORVUS_RULES_URL`). Bundles are accepted only when their signature is valid and they are newer than
the installed one.

For recently published vulnerabilities (React2Shell, wp2shell, PHP-CGI, Vite, n8n …) Corvus ships
**virtual-patch templates**: click **Add from template** on the *Security rules* page to add a
ready-made rule until you can update the affected application. Apply it as **Log** first, check for
false positives, then switch it to **Block**.
