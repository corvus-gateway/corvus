---
description: "How Corvus works, in plain words: it answers on ports 80 and 443 and passes each request to the right web server behind it. Includes moving an existing web server behind Corvus."
---

# How Corvus works

## The idea in one picture

Without Corvus, visitors talk to your web server directly:

```text
visitor ──► :80 / :443  your web server
```

With Corvus, Corvus answers on ports 80 and 443 instead, checks each request and passes it on to
the web server it is meant for:

```text
                         ┌──────────────────────────────┐
visitor ──► :80 / :443 ─►│ Corvus                       │──► blog         (container, port 2368)
                         │  · HTTPS certificates        │──► Nextcloud    (container, port 80)
                         │  · blocks attacks, bans bots │──► old website  (this server, port 8080)
                         │  · access queue, monitoring  │
                         └──────────────────────────────┘
                              admin console :9443
```

Visitors still type `https://blog.example.com`; they never see the ports behind Corvus. This kind
of program is called a **reverse proxy**.

## Words used in this documentation

| Word | Meaning |
|---|---|
| **Gateway** | The part of Corvus that answers visitors on ports 80 and 443. |
| **Proxy host** | One rule in Corvus: "requests for this domain go to that web server". You create one per website or app. |
| **Service behind Corvus**, **forward target**, **upstream**, **backend** | The web server or app that actually produces the page, e.g. a WordPress container or an nginx you already run. Corvus sends it the requests and passes its answers back. |
| **Forward host / port** | Where that service can be reached *from Corvus*: a container name, an IP address, or `127.0.0.1` for a service on the same server. |
| **Admin console** | The web interface where you set all of this up, on port 9443. Visitors never use it. |

Only one program at a time can listen on a port. Because Corvus takes 80 and 443, the services
behind it listen on other ports (or only inside Docker), and Corvus forwards to them.

## I already have a web server on ports 80 and 443 { #i-already-have-a-web-server-on-ports-80-and-443 }

Move your web server to another port, start Corvus on 80 and 443, and point a proxy host at the new
port. Your site stays the same; only the port it listens on changes.

### 1. Move your web server to another port

Pick a free port, for example **8080**. Your web server only needs plain HTTP from now on: Corvus
handles HTTPS and the certificates.

=== "nginx"

    In your site configuration, change the `listen` lines:

    ```nginx
    server {
        listen 8080;            # was: listen 80;
        # remove the "listen 443 ssl" server block, or move it to another port
        server_name example.com;
        ...
    }
    ```

    Then `sudo nginx -t && sudo systemctl reload nginx`.

=== "Apache"

    In `ports.conf`, change `Listen 80` to `Listen 8080` (and remove `Listen 443`), and in your
    virtual host change `<VirtualHost *:80>` to `<VirtualHost *:8080>`. Then
    `sudo apachectl configtest && sudo systemctl reload apache2`.

=== "A Docker container"

    Change the published port in its `docker-compose.yml`, e.g. `'8080:80'` instead of `'80:80'`, and
    remove `'443:443'`. Then `docker compose up -d`.

    Even better: don't publish a port at all and put the container on a Docker network it shares with
    Corvus. See [Connect Corvus to your other containers](first-host.md#before-you-start).

Check that the site answers on the new port: `curl -I http://127.0.0.1:8080`.

!!! warning "The web server must accept connections from Docker"
    Corvus runs in a container, so it reaches `127.0.0.1:8080` through Docker. If your web server
    listens only on `127.0.0.1` (for example `listen 127.0.0.1:8080;`), make it listen on all
    addresses (`listen 8080;`) and protect port 8080 with your firewall, or run Corvus with
    [host networking](install.md#run-it-with-host-networking-optional).

### 2. Install and start Corvus

Ports 80 and 443 are now free. Follow [Install](install.md).

### 3. Add a proxy host for your site

In the admin console, **Proxy hosts → Add proxy host**:

| Field | Value |
|---|---|
| Domains | `example.com` (and `www.example.com` if you use it) |
| Scheme | `http` |
| Forward host / IP | `127.0.0.1` (inside the container this means *this server*) |
| Forward port | `8080` |

Request a Let's Encrypt certificate in the **TLS** section, turn on **Force HTTPS**, and save. Your
site is now served through Corvus. See [Your first proxy host](first-host.md) for the details.

### If something doesn't work

- **502 Bad Gateway**: Corvus can't reach port 8080. See the warning above and
  [Troubleshooting](../reference/troubleshooting.md).
- **Redirect loop**: your web server or app still redirects to HTTPS itself. Corvus already does
  that; turn the app's own HTTPS redirect off, or tell the app it is behind a proxy (it receives
  `X-Forwarded-Proto: https`).
- **Wrong links or mixed content**: set the app's site address to `https://example.com` (for
  WordPress: *Settings → General*).

## The services run on other servers { #services-on-other-servers }

Corvus does not have to run next to your services. A common setup is one gateway server that
receives the traffic from the internet, with the actual services on other servers in the private
network:

```text
Internet ──► :80 / :443  gateway server (Corvus)  ──►  10.0.0.21:8080   (web server)
                                                  ──►  10.0.0.22:3000   (app server)
                                                  ──►  nas.corp.example:5001 (NAS, HTTPS)
```

Enter the other server's address as the **Forward host**: a private IP address such as
`10.0.0.21` or `192.168.1.20`, or a DNS name. Private addresses are allowed. A service that uses
HTTPS with a self-signed certificate works too: choose the `https` scheme; Corvus does not check
the backend's certificate.

**DNS names.** Corvus looks names up through DNS at request time (and again every 30 seconds, so a
changed address is picked up). Two things work differently from typing the name on the server:

- **Use the full name** (`app01.corp.example`), not a short one (`app01`). Corvus does not append
  the search domains of `/etc/resolv.conf`, so a short name that only works through a search domain
  answers *502 Bad Gateway*.
- **Names only in the gateway server's `/etc/hosts`** are not visible inside the container. Add
  them to the Corvus service in `docker-compose.yml`; Corvus uses those addresses:

    ```yaml
        extra_hosts:
          - 'app01.corp.example:10.0.0.21'
    ```

**Docker networks that overlap your network.** Docker picks private address ranges for its own
networks (for example `172.18.0.0/16` or `192.168.16.0/20`). If such a range is also used in your
private network, Corvus cannot reach the servers in it: those hosts answer *502* while others work.
Check with `docker network inspect <network> | grep Subnet` and give the Corvus network a range
that is free in your network:

```yaml
networks:
  default:
    ipam:
      config:
        - subnet: 172.30.0.0/24
```

To keep Docker away from your ranges for all new networks, set `default-address-pools` in
`/etc/docker/daemon.json`.

**Check it before you save.** **Check connection** in the proxy host form tries the target from the
gateway with the same name lookup and reports which of the problems above it hit. See
[Check connection](../proxy/proxy-hosts.md#check-connection).

**On the service servers:**

- Requests arrive from the **gateway server's address**. The visitor's address is in
  `X-Forwarded-For` and `X-Real-IP`. Add the gateway server's address to the app's trusted proxies
  (for example `trusted_proxies` in Nextcloud, `set_real_ip_from` in nginx) so it logs and limits
  the real visitor.
- Let only the gateway server connect to the service ports (firewall). Otherwise anyone in the
  network can bypass Corvus and its protections.

## Next

- [Install](install.md)
- [Your first proxy host](first-host.md)
- [Install on a NAS](nas.md)
