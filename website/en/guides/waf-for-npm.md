---
description: "Nginx Proxy Manager has no WAF, automatic bans or rate limits. Four ways to add them: CrowdSec or fail2ban next to NPM, NPMplus, custom nginx config, or moving to a proxy with protection built in, with the trade-offs of each."
keywords: [Nginx Proxy Manager WAF, NPM security, NPM fail2ban, NPM CrowdSec, NPM rate limit, secure Nginx Proxy Manager, NPM alternative with WAF]
mentions:
  - { name: Nginx Proxy Manager, url: "https://github.com/NginxProxyManager/nginx-proxy-manager" }
  - { name: CrowdSec, url: "https://github.com/crowdsecurity/crowdsec" }
  - { name: NPMplus, url: "https://github.com/ZoeyVid/NPMplus" }
---

# How to add a WAF, bans and rate limits to Nginx Proxy Manager

Nginx Proxy Manager (NPM) makes proxying and certificates easy, but its security features end at
access lists, Basic auth and a "Block common exploits" switch. It has no automatic bans, no rate
limits in the console, no country rules and no view of what was blocked. Here are four ways to close
that gap.

## 1. fail2ban or CrowdSec next to NPM

Both read NPM's access logs and ban IPs that misbehave.

- **fail2ban**: write filters (regular expressions) for the log format and a jail; bans go into
  the host firewall. With Docker, ban in the `DOCKER-USER` chain, or the ban won't affect published
  ports.
- **CrowdSec**: install the engine, a collection for NPM's logs and a firewall bouncer. You also
  get its community blocklist.

Good: keeps NPM as it is. Costs: another service to maintain, bans happen after the fact (from
logs), and behind Cloudflare or another proxy a firewall ban on the client IP has no effect.

## 2. Custom nginx configuration in NPM

In a host's **Advanced** tab you can add `limit_req` for rate limits or `deny` rules. It works, but
the rate-limit zones must be defined in a custom `http` include, nothing is visible in the console,
and a typo can stop nginx from reloading.

## 3. NPMplus

[NPMplus](../compare/npmplus.md) is an NPM fork with an optional CrowdSec integration, HTTP/3 and
GoAccess. A good choice if you want to stay on NPM's interface.

## 4. A proxy with protection built in

If you are adding three tools to NPM, a proxy that has them may be less work.
[Corvus](../index.md) keeps NPM's workflow and builds in what the steps above add:

| Missing in NPM | In Corvus |
|---|---|
| WAF | [Exploit paths and request bodies blocked by default](../security/index.md) |
| Automatic bans | [Built in](../security/bans.md), enforced in the proxy, so it also works behind Cloudflare |
| Rate limits | [Per host, in the console](../security/bans.md#rate-limits) |
| Country, header, User-Agent rules | [Security rules](../security/rules.md) with a log-only mode |
| Bot challenge | [Challenge and "under attack" mode](../security/challenge.md) |
| Seeing what was blocked | [Traffic monitor and events](../operations/traffic.md), [alerts](../operations/alerts.md) |

You can try it without giving up NPM: [Import from NPM](../getting-started/migrate-npm.md) copies
proxy hosts, redirections, streams, certificates and access lists and leaves NPM untouched.
[Corvus vs NPM](../getting-started/compare-npm.md) lists where NPM is still ahead (more DNS
providers, a longer track record, a smaller footprint).

Other options with a built-in WAF, such as BunkerWeb and SafeLine, are in the
[comparison](../compare/index.md).
