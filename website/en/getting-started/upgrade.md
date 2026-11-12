---
description: "Upgrade Corvus with docker compose pull. Settings, certificates and history are kept, and database changes are applied automatically."
---

# Upgrade

```bash
docker compose pull
docker compose up -d
```

Settings, certificates and history in `./data` are kept. Database changes are applied automatically
when the new version starts.

The live views (dashboard, traffic monitor, request log) also continue where they left off: Corvus
saves the last hour when it stops and loads it back on start. After a crash or a stop of more than
an hour, only the 5-minute statistics and events are kept.

!!! tip "Before a major upgrade"
    Download a backup first: *Data retention → Backup & restore → Download backup*
    (see [Backup & restore](../operations/backup.md)).

## Pin a version

`latest` always points to the newest release. To decide yourself when to upgrade, use a version tag
instead:

```yaml
    image: 'ghcr.io/corvus-gateway/gateway:1.2.9'
```

Release notes: [GitHub releases](https://github.com/corvus-gateway/corvus/releases).

## Notes for 1.0.3

- **Gzip** is now on for existing hosts. Turn it off per host under *Advanced* if needed.
- Request logs in the console can appear up to 0.5 s late (they are written in batches).
- Backend names are re-resolved every 30 s instead of per request.
- Advanced configs that use `$proxy_host` now see `corvus_up_<id>`.
- Streams on a bridge network need their ports published in `docker-compose.yml`
  (see [Streams](../proxy/streams.md)).
