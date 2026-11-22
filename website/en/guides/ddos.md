---
description: "What a self-hosted server can and cannot do against DDoS attacks and traffic floods: application-layer floods vs. volumetric attacks, rate limits, bans, challenges, a waiting room, and when you need a CDN."
keywords: [DDoS protection self-hosted, HTTP flood, layer 7 DDoS, rate limiting, under attack mode, waiting room, traffic spike]
---

# Handling traffic floods and DDoS on a self-hosted server

"DDoS protection" covers two very different problems. Knowing which one you have tells you what can
help.

## Two kinds of floods

| | Volumetric (layer 3/4) | Application layer (layer 7) |
|---|---|---|
| What | Huge amounts of packets (UDP floods, SYN floods, amplification) | Many HTTP requests that look valid: page loads, searches, logins |
| Fills | Your internet connection or the provider's network | Your app's CPU, database or worker pool |
| Can your server stop it? | **No.** The traffic is already too much for the link before any software sees it | **Yes, largely**, with a reverse proxy in front |
| What helps | Your hosting provider's DDoS filtering, or a CDN/proxy service in front | Rate limits, bans, challenges, caching, a waiting room |

So be suspicious of any self-hosted tool that promises "DDoS protection" without saying which kind.

## Against application-layer floods

1. **Rate limits per IP** keep one client from sending more than a person ever would.
2. **Automatic bans** remove IPs that keep tripping limits or attack patterns.
3. **A browser challenge** for everyone during an attack ("under attack" mode) stops bots that don't
   run a real browser, while people pass after a second. Best if it turns on automatically when
   requests per second spike.
4. **Allow your own clients** (monitoring, apps, APIs) so they keep working while the challenge is on.

## Against real traffic spikes

Not every flood is an attack: a launch, a ticket sale or a post that goes viral can bring more
genuine visitors than your app can handle. Blocking them is wrong; a **waiting room** admits as many
visitors as your app can serve and queues the rest in order, so the site stays usable instead of
falling over for everyone.

## Doing it with Corvus

| Need | In Corvus |
|---|---|
| Rate limits, automatic bans | [Bans & rate limits](../security/bans.md) |
| Challenge, automatic "under attack" mode | [Challenge & under attack](../security/challenge.md): turns on by itself above a requests-per-second or blocked-share threshold and sends an alert |
| Waiting room | [Access queue](../access-queue.md): capacity-based, fair order, no external service |
| Seeing what happens | [Live traffic monitor](../operations/traffic.md) and [alerts](../operations/alerts.md) |

Corvus runs on your server, so like any self-hosted software it can't absorb a volumetric attack.
If you expect those, put your provider's DDoS filtering or a CDN in front; Corvus reads the real
client IP from [Cloudflare and other proxies](../proxy/proxy-hosts.md#client-ip) and keeps
working behind them.
