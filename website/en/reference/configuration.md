# Configuration

Corvus is configured in the console. A few settings that must be known before it starts are
environment variables: set them under `environment:` in `docker-compose.yml` and run
`docker compose up -d`.

```yaml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    environment:
      CORVUS_ADMIN_HOSTS: corvus.example.com,203.0.113.10
      PUID: 1000
      PGID: 1000
```

| Variable | Description |
|---|---|
| `CORVUS_ADMIN_HOSTS` | Extra names/IPs for the admin console certificate, e.g. `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | Use PostgreSQL instead of SQLite, e.g. `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | Use an external Redis/Valkey for the access queue (several gateways sharing one queue); with `CORVUS_REDIS_PASSWORD`. Default: built-in |
| `CORVUS_REDIS_PORT` | Port of the built-in access-queue store on 127.0.0.1 (default `6379`). Change it with `network_mode: host` if the host already uses 6379 |
| `CORVUS_IMPORT_DIR` | Directory with read-only NPM volumes for [Import from NPM](../getting-started/migrate-npm.md) |
| `CORVUS_RULES_URL` | Optional URL of a signed managed-rules bundle |
| `CORVUS_ADMIN_ALLOW_ANY=1` | Recovery switch if you locked yourself out with the admin allowlist |
| `CORVUS_RESOLVER` | DNS server for forward hosts. Default: Docker's DNS, or the first `nameserver` of `/etc/resolv.conf` with host networking |
| `CORVUS_LOOPBACK=container` | Make `127.0.0.1`/`localhost` forward hosts point at the container itself instead of the Docker host |
| `CORVUS_IPV6` | `auto` (default): also listen on IPv6 when the system has it · `off`: IPv4 only · `on`: always listen on IPv6 and resolve forward hosts to IPv6 addresses too |
| `PUID` / `PGID` | User and group the container runs as and that owns `./data` (default `10001`), e.g. your NAS user |
| `CORVUS_ACME_DIRECTORY` | ACME directory URL of another CA instead of Let's Encrypt, e.g. a private step-ca `https://ca.internal/acme/acme/directory` |
| `LEGO_CA_CERTIFICATES` | PEM file(s) of a private CA the ACME client should trust (mount it into the container), e.g. `/data/ca.pem` |

## Data directory

| Path in `./data` | Contents |
|---|---|
| `corvus.db` | Settings database (SQLite) |
| `secrets/` | Encryption keys. **Back this up** |
| `certs/` | Certificates |
| `nginx/` | Generated configuration versions |
| `history/` | Traffic history and request logs |
| `geoip/` | Country database (downloaded) |

The container runs as the unprivileged user `10001` (or `PUID`) and takes ownership of `./data` on
first start.
