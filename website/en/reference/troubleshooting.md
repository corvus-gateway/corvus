---
description: "Fixes for common Corvus problems: the console won't open, 502/504/413 errors, Let's Encrypt failures, blocked visitors, streams and logs."
---

# Troubleshooting

## I can't open the console

- Use **https**: `https://<server>:9443`. The first certificate is self-signed, so the browser warns.
- Is the container running? `docker compose ps`, then `docker compose logs corvus`.
- A firewall or **Admin console allowed networks** may block you. To recover, start once with
  `CORVUS_ADMIN_ALLOW_ANY=1` and fix the list.
- Lost the setup token? `docker compose logs corvus | grep setup_token` (only shown until the first
  administrator exists).

## I forgot the password or lost two-step verification

Another administrator can reset your password or **Reset MFA** under **Users**. If you are the only
administrator, run this on the server:

```bash
docker compose exec corvus corvus-control reset-admin
```

It prints a new random password for the only active administrator, ends their sessions and deletes
the API tokens they created. Add `--disable-mfa` if you also lost your authenticator app and recovery
codes, and `--email you@example.com` if there are several administrators. Sign in, change the
password on the **Account** page and set up two-step verification again. The reset is recorded in
the audit log. It works while Corvus is running, with SQLite and PostgreSQL.

## 502 Bad Gateway

Corvus can't reach the forward target.

- Container name as target: Corvus must share a Docker network with it.
- `127.0.0.1` / `localhost` as target means the **Docker host**. The service must listen on
  `0.0.0.0` (or the Docker bridge), not only `127.0.0.1`, and the host firewall must allow Docker
  networks. Services bound to `127.0.0.1` only need
  [host networking](../getting-started/install.md#run-it-with-host-networking-optional).
- `upstream sent too big header`: very large cookies or headers from the app. Add
  `proxy_buffer_size 32k;` to the host's **Advanced nginx config**.

## 504 Gateway Timeout

The service took longer than the **Upstream timeout** (60 s by default). Raise it under
*Advanced* for slow endpoints such as reports or exports.

## 413 Request Entity Too Large

The upload is larger than **Max upload size** (100 MB by default). Raise it under *Advanced*
(`0` = unlimited). With the [hardened install](hardened.md) also mind the `/tmp` size.

## Let's Encrypt fails { #lets-encrypt-fails }

- **HTTP-01**: the domain must point at this server and port 80 must be reachable from the internet
  (not only 443).
- **Wildcard** domains need DNS-01.
- Use **staging** while testing.

For the errors below, the certificate's row shows a one-line hint with a link to this page (hover
the status for the full error). The *certificate issuance failed* alert includes the same hint.

### Rate limit reached { #acme-rate-limited }

`urn:ietf:params:acme:error:rateLimited`. Let's Encrypt limits how often the same set of names is
issued (5 per week) and how many validations may fail per hour. Stop retrying, fix the cause, and
wait until the time given in the error. Test with **staging**, which has much higher limits.

### Nameservers answer SERVFAIL or REFUSED { #dns01-servfail }

`propagation: time limit exceeded … NS ns1.example.net. returned SERVFAIL for _acme-challenge…`.
The domain's authoritative nameservers answer the TXT query with an error. Usual causes: broken
DNSSEC (a DS record at the registrar that no longer matches the zone), NS records at the registrar
that point to a provider which does not host the zone, or a provider that refuses some networks.
Check with `dig +dnssec _acme-challenge.example.com TXT @ns1.example.net` or an online DNSSEC
checker, and fix the delegation at the registrar.

### TXT record not visible in time { #dns01-propagation }

`propagation: time limit exceeded`, or `unauthorized … Incorrect TXT record … _acme-challenge`.
The record was created but the nameservers did not serve it within 10 minutes. Make sure the
domain's NS records point to the provider of the DNS credential (not a previous provider), then
**Renew now**. Some providers are slow; a retry usually works.

### DNS provider refused the credential { #dns01-credential }

`error presenting token: …` with `401`, `403`, `unauthorized` or `zone could not be found`. The API
token is wrong, expired or lacks permission. For Cloudflare, use an API token with
**Zone → DNS → Edit** (and **Zone → Read**) for this zone. Update it under
**Certificates → DNS credentials**.

### Nameservers not found { #dns01-lookup }

`could not find the start of authority`, `NS call failed`, `initial recursive nameserver`. Corvus
could not look up the domain's nameservers: the domain is not registered or delegated yet, or the
Corvus server cannot reach a DNS resolver (check `/etc/resolv.conf` and outbound port 53).

### CAA record forbids Let's Encrypt { #acme-caa }

`urn:ietf:params:acme:error:caa`. A CAA record on the domain (or a parent domain) allows other CAs
only. Add `0 issue "letsencrypt.org"` (and `issuewild` for wildcards) or remove the CAA record.

