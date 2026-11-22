---
description: "Frequently asked questions about Corvus AWPG: what it is, license and cost, how it compares to Nginx Proxy Manager, the WAF and auto-bans, DDoS, requirements, migration and what it does not do."
faq: true
---

# Frequently asked questions

## About Corvus

### What is Corvus AWPG?

Corvus AWPG (Active Web Protection Gateway) is a self-hosted reverse proxy with a built-in web
application firewall. It sits in front of your web services on ports 80 and 443, gets free Let's
Encrypt certificates, forwards each domain to the right service, and blocks attacks on the way:
exploit blocking, security rules, automatic IP bans, rate limits, a browser challenge and a
waiting room for traffic spikes. You manage it in a web console, like Nginx Proxy Manager. It runs
as one Docker container (OpenResty + Go) on amd64 and arm64.

### Is Corvus free and open source?

Yes. Corvus is licensed under Apache-2.0 and every feature is free. There is no paid edition, no
feature held back for a "pro" version and no account to create. The source code is on
[GitHub](https://github.com/corvus-gateway/corvus).

### Who is Corvus for?

Self-hosters, homelab users and small teams who expose web apps to the internet (Nextcloud,
Vaultwarden, Immich, Home Assistant, a blog, a shop, an API) and want HTTPS and protection in front
of them without assembling a reverse proxy, a WAF, fail2ban and a log viewer by hand.

### Does any of my traffic go to a third-party cloud?

No. Requests, logs and statistics stay on your server. The only outgoing connections are to Let's
Encrypt (certificates), to GitHub twice a day to check for a new release (can be turned off, and
nothing about your installation is sent), and the alert channels and optional rule bundle you set
up yourself.

## Compared with other tools

### How is Corvus different from Nginx Proxy Manager?

The everyday workflow is the same: add a domain, pick the target, request a certificate. Corvus
adds what NPM leaves to add-ons: automatic bans, security rules (IP, country, path, header,
User-Agent), rate limits, a browser challenge and "under attack" mode, a waiting room, a live
traffic monitor and alerts. It is not an NPM fork, and it can import an existing NPM setup. See
[Corvus vs NPM](getting-started/compare-npm.md).

### Which reverse proxy should I choose?

It depends on what you need. Caddy and Traefik are excellent if you prefer configuration files or
Docker labels; Pangolin if your server has no public IP; BunkerWeb or SafeLine if you want an
OWASP-CRS or semantic WAF; CrowdSec next to any proxy for community blocklists. Corvus is for people
who want a web console and the protection in one container. See
[Corvus compared with other reverse proxies](compare/index.md).

### Does Corvus replace fail2ban or CrowdSec?

For web traffic through Corvus, mostly yes: it bans IPs that keep attacking by itself, from its own
detections, with escalating ban times and exempt networks, and needs no log parsing. CrowdSec adds
something Corvus does not have: a shared blocklist built from many users' reports. You can load
external blocklists into Corvus [IP lists](security/ip-lists.md), and fail2ban or CrowdSec are still
the right tools for SSH and other non-web services.

### Is Corvus a WAF? Does it use ModSecurity or the OWASP Core Rule Set?

Corvus includes a web application firewall, but not ModSecurity or the OWASP Core Rule Set. Its
engine, Corvus Sentinel, checks request paths, query strings, headers and bodies against its own
signatures for common attacks (SQL injection, path traversal, remote code execution, scanners) and
ships virtual-patch templates for recent vulnerabilities. It is tuned for few false positives so it
can be on by default. If you need full OWASP CRS coverage with paranoia levels, look at BunkerWeb
or a ModSecurity/Coraza setup.

## Security

### Can Corvus protect against DDoS attacks?

Against application-layer floods, partly: rate limits, automatic bans, the browser challenge and
"under attack" mode stop bots and floods of HTTP requests, and the waiting room keeps your app
usable during real traffic spikes. Against volumetric attacks that fill your internet connection,
no self-hosted software can help; that needs your provider or a CDN in front. See
[Handling traffic floods and DDoS](guides/ddos.md).

### How does Corvus block bots without a CAPTCHA?

With a challenge the browser solves on its own: a small JavaScript proof-of-work, or a cookie
check. It needs no CAPTCHA provider and no third-party service. Visitors who pass get a signed cookie
and are not asked again for 24 hours by default. See
[Challenge & under attack](security/challenge.md).

### Can I block countries?

Yes. Security rules can match the visitor's country (a country database is built in) and block,
challenge or only log the request. See [Security rules](security/rules.md).

### Is the admin console exposed to the internet?

The console runs on its own port (9443), separate from the sites. Limit it to trusted networks with
**Admin console allowed networks**, publish it on a private address only, or use a VPN. It supports
two-step verification, single sign-on (OIDC) and roles. Note that ufw and firewalld do not filter
ports published by Docker; see [Install](getting-started/install.md).

## Running Corvus

### What do I need to run Corvus?

A Linux server (amd64 or arm64) with Docker and the Compose plugin, and ports 80 and 443. A small
VPS, a NAS (TrueNAS SCALE, Unraid) or a 64-bit Raspberry Pi is enough: in the benchmark Corvus used
about 70–90 MB of memory. See [Install](getting-started/install.md).

### How do I move from Nginx Proxy Manager?

Use **Import from NPM** in the console. It reads proxy hosts, redirections, streams, certificates and
access lists from NPM without changing NPM, so you can switch back. See
[Migrate from NPM](getting-started/migrate-npm.md).

### Can Corvus forward to services that don't run in Docker?

Yes. Corvus forwards to any address it can reach: other containers, services on the same server
(through the Docker host address), or other machines in your network. See
[How Corvus works](getting-started/how-it-works.md).

### Does Corvus work behind Cloudflare or a tunnel?

Yes. Corvus takes the real client IP from Cloudflare and other trusted proxies, and from the PROXY
protocol when a TCP tunnel or load balancer is in front. If your server has no public IP, run a
tunnel (for example Cloudflare Tunnel, Pangolin or a VPS with WireGuard) and point it at Corvus.

### Does Corvus support wildcard certificates?

Yes, with a DNS challenge. Saved DNS credentials are supported for Cloudflare, DigitalOcean, DuckDNS,
Gandi, GoDaddy, Hetzner and Namecheap. You can also upload your own certificates. See
[Certificates](proxy/certificates.md).

## What Corvus does not do

### Does Corvus work as a Kubernetes ingress or read Docker labels?

No. Corvus is configured in its web console or through its HTTP API, not from Kubernetes resources or
Docker labels. If you want routes to appear automatically from container labels, Traefik or Caddy
with a Docker plugin fit better.

### Does Corvus support HTTP/3?

Not yet. Corvus serves HTTP/1.1 and HTTP/2 over TLS.

### Can Corvus add a login (single sign-on) in front of my apps?

Corvus can protect a host with a password (Basic auth), an IP allow list or a challenge. It does not
include a forward-auth or identity provider for the apps behind it; single sign-on is for the Corvus
console only. Put Authelia, Authentik or the app's own login in front if you need SSO for your apps.
