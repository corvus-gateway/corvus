---
description: "Install Corvus on a NAS: TrueNAS SCALE app catalog, Unraid Community Applications, Synology Container Manager, ports, the setup token, and tips for other NAS systems."
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

## Unraid

Corvus is in **Community Applications** (the **Apps** tab).

1. **Apps**, search for **Corvus**, then **Install**.
2. Keep the default ports or change them:

    | Port | Default | Used for |
    |---|---|---|
    | Admin console | **9443** | The console (HTTPS, self-signed certificate at first) |
    | HTTP | **1880** | Your sites over HTTP and Let's Encrypt HTTP validation |
    | HTTPS | **18443** | Your sites over HTTPS |

    Unraid's web interface usually uses 80 and 443, which is why the template uses other ports.
    To use 80/443 directly, give the container its own IP address (**Network Type** `br0` or a
    custom network).

3. Data is kept in `/mnt/user/appdata/corvus` and owned by `nobody:users` (PUID 99, PGID 100).
   Back it up like other app data; it holds the database, certificates and the encryption keys.
4. To choose the setup token yourself, fill in **Setup Token** (at least 12 characters) before the
   first start. Otherwise open the container's **Logs** and find the line with `setup_token`.
5. Click **WebUI** in the container menu (or open `https://<unraid-ip>:9443`), accept the
   certificate warning, enter the token and create the administrator account.

Forward ports **80 → 1880** and **443 → 18443** on your router to reach your sites from the
internet. Keep the console port closed to the internet. Updates appear in the **Docker** tab like
for other containers.

## Synology (Container Manager)

On DSM 7.2 and later, install Corvus as a **Project** in Container Manager.

1. In **File Station**, create a folder for it, e.g. `docker/corvus` on `volume1`.
2. **Container Manager → Project → Create**. Name it `corvus`, choose the folder from step 1 as the
   path and **Create docker-compose.yml**:

    ```yaml
    services:
      corvus:
        image: 'ghcr.io/corvus-gateway/gateway:latest'
        restart: unless-stopped
        environment:
          PUID: 1026   # your DSM user, see below
          PGID: 100
        ports:
          - '8080:80'
          - '8443:443'
          - '9443:9443'
        volumes:
          - ./data:/data
    ```

    DSM itself uses ports 80 and 443 (for its web services and its own reverse proxy), so Corvus is
    published on 8080 and 8443. The console port 9443 is free on a default DSM.

3. **PUID/PGID** set who owns the data folder. Use the DSM user that owns the folder from step 1:
   the first user created on DSM usually has the ID 1026 and the group `users` is 100. If you're not
   sure, run `id <username>` over SSH.
4. Finish the wizard; Container Manager pulls the image and starts the project. Open **Container →
   corvus → Log** and find the line with `setup_token` (or add `CORVUS_SETUP_TOKEN` under
   `environment` before the first start).
5. Open `https://<nas-ip>:9443`, accept the certificate warning, enter the token and create the
   administrator account.

**From the internet:** forward ports **80 → 8080** and **443 → 8443** on your router to the NAS. If
the DSM firewall is on, allow 8080, 8443 and (only from your LAN) 9443. Keep the console port closed
to the internet.

**Updates:** **Project → corvus → Action → Build** pulls the latest image and recreates the
container; data in `./data` is kept.

## Other NAS systems

Any other NAS that runs Docker Compose (QNAP Container Station, …)
can use the normal [install](install.md). Things to watch for:

- **Ports 80/443 already in use by the NAS.** Publish Corvus on other ports (e.g. `8080:80` and
  `8443:443`) and forward 80/443 from your router to them, or move the NAS interface to other ports.
- **Folder permissions.** Set `PUID`/`PGID` to the NAS user that owns the data folder
  (see [Configuration](../reference/configuration.md)).
- **The setup token** is in the container log, or set `CORVUS_SETUP_TOKEN`.

Running Corvus on a NAS that isn't covered here? Notes are welcome in
[issue #94](https://github.com/corvus-gateway/corvus/issues/94).
