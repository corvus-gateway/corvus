---
description: "강화용 Compose 파일로 읽기 전용 루트 파일 시스템, Linux capability 없음, 권한 상승 금지 상태로 Corvus를 실행합니다."
---

# 강화 설치

기본 이미지도 이미 root가 아닌 사용자로 실행되며, Redis는 생성된 비밀번호로 보호되고 OpenResty는
서명을 검증한 소스로 빌드됩니다.
[`deploy/docker-compose.hardened.yml`](https://github.com/corvus-gateway/corvus/blob/main/deploy/docker-compose.hardened.yml)은
여기서 더 나아가 **읽기 전용 루트 파일 시스템**, **Linux capability 없음**, **권한 상승 금지**를
적용합니다.

먼저 사용자 `10001`용 데이터 디렉터리를 만듭니다.

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

!!! note "큰 파일 업로드와 다운로드"
    이 모드에서는 임시 파일이 256 MB 크기의 메모리 기반 `/tmp`에 저장됩니다. 다운로드는 이곳을 쓰지
    않고, 업로드도 서비스로 바로 흘려보냅니다. 단, 요청 본문 검사가 읽는 본문은 이곳에 임시로
    저장됩니다. `Content-Length`가 검사 크기 제한 이내인 본문과, 길이 없이 보내는 본문(chunked 또는
    HTTP/2)은 호스트의 최대 업로드 크기까지입니다. 이런 업로드가 더 클 수 있다면 `tmpfs` 크기를
    늘리세요.

## 이미지 검증

릴리즈 이미지는 태그된 소스로 GitHub Actions에서 빌드되고, [Sigstore](https://www.sigstore.dev/)
cosign으로 키 없이(keyless) 서명됩니다. 따로 받아 둘 공개 키는 없습니다. 서명이 이 저장소의
`release.yml` 워크플로에서 만든 이미지임을 증명합니다. cosign 3(또는 `--new-bundle-format`을 쓴
2.6)을 사용하세요.

```bash
cosign verify ghcr.io/corvus-gateway/gateway:1.2.9 \
  --certificate-identity-regexp '^https://github.com/corvus-gateway/corvus/.github/workflows/release.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com

# 빌드 출처(SLSA) 확인, GitHub CLI 사용
gh attestation verify oci://ghcr.io/corvus-gateway/gateway:1.2.9 --owner corvus-gateway
```

각 [GitHub 릴리즈](https://github.com/corvus-gateway/corvus/releases)에는 다음 파일도 첨부됩니다.

| 파일 | 내용 |
|---|---|
| `corvus-<버전>-sbom-linux-amd64.spdx.json`, `…-arm64.spdx.json` | 소프트웨어 구성 목록(SBOM, SPDX 2.3) |
| `corvus-<버전>.intoto.jsonl` | 빌드 출처(SLSA v1, in-toto 형식) |
| `corvus-<버전>.provenance.sigstore.json` | 같은 빌드 출처의 Sigstore 번들 |
| `corvus-<버전>.image-digest.txt` | 이 파일들이 가리키는 이미지 다이제스트 |
| `SHA256SUMS`, `SHA256SUMS.sigstore.json` | 위 파일들의 체크섬과 그 서명 |

내려받은 파일 확인:

```bash
cosign verify-blob SHA256SUMS --bundle SHA256SUMS.sigstore.json \
  --certificate-identity-regexp '^https://github.com/corvus-gateway/corvus/.github/workflows/release.yml@refs/tags/v' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com
sha256sum -c SHA256SUMS
```

파일 첨부는 1.2.4부터입니다.
