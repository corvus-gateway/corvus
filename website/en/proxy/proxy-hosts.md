---
description: "Proxy hosts forward one or more domains to a service and apply TLS and protection on the way: forward target, certificates, WebSocket and security settings."
---

# Proxy hosts

A proxy host forwards requests for one or more domains to a service (the *forward target*) and
applies TLS and protection on the way. For a step-by-step first setup see
[Your first proxy host](../getting-started/first-host.md).

![Proxy hosts](../img/proxy-hosts.png){ .shot }

The list shows each host's target, certificate and the features that are on (HTTPS, WebSocket,
access queue, rate limit, exploit blocking, security headers …). The switch in **Status** turns a
host off without deleting it.

## The form, section by section

![Proxy host form](../img/proxy-host-form.png){ .shot .panel }

### Domains

Type a domain and press ++enter++ or a comma. Wildcards such as `*.example.com` work. A domain can
belong to only one proxy or redirection host.

### Forward target

**Scheme**, **Host / IP** and **Port** of your service. **WebSocket support** is on for new hosts:
the upgrade is forwarded only when a client asks for a WebSocket, so other apps are unaffected. With
it on and no upstream timeout set, the timeout is 1 hour, so idle WebSocket connections and
server-sent events (SSE) stay open.

#### Forward targets

| Target | Example | Notes |
|---|---|---|
| Another container | `shop-web` port `80` | Corvus must share a Docker network with it: add that network to the `corvus` service |
| A private or public IP | `10.3.2.39` port `8084` | |
| A service on the Docker host | `127.0.0.1` or `localhost` | Inside the container these mean the Docker host, like in NPM; no `host.docker.internal` needed. The service must listen on an address the container can reach (`0.0.0.0` or the Docker bridge), and the host firewall must allow Docker networks |