### Name not allowed { #acme-rejected-name }

`urn:ietf:params:acme:error:rejectedIdentifier`. Let's Encrypt only issues for public domain names:
not IP addresses, `localhost` or internal names such as `nas.lan`. Use a public domain, or upload
your own certificate.

### Account email rejected { #acme-email }

`urn:ietf:params:acme:error:invalidContact`. The email's domain does not accept mail or is a
placeholder such as `example.com`. Use a real address.

### Domain has no DNS records { #acme-dns }

`urn:ietf:params:acme:error:dns`, for example *NXDOMAIN looking up A*. Let's Encrypt could not
resolve the name. Check the spelling and that its A/AAAA record exists in public DNS, not only on
your LAN.

### Port 80 not reachable { #acme-port-80 }

`urn:ietf:params:acme:error:connection`, *Timeout during connect (likely firewall problem)*. Open
port 80 in the firewall and forward it to Corvus on the router; check that the A/AAAA records hold
your public address (an AAAA record pointing elsewhere is a common cause). Can't open port 80? Use
DNS-01.

### HTTP-01 reached another server { #acme-wrong-server }

`urn:ietf:params:acme:error:unauthorized … Invalid response from http://…/.well-known/acme-challenge/…`.
The request arrived, but not at Corvus: the domain points to another server, another proxy answers
port 80, or a CDN (for example Cloudflare's proxy) redirects or caches the path. Point the domain at
Corvus or use DNS-01.

### Let's Encrypt not reachable { #acme-ca-unreachable }

`get directory at 'https://acme-v02.api.letsencrypt.org/directory'`. The Corvus server has no
outbound HTTPS access or cannot resolve names. Check DNS, the firewall and any outbound proxy.

## A change is rejected { #config-rejected }

The gateway tests every change before applying it; a rejected change leaves the running
configuration untouched. The error shows nginx's output, and **Deployments** keeps it.

### Port already in use { #deploy-port-in-use }

*The gateway kept the previous configuration* (`reload_rejected`). The test passed, but nginx could
not apply the change, usually because a stream port (or 80/443) is already used by another program
on the server or by another container with host networking. Free the port or choose another;
`docker compose logs corvus` shows the exact `bind()` error.

### Advanced configuration error { #deploy-advanced-config }

`[emerg] unknown directive …`, `… is not allowed here` or `invalid number of arguments` in
`hosts/<id>.conf`. A host's **Advanced nginx config** contains something nginx does not accept. The
number in the file name is the host's ID; fix or remove the snippet on that host.

## Outbound connections fail { #outbound-errors }

List feeds, the country database, managed rules updates, alert channels, Cloudflare ranges, SSO
discovery and the NPM import connect from the Corvus server. Their errors carry a hint:

| Error contains | Meaning and fix |
| --- | --- |
| `no such host`, `server misbehaving` | The name does not resolve: check the URL and the server's DNS. |
| `connection refused` | Nothing listens there: check host, port and that the service runs. |
| `i/o timeout`, `deadline exceeded` | No answer: check the outbound firewall or proxy. |
| `x509:` | Certificate not trusted: check the URL; use a certificate from a public CA. |
| `is not allowed`, `non-public address` | Corvus does not connect to loopback, link-local or (for list feeds) private addresses. |
| `HTTP 401`, `HTTP 403` | Credentials refused: check the token, webhook URL or password. |
| `HTTP 404`, `404 Not Found` | Wrong URL. For SSO, enter the issuer without `/.well-known/openid-configuration`. |
| `Error 1045` | NPM import: MySQL user name or password is wrong. |
| `Error 1049` | NPM import: the MySQL database does not exist (NPM's default is `npm`). |
| `did not match the issuer URL` | SSO: the issuer differs from the provider's `issuer`, often by a trailing slash. Copy it exactly. |

## A visitor says they were blocked

The block page shows the visitor's IP and the time. Search for the IP in
**Traffic monitor → Request logs** around that time; the row shows which rule or ban blocked them. Unban under
**IP bans**, or add an **Allow** rule / exempt network.

## API clients get 429 with "cookie-check"

The host has an [access queue](../access-queue.md) and the client didn't send the admission cookie.
This is the queue asking it to wait, not an error. Protect only browser paths, or move the API to a
host without a queue.

## Streams don't answer

On a bridge network the port must also be published in `docker-compose.yml`. See
[Streams](../proxy/streams.md#publish-the-port).

## `sendfile() failed (9: Bad file descriptor)` in the logs

Harmless: the backend answered before it read the whole upload. The request itself completed.

## Where are the logs?

```bash
docker compose logs -f corvus
```

The console's **Deployments** page shows the output of a rejected configuration.

Still stuck? [Open an issue](https://github.com/corvus-gateway/corvus/issues) with the Corvus
version and the relevant log lines (remove domains and IPs you don't want to share).
