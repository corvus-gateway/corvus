# Hardened install

The default image already runs as a non-root user, with Redis protected by a generated password and
OpenResty built from signature-verified sources.
[`deploy/docker-compose.hardened.yml`](https://github.com/corvus-gateway/corvus/blob/main/deploy/docker-compose.hardened.yml)
goes further: a **read-only root filesystem**, **no Linux capabilities** and **no privilege
escalation**.

Create the data directory for user `10001` first:

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

!!! note "Large uploads"
    In this mode temporary files live in a 256 MB in-memory `/tmp`. If a proxy host accepts uploads
    larger than about 200 MB, raise the `tmpfs` size in the compose file.
