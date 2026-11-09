# Roadmap

Corvus aims to be a reverse proxy that is as easy to run as Nginx Proxy Manager and protects the
services behind it without extra tools. The project prefers fewer, well-supported features and a
simple console over a long feature list.

This roadmap covers roughly the next twelve months (from November 2026). It is a direction, not a
promise; priorities change with user feedback. Ideas and votes are welcome in
[Discussions](https://github.com/corvus-gateway/corvus/discussions) and
[issues](https://github.com/corvus-gateway/corvus/issues).

## Now

- **Reliability first.** Bug-fix releases for problems reported by users, each tested as a release
  candidate before the final version.
- **Tests.** Keep statement coverage of the control plane at 80% or more (enforced in CI, about
  90% today) and extend the integration suite (`deploy/dev/itest.sh`) to more gateway scenarios.
- **Release integrity.** SBOM, build provenance and signed checksums attached to every GitHub
  release, next to the signed container images.

## Next

- **Installation guides** for more platforms (Synology, Unraid, a plain `docker run`).
- **Translations** of the console reviewed by native speakers (de, es, fr, ja, ru, zh).
- **Migration from Nginx Proxy Manager:** cover more of the settings people bring over, based on
  reports from real migrations.
- **Smaller console fixes** collected in issues labelled
  [`good first issue`](https://github.com/corvus-gateway/corvus/labels/good%20first%20issue).

## Later

- Better tuning help for security rules and the access queue, driven by false-positive reports.
- A second maintainer, so that reviews and releases do not depend on one person.

## Not planned

To keep Corvus small and easy to operate, these are out of scope for now: load balancing across
several backends, forward authentication for other apps, client-certificate (mTLS)
authentication, and running as a cluster.
