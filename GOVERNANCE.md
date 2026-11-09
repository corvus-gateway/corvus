# Governance

Corvus is a small project with a single maintainer today. This document describes how decisions
are made, who does what, and how the project keeps going if the maintainer is unavailable.

## Roles

| Role | Who | Responsibilities |
|---|---|---|
| **Maintainer** | [@moonjikwang](https://github.com/moonjikwang) (Jikwang Moon) | Sets direction and the [roadmap](ROADMAP.md); reviews and merges pull requests; cuts releases; handles security reports ([SECURITY.md](SECURITY.md)); enforces the [Code of Conduct](CODE_OF_CONDUCT.md); owns the GitHub organization, the GHCR packages and the website domain |
| **Contributor** | Anyone who opens an issue, a discussion or a pull request | Follows [CONTRIBUTING.md](CONTRIBUTING.md) and the Code of Conduct |

A contributor who has made several good contributions over time may be invited to become a
maintainer with write access. New maintainers are added by the existing maintainers and listed
here.

## Decisions

- Everyday changes are decided in issues and pull requests. Every change, including the
  maintainer's own, reaches `main` through a pull request with passing CI.
- Larger changes (new features, changes to the configuration format, the API or the security
  model) start as an issue or a discussion so that users can comment before code is written.
- The maintainer makes the final decision. The project prefers few, well-supported features over
  many: see the scope in the [roadmap](ROADMAP.md).
- Design decisions that affect the architecture are recorded in pull request descriptions and in
  [ARCHITECTURE.md](docs/ARCHITECTURE.md).

## Releases

Releases follow semantic versioning. Each version is first published as a release candidate
(`-rc.N`, a pre-release that does not move `:latest`), tested, and then published as the final
version from the same commit. Release notes list user-visible changes and upgrade steps.

## Continuity

If the maintainer is unavailable, the project must still be able to publish a security fix:

- The source, the issue tracker, the container images (GHCR) and the release workflow all live in
  the `corvus-gateway` GitHub organization. Releases are built and signed by GitHub Actions with
  keyless Sigstore signing, so publishing a release needs no personal key or machine.
- The only long-lived secret is the key that signs managed rule bundles. Gateways keep working
  without new bundles; a replacement key can be shipped in a new release.
- The project is licensed under Apache-2.0, so anyone can fork and continue it.

The project is looking for a second maintainer; see [CONTRIBUTING.md](CONTRIBUTING.md).

## Changes to this document

Changes to governance are made through a pull request and announced in
[Discussions](https://github.com/corvus-gateway/corvus/discussions).
