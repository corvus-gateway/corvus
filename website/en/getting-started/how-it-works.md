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

## Next

- [Install](install.md)
- [Your first proxy host](first-host.md)
- [Install on a NAS](nas.md)
