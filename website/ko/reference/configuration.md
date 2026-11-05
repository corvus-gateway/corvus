# 환경 변수

Corvus는 콘솔에서 설정합니다. 시작하기 전에 알아야 하는 몇 가지 설정만 환경 변수로 지정합니다.
`docker-compose.yml`의 `environment:` 아래에 넣고 `docker compose up -d`를 실행하세요.

```yaml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    environment:
      CORVUS_ADMIN_HOSTS: corvus.example.com,203.0.113.10
      PUID: 1000
      PGID: 1000
```

| 변수 | 설명 |
|---|---|
| `CORVUS_ADMIN_HOSTS` | 관리 콘솔 인증서에 추가할 이름/IP. 예: `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | SQLite 대신 PostgreSQL 사용. 예: `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | 접속 대기열에 외부 Redis/Valkey 사용(여러 게이트웨이가 대기열 하나를 공유할 때). `CORVUS_REDIS_PASSWORD`와 함께 씁니다. 기본값: 내장 저장소 |
| `CORVUS_REDIS_PORT` | 127.0.0.1에서 동작하는 내장 접속 대기열 저장소의 포트(기본값 `6379`). `network_mode: host`에서 호스트가 이미 6379를 쓰고 있다면 바꾸세요 |
| `CORVUS_IMPORT_DIR` | [NPM 가져오기](../getting-started/migrate-npm.md)에 쓸, 읽기 전용 NPM 볼륨이 있는 디렉터리 |
| `CORVUS_RULES_URL` | 선택 사항. 서명된 관리형 규칙 번들의 URL |
| `CORVUS_ADMIN_ALLOW_ANY=1` | 관리 콘솔 접근 허용 목록 때문에 접속이 막혔을 때 쓰는 복구 스위치 |
| `CORVUS_RESOLVER` | 전달 대상 호스트 이름을 조회할 DNS 서버. 기본값: Docker의 DNS, 호스트 네트워크에서는 `/etc/resolv.conf`의 첫 번째 `nameserver` |
| `CORVUS_LOOPBACK=container` | 전달 대상 `127.0.0.1`/`localhost`가 Docker 호스트 대신 컨테이너 자신을 가리키게 합니다 |
| `CORVUS_IPV6` | `auto`(기본값): 시스템에 IPv6가 있으면 IPv6로도 수신 · `off`: IPv4만 · `on`: 항상 IPv6로 수신하고 전달 대상도 IPv6 주소로 조회 |
| `PUID` / `PGID` | 컨테이너를 실행하고 `./data`를 소유할 사용자와 그룹(기본값 `10001`). 예: NAS 사용자 |
| `CORVUS_ACME_DIRECTORY` | Let's Encrypt 대신 쓸 다른 CA의 ACME 디렉터리 URL. 예: 사설 step-ca `https://ca.internal/acme/acme/directory` |
| `LEGO_CA_CERTIFICATES` | ACME 클라이언트가 신뢰할 사설 CA의 PEM 파일(컨테이너에 마운트). 예: `/data/ca.pem` |

## 데이터 디렉터리

| `./data` 안의 경로 | 내용 |
|---|---|
| `corvus.db` | 설정 데이터베이스(SQLite) |
| `secrets/` | 암호화 키. **반드시 백업하세요** |
| `certs/` | 인증서 |
| `nginx/` | 생성된 설정 버전 |
| `history/` | 트래픽 기록과 요청 로그 |
| `geoip/` | 국가 DB(내려받음) |

컨테이너는 권한 없는 사용자 `10001`(또는 `PUID`)로 실행되며, 처음 시작할 때 `./data`의 소유권을
가져옵니다.
