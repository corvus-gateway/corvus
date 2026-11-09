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

!!! note "Large uploads"
    In this mode temporary files live in a 256 MB in-memory `/tmp`. If a proxy host accepts uploads
    larger than about 200 MB, raise the `tmpfs` size in the compose file.

## Verify the image

Release images are built by GitHub Actions from the tagged source and signed keylessly with
[Sigstore](https://www.sigstore.dev/) cosign. There is no long-lived key to obtain: the signature
proves the image came from the `release.yml` workflow of this repository. Use cosign 3 (or 2.6 with
`--new-bundle-format`).

```bash
cosign verify ghcr.io/corvus-gateway/gateway:1.2.4 \
  --certificate-identity-regexp '^https://github.com/corvus-gateway/corvus/.github/workflows/release.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# build provenance (SLSA), with the GitHub CLI
gh attestation verify oci://ghcr.io/corvus-gateway/gateway:1.2.4 --owner corvus-gateway
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
  --certificate-identity-regexp '^https://github.com/corvus-gateway/corvus/.github/workflows/release.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c SHA256SUMS
```

Files are attached from 1.2.4 on.
