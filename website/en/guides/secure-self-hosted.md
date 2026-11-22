---
description: "A practical checklist for securing self-hosted web apps exposed to the internet: what to expose, a reverse proxy with HTTPS, a WAF and automatic bans, rate limits, the Docker firewall trap, updates, backups and monitoring."
keywords: [self-hosted security, homelab security, reverse proxy security, expose services to the internet, WAF, fail2ban, Docker firewall]
---

# How to secure self-hosted services exposed to the internet

Once a service is reachable from the internet, it is scanned within minutes: bots try `/.env`,
`/wp-login.php`, `/.git/config`, known exploits for popular apps and default passwords. This
checklist applies whatever tools you use; the last section shows how Corvus covers each point.

## 1. Expose only what has to be public

The safest service is one the internet can't reach.

- **Only for you and your family?** Put it behind a VPN (WireGuard, Tailscale, Headscale) instead of
  opening it to the world. No WAF beats not being reachable.
- **Has to be public** (a blog, a shop, sharing links, an app for many users)? Open **only ports 80
  and 443**, to a reverse proxy. Never forward database, SSH or admin ports directly.
- **No public IP** (CGNAT, mobile connection)? Use a tunnel (Cloudflare Tunnel, Pangolin, or a small
  VPS with WireGuard) that ends at your reverse proxy.

## 2. One reverse proxy in front of everything, with HTTPS

A reverse proxy answers on 80/443 and forwards each domain to the right app, so your apps never face
the internet directly and you have one place to apply security.

- HTTPS everywhere with automatically renewed certificates (Let's Encrypt); redirect HTTP to HTTPS
  and turn on HSTS once everything works.
- Keep the apps themselves off public ports: publish them only on the Docker network the proxy uses,
  or on `127.0.0.1`.

## 3. Block known attacks before they reach the app

Most attacks against self-hosted apps are automated: the same exploit paths and payloads sent to
every IP address. A web application firewall (WAF) in the proxy stops them before they hit an app
that may not be patched yet.

- Block well-known exploit paths and payloads (SQL injection, path traversal, remote code execution,
  leaked files like `.env` and `.git`).
- When a new vulnerability is published for an app you run, add a **virtual patch** (a rule that
  blocks the exploit) until you can update.

## 4. Ban IPs that keep attacking

One blocked request is noise; an IP that sends hundreds is an attacker or a scanner. Ban it for a
while, and longer if it comes back.

- Classic setup: fail2ban or CrowdSec reading the proxy's access log.
- Exempt your own networks and monitoring so you don't lock yourself out.
- Be careful with shared IPs (mobile carriers, offices): ban by time, not forever.

## 5. Limit the rate of requests, especially logins

Rate limits slow down password guessing and scraping. Apply a stricter limit to login and API paths,
and put a browser challenge in front of login pages if bots keep hammering them.

## 6. Keep admin interfaces private

The admin panels of your proxy, NAS, router and apps should not be on the public internet.

- Allow them only from your LAN or VPN, use strong passwords and two-step verification.
- **Docker trap:** ufw and firewalld do **not** filter ports published by Docker (`ports:` in
  compose). A port `ufw status` shows as closed can still be open. Publish admin ports on
  `127.0.0.1` or a private address, or filter in the `DOCKER-USER` chain.

## 7. Update, back up, watch

- Update the apps and the proxy regularly; subscribe to their release notifications.
- Back up data **and** the proxy configuration, and test a restore once.
- Look at your traffic now and then: which hosts are attacked, which IPs are banned, which errors
  your apps return. Get an alert when something unusual happens.

## How Corvus covers this

[Corvus](../index.md) is a reverse proxy with these protections built in, managed in a web console:

| Checklist item | In Corvus |
|---|---|
| Reverse proxy and HTTPS | [Proxy hosts](../proxy/proxy-hosts.md), [Let's Encrypt with renewal](../proxy/certificates.md), forced HTTPS and HSTS |
| Block known attacks | [Exploit blocking on by default](../security/index.md), request body inspection, virtual-patch templates |
| Ban repeat attackers | [Automatic bans](../security/bans.md) with escalating duration and exempt networks, no log parsing |
| Rate limits and bots | [Per-host rate limits](../security/bans.md#rate-limits), [browser challenge and "under attack" mode](../security/challenge.md) |
| Rules and blocklists | [Security rules](../security/rules.md) on IP, country, path, header, User-Agent; [blocklists from a URL](../security/ip-lists.md) |
| Private console | Separate port, [allowed networks, MFA, SSO](../operations/users.md) |
| Watch | [Live traffic, request logs](../operations/traffic.md), [alerts](../operations/alerts.md), [backups](../operations/backup.md) |

It still can't replace the first item: if only you use a service, a VPN is the better answer.

[Install Corvus](../getting-started/install.md){ .md-button .md-button--primary }
[Compare with other reverse proxies](../compare/index.md){ .md-button }
