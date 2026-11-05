# Troubleshooting

## I can't open the console

- Use **https**: `https://<server>:9443`. The first certificate is self-signed, so the browser warns.
- Is the container running? `docker compose ps`, then `docker compose logs corvus`.
- A firewall or **Admin console allowed networks** may block you. To recover, start once with
  `CORVUS_ADMIN_ALLOW_ANY=1` and fix the list.
- Lost the setup token? `docker compose logs corvus | grep setup_token` (only shown until the first
  administrator exists).

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

## Let's Encrypt fails

- **HTTP-01**: the domain must point at this server and port 80 must be reachable from the internet
  (not only 443).
- **Wildcard** domains need DNS-01.
- Too many attempts? Let's Encrypt has rate limits; use **staging** while testing.
- The certificate's row shows the last error.

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
