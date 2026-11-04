#!/bin/sh
# Runs corvus-control as the unprivileged user 10001.
#
# Started as root (the default — e.g. `docker compose up` with a ./data bind mount that
# Docker created as root), it first makes the data directory writable for that user and
# then drops privileges, keeping only CAP_NET_BIND_SERVICE so the gateway can bind ports
# 80/443 also with `network_mode: host`, where the host's
# net.ipv4.ip_unprivileged_port_start (usually 1024) applies. In Docker's bridge networks
# the container namespace already allows it (ip_unprivileged_port_start=0).
#
# Started with `user: "10001:10001"` (hardened compose), it runs directly.
set -e
if [ "$(id -u)" = "0" ]; then
  data="${CORVUS_DATA_DIR:-/data}"
  mkdir -p "$data"
  if [ "$(stat -c %u "$data")" != "10001" ]; then
    echo "corvus: setting owner of $data to 10001:10001"
    chown -R 10001:10001 "$data"
  fi
  # CAP_NET_BIND_SERVICE is bit 10; it is missing when the container runs with cap_drop: ALL.
  bnd=$(awk '/^CapBnd:/ {print $2}' /proc/self/status)
  if [ $((0x$bnd & 0x400)) -ne 0 ]; then
    exec setpriv --reuid=10001 --regid=10001 --clear-groups \
      --bounding-set=-all,+net_bind_service --inh-caps=-all,+net_bind_service \
      --ambient-caps=-all,+net_bind_service \
      /usr/local/bin/corvus-control "$@"
  fi
  exec su-exec 10001:10001 /usr/local/bin/corvus-control "$@"
fi
exec /usr/local/bin/corvus-control "$@"
