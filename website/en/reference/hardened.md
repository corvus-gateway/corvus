---
description: "Run Corvus with a read-only root filesystem, no Linux capabilities and no privilege escalation using the hardened Compose file."
---

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

!!! note "Large uploads and downloads"
    In this mode temporary files live in a 256 MB in-memory `/tmp`. Downloads never use it, and
    uploads stream straight to your service, except bodies that request body inspection reads: those
    with a `Content-Length` within the inspection size limit, and bodies sent without one (chunked or
    HTTP/2), up to the host's maximum upload size. Raise the `tmpfs` size if such uploads can be
    larger.

## Verify the image

Release images are built by GitHub Actions from the tagged source and signed keylessly with
[Sigstore](https://www.sigstore.dev/) cosign. There is no long-lived key to obtain: the signature
proves the image came from this repository's release workflow. Use cosign 3 (or 2.6 with
`--new-bundle-format`).

From 1.10.0 on, the build runs in a separate reusable workflow, `build-image.yml`, that the
release workflow can only start, not change. Its build provenance meets
[SLSA](https://slsa.dev/) v1.0 **Build Level 3**; releases up to 1.9.x were signed by `release.yml`
directly (Build Level 2). The commands below accept both.

```bash
cosign verify ghcr.io/corvus-gateway/gateway:1.10.1 \
  --certificate-identity-regexp '^https://github.com/corvus-gateway/corvus/.github/workflows/(build-image|release).yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# build provenance (SLSA Build Level 3), with the GitHub CLI
gh attestation verify oci://ghcr.io/corvus-gateway/gateway:1.10.1 --owner corvus-gateway \
  --signer-workflow corvus-gateway/corvus/.github/workflows/build-image.yml

# 1.9.x and earlier were built by release.yml itself: leave out --signer-workflow
gh attestation verify oci://ghcr.io/corvus-gateway/gateway:1.9.2 --owner corvus-gateway
```

Each [GitHub release](https://github.com/corvus-gateway/corvus/releases) also has these files:

| File | Content |
|---|---|
| `corvus-<version>-sbom-linux-amd64.spdx.json`, `…-arm64.spdx.json` | Software bill of materials (SPDX 2.3) |
| `corvus-<version>.intoto.jsonl` | Build provenance (SLSA v1, in-toto envelope) |
| `corvus-<version>.provenance.sigstore.json` | The same provenance as a Sigstore bundle |
| `corvus-<version>.image-digest.txt` | The image digest the files describe |
| `SHA256SUMS`, `SHA256SUMS.sigstore.json` | Checksums of the files above and their signature |

Check the downloaded files:

```bash
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github.com/corvus-gateway/corvus/.github/workflows/(build-image|release).yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c SHA256SUMS
```

Files are attached from 1.2.4 on.