To reach services that listen only on the host's `127.0.0.1`, run Corvus with
[host networking](../getting-started/install.md#run-it-with-host-networking-optional).

### Custom locations

Send paths such as `/api` to another service. See [Custom locations](locations.md).

### TLS

![TLS section](../img/proxy-host-tls.png){ .shot .panel }

- **Certificate**: an existing certificate, **Request a new certificate (Let's Encrypt)**, or
  **None (HTTP only)**. See [Certificates](certificates.md).
- **Force HTTPS** redirects HTTP to HTTPS and sends `Strict-Transport-Security`.
- **HTTP/2** on HTTPS connections.

### Client IP

Where the real visitor IP comes from. It is used by rules, bans, rate limits and logs.

| Option | Use when |
|---|---|
| **Direct** | Visitors connect straight to Corvus (default) |
| **Cloudflare** | The site is behind Cloudflare. `CF-Connecting-IP` is trusted only from Cloudflare's official ranges |
| **Trusted proxies** | Another proxy or load balancer is in front. Set its ranges and header on the **Network** page |

!!! warning
    A wrong trusted-proxy setting lets clients fake their IP and get around rate limits and bans.

#### Behind a TCP tunnel or load balancer (PROXY protocol)

A TCP tunnel or load balancer (FRP, HAProxy, a cloud load balancer) passes the connection through
without reading it, so Corvus sees the tunnel's address for every visitor. Most of them can send
the visitor's address in a [PROXY protocol](https://www.haproxy.org/download/2.9/doc/proxy-protocol.txt)
header (v1 or v2) at the start of the connection. Corvus accepts that header on two extra ports:

1. On the **Network** page, turn on **Accept PROXY protocol**. The ports default to **8080** (HTTP)
   and **8443** (HTTPS); they can't be Corvus's own ports or a stream's port.
2. Enter the address the tunnel connects from in **Trusted PROXY protocol sources** (in the same
   section). The header is used only on connections from there, and the setting can't be turned on
   with an empty list. This is a separate list: don't add the tunnel to **Trusted proxies**, which
   are trusted for `X-Forwarded-For` (see below).
3. Publish the ports in `docker-compose.yml`. When the tunnel runs on the same host, publish them on
   loopback only:

    ```yaml
        ports:
          - '80:80'
          - '443:443'
          - '9443:9443'
          - '127.0.0.1:8080:8080'   # PROXY protocol, HTTP
          - '127.0.0.1:8443:8443'   # PROXY protocol, HTTPS
    ```

    When the tunnel or load balancer is on another machine, publish them without `127.0.0.1:` and
    let a firewall allow only that machine's address to reach them.

4. Point the tunnel at these ports with the PROXY protocol turned on.

Ports 80 and 443 stay as they are: visitors on your network and the tunnel can use Corvus side by
side. (A port that expects the header refuses connections without one, so the extra ports should be
used by the tunnel only.)

**Which address to trust.** Enter the address Corvus sees the tunnel connect from:

- **frpc (or HAProxy) in a container on a Docker network shared with Corvus**, connecting to the
  Corvus container by name: that container's address, or the network's subnet if it changes. The
  ports don't need to be published at all in this case.
- **frpc on the host, ports published on `127.0.0.1` as above**: the Docker bridge gateway
  (e.g. `172.18.0.1`, shown as the client address on the **Traffic** page before you add it).
- **A load balancer on another machine**: its address.

!!! warning "Don't trust the Docker gateway unless the ports are on loopback"
    With ports published on all addresses, Docker often makes outside connections appear to come
    from the bridge gateway (its userland proxy, IPv6 without ip6tables, Docker Desktop, rootless
    Docker). Trusting the gateway then lets anyone who reaches the port set their own address.

=== "FRP"

    `frpc.toml` on the machine next to Corvus (frp 0.52 or later):

    ```toml
    [[proxies]]
    name = "corvus-https"
    type = "tcp"
    localIP = "127.0.0.1"      # the Corvus host (or the Corvus container's name)
    localPort = 8443
    remotePort = 443
    transport.proxyProtocolVersion = "v2"

    [[proxies]]
    name = "corvus-http"
    type = "tcp"
    localIP = "127.0.0.1"
    localPort = 8080
    remotePort = 80
    transport.proxyProtocolVersion = "v2"
    ```

=== "HAProxy"

    ```
    frontend https_in
        mode tcp
        bind :443
        default_backend corvus_https

    backend corvus_https
        mode tcp
        server corvus 192.168.1.10:8443 send-proxy-v2
    ```

TLS still ends at Corvus, so certificates (including Let's Encrypt HTTP-01 through the HTTP port)
work as usual.

The address from the header is used by hosts set to **Direct**, by redirection hosts and for
unknown hosts. Hosts set to **Cloudflare** or **Trusted proxies** keep reading the address from
their header with their own rules, also when reached through the tunnel. Don't add the tunnel to
**Trusted proxies** for them unless the tunnel sets `X-Forwarded-For` itself: the tunnel passes on
whatever the visitor sent, so trusting it would let visitors choose their own address. With
Cloudflare in front of the tunnel, set the host to **Trusted proxies** with the `CF-Connecting-IP`
header and add the tunnel to **Trusted proxies** only when the tunnel's public ports accept nothing
but Cloudflare's addresses (a firewall on the tunnel server).

!!! note "Advanced configuration with real IP directives"
    While the PROXY protocol is on, Corvus sets `set_real_ip_from` and `real_ip_header` for all
    servers. An advanced configuration that sets only one of them (only `real_ip_header`, or only
    `set_real_ip_from`) is combined with the other one from Corvus. Set both, or neither.

!!! note
    [Streams](streams.md) don't accept the PROXY protocol yet.

### Protection

![Protection section](../img/proxy-host-protection.png){ .shot .panel }

| Option | What it does |
|---|---|
| **Rate limit policy** | Limits requests per IP. See [Bans & rate limits](../security/bans.md#rate-limits) |
| **Password protection** | Asks for a username and password before the site opens (HTTP Basic auth). Good for staging sites and internal tools |
| **Block known exploit paths** | Blocks common attack patterns: SQL injection, path traversal, scanner probes. **On by default** |
| **Request body inspection** | Checks JSON, form and multipart bodies for known exploit payloads: **Off**, **Log only** or **Block**. In **Block** mode, bodies that cannot be fully inspected (compressed, malformed multipart, a charset other than UTF-8 or another ASCII-compatible one, too many fields) are refused too. Bodies above the inspection size limit pass and are logged as not inspected |
| **Under attack mode** | Makes every visitor pass a browser challenge. See [Challenge & under attack](../security/challenge.md) |
| **Security headers** | Adds browser security headers (**Basic** or **Strict**). Headers your app already sends are never replaced |

### Access queue

Attach an [access queue](../access-queue.md) and choose the **Protected path** (usually `/`).

### Advanced

![Advanced section](../img/proxy-host-advanced.png){ .shot .panel }

| Option | Default | Meaning |
|---|---|---|
| **Max upload size** | 100 MB | Largest request body. `0` = unlimited. Larger uploads get `413` |
| **Upstream timeout** | 60 s (1 h with WebSocket) | How long Corvus waits for your service to answer. Empty: 60 s, or 1 h for WebSocket hosts; a value you set applies as is |
| **Gzip compression** | On | Compresses text responses (HTML, CSS, JS, JSON …). Responses that are already compressed pass unchanged |
| **Advanced nginx config** | empty | Nginx directives inserted into the host's `server` block (administrators only) |

### Notes

![Notes section](../img/proxy-host-notes.png){ .shot .panel }

Optional free text, up to 1000 characters: what the host is for, who asked for it, why it has a
special rule. The first line is shown under the domain in the list, and the search box also
searches notes. Notes are only shown in the console; they don't change the gateway configuration.

### Status

When **Enabled** is off, the host's settings are kept but it is removed from the gateway.

If you opened the console through a proxy host (for example `https://corvus.example.com`), that
host is marked **Console** in the list. Editing it shows a reminder, and disabling or deleting it
asks for confirmation first, because the console would no longer be reachable at that address. Port
9443 of the server (`https://server-ip:9443`) keeps working either way.

## How changes are applied

**Save & deploy** creates a new configuration version, tests it with `openresty -t` and applies it
without dropping connections. If the test fails, nothing changes and the error is shown. Every
version is listed under **Deployments**, where you can inspect the generated files and roll back.

![Deployments](../img/deployments.png){ .shot }
