---
description: Install Corvus on a NAS — TrueNAS SCALE app catalog, ports, the setup token, and tips for other NAS systems.
---

# Install on a NAS

## TrueNAS SCALE

Corvus is in the **Community** train of the TrueNAS app catalog.

1. **Apps → Discover Apps**, search for **Corvus**, then **Install**.
2. Keep the default ports or change them:

    | Port | Default | Used for |
    |---|---|---|
    | Admin Console Port | **30553** | The console (HTTPS, self-signed certificate at first) |
    | HTTP Port | **30554** | Your sites over HTTP and Let's Encrypt HTTP validation |
    | HTTPS Port | **30555** | Your sites over HTTPS |

    The TrueNAS web interface already uses 80 and 443, which is why the app uses other ports.

3. Choose where Corvus keeps its data (**Data Storage**). Back up that dataset like any other app
   data; it holds the database, certificates and the encryption keys.
4. After the app starts, open its **logs** in the Apps page and find the line with `setup_token`.
   To choose the token yourself instead, add `CORVUS_SETUP_TOKEN` (at least 12 characters) under
   **Additional Environment Variables** before installing.
5. Open `https://<truenas-ip>:30553`, accept the certificate warning, enter the token and create the
   administrator account. Then continue with [your first proxy host](first-host.md).

### Reaching your sites from the internet

Forward ports **80 → 30554** and **443 → 30555** on your router to the TrueNAS address. Visitors and
Let's Encrypt then reach Corvus on the usual ports. Without port 80 forwarded, use the
[DNS challenge](../proxy/certificates.md) for certificates.

Keep the console port (30553) closed to the internet; see [Protect the console](../operations/users.md#protect-the-console).

### Updates

TrueNAS shows an update for the app when a new Corvus version is in the catalog. Data is migrated
automatically on start.

## Other NAS systems

Any NAS that runs Docker Compose (Synology Container Manager, Unraid, QNAP Container Station, …)
can use the normal [install](install.md). Things to watch for:

- **Ports 80/443 already in use by the NAS.** Publish Corvus on other ports (e.g. `8080:80` and
  `8443:443`) and forward 80/443 from your router to them, or move the NAS interface to other ports.
- **Folder permissions.** Set `PUID`/`PGID` to the NAS user that owns the data folder
  (see [Configuration](../reference/configuration.md)).
- **The setup token** is in the container log, or set `CORVUS_SETUP_TOKEN`.

Running Corvus on a NAS that isn't covered here? Notes are welcome in
[issue #94](https://github.com/corvus-gateway/corvus/issues/94).
