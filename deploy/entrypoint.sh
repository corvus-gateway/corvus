#!/bin/sh
# Runs corvus-control as an unprivileged user: uid/gid 10001 by default, or PUID/PGID.
#
# Started as root (the default — e.g. `docker compose up` with a ./data bind mount that
# Docker created as root), it first makes the data directory writable for that user and
# then drops privileges, keeping only CAP_NET_BIND_SERVICE so the gateway can bind ports
# 80/443 also with `network_mode: host`, where the host's
# net.ipv4.ip_unprivileged_port_start (usually 1024) applies. In Docker's bridge networks
# the container namespace already allows it (ip_unprivileged_port_start=0).
#
# Started with `user: "…"` (hardened compose), it runs directly and PUID/PGID are ignored.
set -e
if [ "$(id -u)" = "0" ]; then
  uid="${PUID:-10001}"
  gid="${PGID:-10001}"
  case "$uid$gid" in
    *[!0-9]*) echo "corvus: PUID and PGID must be numbers" >&2; exit 1 ;;
  esac
  if [ "$uid" = "0" ] || [ "$gid" = "0" ]; then
    echo "corvus: PUID/PGID 0 (root) is not allowed" >&2; exit 1
  fi
  data="${CORVUS_DATA_DIR:-/data}"
  mkdir -p "$data"
  if [ "$(stat -c %u:%g "$data")" != "$uid:$gid" ]; then
    echo "corvus: setting owner of $data to $uid:$gid"
    chown -R "$uid:$gid" "$data"
  fi
  # CAP_NET_BIND_SERVICE is bit 10; it is missing when the container runs with cap_drop: ALL.
  bnd=$(awk '/^CapBnd:/ {print $2}' /proc/self/status)
  if [ $((0x$bnd & 0x400)) -ne 0 ]; then
    exec setpriv --reuid="$uid" --regid="$gid" --clear-groups \
      --bounding-set=-all,+net_bind_service --inh-caps=-all,+net_bind_service \
      --ambient-caps=-all,+net_bind_service \
      /usr/local/bin/corvus-control "$@"
  fi
  exec su-exec "$uid:$gid" /usr/local/bin/corvus-control "$@"
fi
exec /usr/local/bin/corvus-control "$@"
