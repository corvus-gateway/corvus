---
description: "Corvus vs Traefik: Traefik builds routes from Docker labels and Kubernetes resources; Corvus is managed in a web console with a free built-in WAF, bans, bot challenge and waiting room. When to pick which."
title: Corvus vs Traefik
keywords: [Traefik alternative, Traefik vs Corvus, Traefik WAF, Traefik web UI, Traefik CrowdSec, self-hosted reverse proxy]
mentions:
  - { name: Traefik, url: "https://github.com/traefik/traefik" }
---

# Corvus vs Traefik

[Traefik Proxy](https://traefik.io/traefik/) (MIT) is a cloud-native reverse proxy in Go. Its
strength is **service discovery**: it reads Docker labels, Kubernetes Ingress and its own CRDs and
creates routes automatically. Its dashboard shows the routes but does not edit them. Corvus is the
opposite approach: you add hosts in a console, and the security layer is built in.

## Choose Traefik if

- Your services are containers and you want routes to **appear from their labels** when they start.
- You run **Kubernetes** or Docker Swarm, or keep everything in Git.
- You need middlewares such as ForwardAuth (for Authelia or Authentik), circuit breakers or
  in-flight request limits.
- You want HTTP/3.

## Choose Corvus if

- You prefer to **manage hosts in a web console** instead of labels and YAML, or you proxy to
  services that are not containers.
- You want a **WAF without a paid edition**: in Traefik the native WAF middleware is part of Traefik
  Hub, otherwise it comes from plugins (Coraza WASM, the CrowdSec bouncer). In Corvus
  [exploit blocking](../security/index.md), [automatic bans](../security/bans.md),
  [country and header rules](../security/rules.md) and a [browser challenge](../security/challenge.md)
  are built in and free.
- You want a [live traffic monitor, request logs](../operations/traffic.md) and
  [alerts](../operations/alerts.md) in the same UI.
- You need a [waiting room](../access-queue.md) for traffic spikes.

## Key differences

| | Traefik Proxy | Corvus |
|---|---|---|
| Configuration | Docker labels, Kubernetes, files | Web console, HTTP API |
| Dashboard | Read-only view of routes | Full management |
| Let's Encrypt, DNS challenge | ✅ many providers | ✅ 7 providers |
| Rate limits, IP allow lists | ✅ middlewares | ✅ |
| WAF | Traefik Hub (paid) or plugins | ✅ built in |
| Automatic bans, country rules, bot challenge | Plugins | ✅ built in |
| Traffic monitor, request logs in the UI | ❌ (logs and metrics for other tools) | ✅ |
| Kubernetes ingress | ✅ | ❌ |
| License | MIT (Hub is commercial) | Apache-2.0, no paid edition |

They can also work together: Traefik for discovery inside a cluster, with Corvus in front as the
protected entry point. [All comparisons](index.md)
