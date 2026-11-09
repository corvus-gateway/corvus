---
description: "docker compose pull로 Corvus를 업그레이드합니다. 설정, 인증서, 기록은 유지되고 데이터베이스 변경은 자동으로 적용됩니다."
---

# 업그레이드

```bash
docker compose pull
docker compose up -d
```

`./data`에 있는 설정, 인증서, 기록은 그대로 유지됩니다. 데이터베이스 변경은 새 버전이 시작될 때
자동으로 적용됩니다.

!!! tip "메이저 업그레이드 전에"
    먼저 백업을 내려받으세요: *데이터 보관 → 백업 및 복원 → 백업 다운로드*
    ([백업과 복원](../operations/backup.md) 참고).

## 버전 고정

`latest`는 항상 최신 릴리스를 가리킵니다. 업그레이드 시점을 직접 정하려면 버전 태그를
사용하세요.

```yaml
    image: 'ghcr.io/corvus-gateway/gateway:1.0.3'
```

릴리스 노트: [GitHub releases](https://github.com/corvus-gateway/corvus/releases).

## 1.0.3 참고 사항

- 기존 호스트에도 **Gzip**이 켜집니다. 필요하면 호스트별로 *고급*에서 끌 수 있습니다.
- 콘솔의 요청 로그는 최대 0.5초 늦게 표시될 수 있습니다(묶어서 기록하기 때문입니다).
- 백엔드 이름은 요청마다가 아니라 30초마다 다시 조회합니다.
- `$proxy_host`를 쓰는 고급 설정은 이제 `corvus_up_<id>` 값을 보게 됩니다.
- 브리지 네트워크의 스트림은 `docker-compose.yml`에서 포트를 공개해야 합니다
  ([스트림](../proxy/streams.md) 참고).
