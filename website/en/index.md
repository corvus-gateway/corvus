---
title: Corvus AWPG
hide:
  - navigation
---

# Corvus AWPG

**Corvus is a self-hosted gateway that sits in front of your web services.** It is as easy to run
as Nginx Proxy Manager (domains, forwarding and free SSL in a few clicks), and it also **watches,
protects and controls** the traffic that passes through it. Everything runs on your own server; no
traffic goes to a third-party cloud.

![Corvus demo](img/demo.gif){ .shot }

<div class="grid cards" markdown>

- :material-rocket-launch: **[Install in 5 minutes](getting-started/install.md)**

    One container, one `docker-compose.yml`, and a browser.

- :material-swap-horizontal: **[Coming from NPM?](getting-started/migrate-npm.md)**

    See [how Corvus compares](getting-started/compare-npm.md), then import proxy hosts, certificates,
    streams and access lists. NPM itself is not changed.

- :material-shield-check: **[Protection](security/index.md)**

    Attack blocking works the moment you add a host. Rules, bans and challenges are there when you need them.

- :material-account-clock: **[Access queue](access-queue.md)**

    Admit visitors up to your capacity during a launch or a flash sale and queue the rest fairly.

</div>

## What you get

| | |
|---|---|
| **Proxy & TLS** | Proxy hosts with [notes](proxy/proxy-hosts.md#notes), [custom locations](proxy/locations.md), [redirections](proxy/redirection-hosts.md), [TCP/UDP streams](proxy/streams.md), [Let's Encrypt certificates](proxy/certificates.md) with automatic renewal and saved DNS credentials, what [unknown hosts](operations/pages.md#unknown-hosts) get: 404, closed connection or a redirect |
| **Security** | Known attack patterns blocked by default, [security rules](security/rules.md), [IP and User-Agent lists](security/ip-lists.md), [browser challenge and "under attack" mode](security/challenge.md), [automatic bans](security/bans.md) |
| **Access queue** | [Capacity-based waiting room](access-queue.md) for traffic spikes |
| **Monitoring** | [Live traffic, request logs and security events](operations/traffic.md), [alerts](operations/alerts.md) to Slack, Discord, Teams, Telegram and more |
| **Operations** | Every change is tested and versioned, [backup & restore](operations/backup.md), [roles, MFA and SSO](operations/users.md), [update notices](reference/configuration.md) |

!!! tip "Safe defaults"
    You don't need to write a single rule to get started. Basic attack detection is on for every new
    host, and every advanced option stays optional.
