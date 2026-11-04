<div align="center">

[English](../../README.md) · **한국어** · [简体中文](README_zh.md) · [日本語](README_ja.md) · [Русский](README_ru.md) · [Français](README_fr.md) · [Deutsch](README_de.md) · [Español](README_es.md)

<br>

<picture>
  <source media="(prefers-color-scheme: dark)" srcset="../../assets/brand/corvus-logo-dark.svg">
  <img src="../../assets/brand/corvus-logo.svg" alt="Corvus" width="420">
</picture>

### Active Web Protection Gateway

<a href="https://github.com/corvus-gateway/corvus/releases/latest"><img src="https://img.shields.io/github/v/release/corvus-gateway/corvus?label=version&color=2ea44f&style=for-the-badge"></a>
<a href="../../LICENSE"><img src="https://img.shields.io/badge/license-Apache--2.0-blue.svg?style=for-the-badge"></a>
<a href="https://github.com/corvus-gateway/corvus/pkgs/container/gateway"><img src="https://img.shields.io/badge/ghcr.io-corvus--gateway%2Fgateway-24292f.svg?style=for-the-badge&logo=docker&logoColor=white"></a>
<a href="https://github.com/corvus-gateway/corvus/actions/workflows/ci.yml"><img src="https://img.shields.io/github/actions/workflow/status/corvus-gateway/corvus/ci.yml?branch=main&label=CI&style=for-the-badge"></a>

<img src="https://img.shields.io/badge/OpenResty-1.31-00b140?style=flat-square">
<img src="https://img.shields.io/badge/Go-1.26-00add8?style=flat-square&logo=go&logoColor=white">
<img src="https://img.shields.io/badge/React-18-61dafb?style=flat-square&logo=react&logoColor=black">
<img src="https://img.shields.io/badge/i18n-8_languages-8250df?style=flat-square">
<img src="https://img.shields.io/badge/platform-amd64_|_arm64-555?style=flat-square&logo=docker&logoColor=white">
<a href="https://scorecard.dev/viewer/?uri=github.com/corvus-gateway/corvus"><img src="https://api.scorecard.dev/projects/github.com/corvus-gateway/corvus/badge?style=flat-square"></a>
<a href="https://www.bestpractices.dev/projects/16315"><img src="https://www.bestpractices.dev/projects/16315/badge"></a>
<a href="https://www.bestpractices.dev/projects/16315/baseline-1"><img src="https://www.bestpractices.dev/projects/16315/baseline"></a>

</div>

Corvus는 웹 서비스 앞단에 두는 셀프 호스팅 게이트웨이입니다. Nginx Proxy Manager만큼 운영이
쉬워서 도메인, 포워딩, 무료 SSL을 몇 번의 클릭으로 설정할 수 있으며, 여기에 더해 통과하는
트래픽을 **감시하고, 보호하고, 제어**합니다. 실시간 모니터링, 공격 차단, 속도 제한(Rate Limit),
트래픽 급증에 대비한 접속 대기열까지 제공합니다. 모든 것이 여러분의 서버에서 동작하며, 트래픽이
외부 클라우드로 전송되지 않습니다.

- [빠른 설정](#빠른-설정)
- [전체 설정](#전체-설정)
- [스크린샷](#스크린샷)

## 프로젝트 목표

작은 팀은 대개 TLS용 리버스 프록시 하나, 제대로 튜닝해 본 적 없는 별도의 WAF 하나를 두고, 서비스
오픈이나 공격이 닥쳤을 때 무슨 일이 벌어지는지는 알지 못한 채 지냅니다. Corvus는 이 모든 것을
안전한 기본값과 함께 하나의 상자에 담았습니다. 호스트를 추가하는 순간 규칙을 한 줄도 쓰지 않아도
기본적인 공격 탐지가 동작하고, 고급 옵션은 모두 선택 사항으로 남아 있습니다. 5분 만에 설치할 수
있고, 트래픽이 열 배로 늘어나는 날에도 믿고 맡길 수 있는 게이트웨이를 목표로 합니다.

Corvus는 Nginx Proxy Manager의 포크가 아닙니다. 기존 NPM 설정을 **가져올 수 있는** 새로운
구현체(OpenResty 데이터 플레인, Go 컨트롤 플레인, React 콘솔)입니다.

## 주요 기능

**프록시 및 TLS**
- 와일드카드, WebSocket, HTTP/2, HTTPS 강제 전환, 사용자 정의 Nginx 스니펫을 지원하는 프록시 호스트
- 리디렉션 호스트: 도메인을 다른 도메인으로 보냅니다(301/302/303/307/308, 경로 유지 선택). TLS와 보호 기능도 똑같이 적용됩니다
- 자동 갱신되는 무료 Let's Encrypt 인증서(HTTP-01, 또는 DNS-01: Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap) 또는 직접 업로드한 인증서
- Cloudflare 또는 신뢰할 수 있는 프록시 뒤에서도 실제 클라이언트 IP 확인, 호스트별 비밀번호 보호(선택)
- 호스트별 보안 헤더 프리셋(기본 / 엄격: nosniff, 프레임 및 리퍼러 정책, 보고 전용 CSP, HttpOnly/SameSite/Secure 쿠키 플래그). 애플리케이션이 이미 보내는 헤더는 덮어쓰지 않습니다
- 텍스트 응답 Gzip 압축(기본 켜짐), 호스트별 업로드 크기 제한과 업스트림 타임아웃
- 모든 변경은 `openresty -t`로 검증하고, 원자적으로 적용하며, 롤백할 수 있습니다

**Corvus Sentinel — 보안 엔진**
- 보안 규칙: IP, IP 목록, 국가, 경로, 쿼리, 메서드, 헤더, User-Agent 목록을 매칭하여 차단·허용·기록·챌린지, 전역 또는 호스트별 적용
- URL에서 최신 상태로 유지되는 IP 및 User-Agent 목록(예: Tor 출구 노드, 악성 봇 목록, 프리셋 포함). 백그라운드에서 갱신되며 내용이 바뀔 때만 다시 배포합니다
- 외부 서비스 없이 동작하는 브라우저 챌린지: JavaScript 작업 증명(난이도 조절 가능) 또는 가벼운 쿠키 확인, 서명되고 IP에 묶인 통과 쿠키 발급
- 호스트별 "공격 대응 모드": 모든 방문자에게 챌린지를 요구합니다. 직접 켜거나, 초당 요청 수 또는 차단된 요청 비율이 기준을 넘는 동안 자동으로 켜집니다(켜지고 꺼질 때 알림)
- JSON, 폼, multipart 요청 본문 검사(기록 또는 차단)
- 최신 CVE(React2Shell, wp2shell, PHP-CGI, Vite, n8n …)에 대한 원클릭 **가상 패치**
- 서명되어 있고 더 최신인 경우에만 적용하는 관리형 시그니처 업데이트(변조 및 다운그레이드 방지)
- 반복 위반자에 대한 자동 IP 차단(단계적 가중 및 예외 지정 지원), 이상 행동 시 즉시 차단(선택: IP·사이트별 4xx 응답이 너무 많은 경우)
- 드라이런 모드를 갖춘 속도 제한, 기본으로 제공되는 익스플로잇 경로 차단

**접속 대기열(Access Queue)**
- 수용 한도까지만 방문자를 입장시키고, 트래픽이 몰리면 나머지는 공정하게 대기열에 세웁니다
- 입장 속도, fail-open / fail-closed 정책, 실시간 입장/대기 인원, 긴급 해제
- 8개 언어로 현지화된 대기 페이지

**모니터링 및 알림**
- 실시간 초당 요청 수, 상태 코드, p50/p95/p99 지연 시간, 호스트별·경로별 분석
- 요청 로그 검색과 CSV/JSON 내보내기, 보안 이벤트, 보존 기간과 로그 로테이션을 갖춘 영구 이력
- Slack, Mattermost, Discord, Teams, Telegram 또는 서명된 웹훅으로 알림 전송
- Prometheus `/metrics`

**관리**
- 역할, MFA(TOTP + 복구 코드), OIDC 싱글 사인온, API 토큰
- 관리 콘솔 IP 허용 목록, 감사 로그, 배포 이력
- 기본 SQLite 또는 PostgreSQL, 라이트/다크 테마를 지원하는 8개 언어 관리 콘솔

**기본값부터 견고하게**
- 읽기 전용 루트 파일시스템과 Linux capability 없이 비루트로 실행되는 컨테이너
- 자동 생성된 비밀번호와 최소 ACL로 보호되는 Redis
- 서명 검증을 거친 소스로 빌드한 OpenResty

## 스크린샷

<p align="center">
  <img src="../../assets/screenshots/dashboard.png" alt="Dashboard" width="900">
</p>

| 트래픽 관제 | 보안 규칙 |
|:---:|:---:|
| <img src="../../assets/screenshots/traffic.png" alt="Traffic monitor"> | <img src="../../assets/screenshots/security-rules.png" alt="Security rules"> |
| **프록시 호스트** | **접속 대기열** |
| <img src="../../assets/screenshots/proxy-host.png" alt="Proxy host"> | <img src="../../assets/screenshots/access-queues.png" alt="Access queues"> |

## 빠른 설정

1. Compose 플러그인이 포함된 [Docker를 설치](https://docs.docker.com/engine/install/)합니다.
2. 다음과 같은 `docker-compose.yml` 파일을 만듭니다.

```yml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    restart: unless-stopped
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
    volumes:
      - ./data:/data
```

이것이 설치의 전부입니다. 게이트웨이, 관리 콘솔, 접속 대기열 저장소가 이 컨테이너 하나에서
실행됩니다. 옵션은 [전체 설정](#전체-설정)을 참고하세요.

3. 스택을 시작합니다.

```bash
docker compose up -d
```

4. 일회용 설정 토큰을 확인합니다.

```bash
docker compose logs corvus | grep setup_token
```

5. `https://<your-server>:9443`에서 관리 콘솔에 로그인하고, 토큰을 입력한 뒤 첫 번째 관리자를
   만듭니다. 직접 인증서를 설정하기 전까지 콘솔은 자체 서명 인증서를 사용합니다.

> [!TIP]
> 도메인의 DNS를 서버로 향하게 한 뒤 프록시 호스트를 추가하고, 같은 폼에서 Let's Encrypt
> 인증서를 발급받으세요.

### 업그레이드

```bash
docker compose pull
docker compose up -d
```

`./data`에 있는 설정, 인증서, 이력은 그대로 유지되며, 데이터베이스 변경 사항은 시작할 때
자동으로 적용됩니다.

## 전체 설정

### 포트

| 포트 | 용도 |
|---|---|
| 80 / 443 | 게이트웨이 — 프록시 트래픽 |
| 9443 | 관리 콘솔 및 API — 신뢰할 수 있는 네트워크만 허용하세요(방화벽, 또는 *네트워크 → 관리 콘솔 접근 허용 네트워크*) |

### 데이터와 백업

모든 상태는 `./data`에 저장됩니다. 설정 데이터베이스, 생성된 설정 이력, 인증서, 트래픽 이력,
`secrets/`가 여기에 들어 있습니다. **`secrets/`는 반드시 백업하세요.** 데이터베이스에 저장된
MFA 시크릿과 DNS 토큰을 암호화하는 키가 이곳에 있습니다. 컨테이너는 권한 없는 사용자
`10001`로 실행되며, 처음 시작할 때 `./data`의 소유권을 가져옵니다.

**콘솔에서 백업:** *데이터 보관 → 백업 및 복원 → 백업 다운로드*(관리자, 로그인한 세션 전용 — API
토큰으로는 백업을 내려받을 수 없습니다)를 누르면 다음 내용이 담긴
`corvus-backup-<version>-<timestamp>.tar.gz`가 만들어집니다.

- `corvus.db` — SQLite 설정 데이터베이스의 일관된 스냅샷(Corvus가 실행되는 동안 `VACUUM INTO`로
  생성),
- `secrets/`, `certs/`, `rules/`(설치된 관리형 규칙 번들),
- `manifest.json` — 형식 버전, Corvus 버전, 생성 시각, 데이터베이스 엔진, 모든 파일의 SHA-256.

생성된 설정 버전(`nginx/`), 트래픽 이력과 요청 로그(`history/`), 국가 데이터베이스(`geoip/`),
`run/`은 포함되지 않습니다. 다시 생성되거나 다시 내려받기 때문입니다. PostgreSQL(`CORVUS_DB_URL`)을
쓰는 경우 아카이브에는 데이터베이스 없이 `README.txt`만 들어 있습니다. 데이터베이스는 `pg_dump`로
백업하고 덤프를 아카이브와 함께 보관하세요(암호화된 필드를 풀려면 `secrets/data_key`가 필요합니다).
**아카이브에는 개인 키와 데이터 키가 들어 있으므로 암호화해서 보관하세요.** 모든 다운로드는 감사
로그에 기록됩니다.

**콘솔에서 복원:** *백업에서 복원*으로 아카이브(최대 200 MB)를 업로드합니다. 형식 버전, 모든 파일의
SHA-256, 절대 경로·`..`·링크·알 수 없는 항목이 없는지, 데이터베이스 엔진이 같은지를 검사한 뒤 요약
(버전, 날짜, 파일 수와 인증서 수)을 보여 줍니다. *다음 재시작 때 복원*을 누르면 아카이브가
`./data/restore-pending/`에 준비되며, 컨테이너를 재시작하기 전까지는 아무것도 바뀌지 않습니다.

```bash
docker compose restart
```

시작할 때 데이터베이스를 열기 전에 Corvus는 현재의 `corvus.db`, `secrets/`, `certs/`, `rules/`,
`nginx/`를 `./data/restore-backup-<timestamp>/`로 옮기고 그 자리에 백업을 넣습니다(그다음 복원된
데이터베이스로 설정을 다시 생성합니다). 되돌리려면 컨테이너를 중지하고 이 파일들을 원래 위치로
옮기세요. 준비된 파일이 검사를 통과하지 못하면 복원은 적용되지 않고 현재 상태가 유지되며, 준비된
파일은 `./data/restore-failed-<timestamp>/`로 옮겨집니다(`docker compose logs` 참고). 트래픽 이력은
복원의 영향을 받지 않습니다.

**수동 복원:** 아카이브 구조는 데이터 디렉터리 구조와 같습니다. 처음 시작하기 전에 빈 `./data`에
압축을 풀면 됩니다(추가로 들어 있는 `manifest.json`은 무해합니다).

```bash
mkdir data && tar -xzf corvus-backup-….tar.gz -C data
docker compose up -d
```

### 강화된 설치

[`deploy/docker-compose.hardened.yml`](../../deploy/docker-compose.hardened.yml)은 같은 이미지를 읽기 전용 루트
파일 시스템, Linux capability 없음, 권한 상승 불가 상태로 실행합니다. 먼저 사용자 `10001`용
데이터 디렉터리를 만드세요.

```bash
mkdir -p data && sudo chown 10001:10001 data
docker compose -f docker-compose.hardened.yml up -d
```

### 구성

다음 변수는 `docker-compose.yml`의 `environment:` 아래에 설정합니다.

| 변수 | 설명 |
|---|---|
| `CORVUS_ADMIN_HOSTS` | 관리 콘솔 인증서에 추가할 이름/IP. 예: `corvus.example.com,203.0.113.10` |
| `CORVUS_DB_URL` | SQLite 대신 PostgreSQL 사용. 예: `postgres://corvus:…@db:5432/corvus` |
| `CORVUS_REDIS_HOST` | 접속 대기열에 외부 Redis/Valkey 사용(여러 게이트웨이가 하나의 대기열을 공유). `CORVUS_REDIS_PASSWORD`와 함께 사용. 기본값: 내장 |
| `CORVUS_IMPORT_DIR` | *NPM 가져오기*에 사용할 읽기 전용 NPM 볼륨이 있는 디렉터리 |
| `CORVUS_RULES_URL` | 서명된 관리형 규칙 번들의 URL(선택) |
| `CORVUS_ADMIN_ALLOW_ANY=1` | 관리자 허용 목록 설정 때문에 접속할 수 없게 되었을 때 쓰는 복구 스위치 |
| `CORVUS_REDIS_PORT` | 127.0.0.1에서 동작하는 내장 접속 대기열 저장소의 포트(기본값 `6379`). `network_mode: host`에서 호스트가 이미 6379를 쓰고 있다면 변경 |
| `CORVUS_RESOLVER` | 포워드 호스트 이름 확인에 쓸 DNS 서버. 기본값: Docker DNS, 호스트 네트워킹에서는 `/etc/resolv.conf`의 첫 번째 `nameserver` |
| `CORVUS_LOOPBACK=container` | `127.0.0.1`/`localhost` 포워드 호스트가 Docker 호스트 대신 컨테이너 자신을 가리키도록 설정 |
| `CORVUS_IPV6` | `auto`(기본): 시스템에 IPv6가 있으면 IPv6로도 수신 · `off`: IPv4만 · `on`: 항상 IPv6로 수신하고 포워드 호스트를 IPv6 주소로도 조회(IPv6 전용 업스트림) |
| `PUID` / `PGID` | 컨테이너를 실행하고 `./data`를 소유할 사용자와 그룹(기본값 `10001`). 예: NAS 사용자 |
| `CORVUS_ACME_DIRECTORY` | Let's Encrypt 대신 사용할 다른 CA의 ACME 디렉터리 URL. 예: 사설 step-ca `https://ca.internal/acme/acme/directory` |
| `LEGO_CA_CERTIFICATES` | ACME 클라이언트가 신뢰할 사설 CA의 PEM 파일(컨테이너에 마운트). 예: `/data/ca.pem` |

### 포워드 호스트

프록시 호스트는 다음 대상으로 전달할 수 있습니다.

- **다른 컨테이너**를 이름으로 지정. 예: `xoshop-web` 포트 `80`. Corvus가 그 컨테이너와 같은 Docker
  네트워크에 있어야 합니다(`corvus` 서비스에 해당 네트워크를 추가).
- **사설 또는 공인 IP**. 예: `10.3.2.39` 포트 `8084`.
- **Docker 호스트 자체의 서비스**를 NPM과 마찬가지로 `127.0.0.1` 또는 `localhost`로 지정. 컨테이너
  안에서 이 주소는 Docker 호스트를 뜻하므로 `host.docker.internal`이 필요 없습니다. 단, 서비스가
  컨테이너에서 접근할 수 있는 주소(`127.0.0.1`만이 아니라 `0.0.0.0` 또는 Docker 브리지)에서 수신해야
  하며, 호스트 방화벽이 Docker 네트워크에서 오는 연결을 허용해야 합니다.

호스트의 `127.0.0.1`에서만 수신하는 서비스에 연결하거나 `network_mode: host`를 쓰던 NPM 구성을 그대로
유지하려면 Corvus를 호스트 네트워킹으로 실행하세요. `ports:`와 `networks:`를 `network_mode: host`로
바꾸면 됩니다(호스트에서 6379를 이미 쓰고 있다면 `CORVUS_REDIS_PORT`도 설정).

### Nginx Proxy Manager에서 마이그레이션

콘솔의 *NPM 가져오기*는 기존 NPM 설치에서 **프록시 호스트, 리디렉션 호스트, 인증서**(Cloudflare DNS
토큰을 포함한 Let's Encrypt 인증서와 사용자 지정 인증서), **접근 목록**(사용자와 IP 규칙)을 복사합니다.
NPM의 파일과 데이터베이스를 읽기만 하고 NPM 자체는 변경하지 않으므로 언제든 되돌아갈 수 있습니다.

가져오지 않는 항목: 스트림(TCP/UDP), 404 호스트, 사용자 지정 위치, NPM 전용 변수를
사용하는 고급 설정. 건너뛰는 항목은 모두 미리보기에 표시됩니다.

**NPM이 어떤 데이터베이스를 쓰는지 확인하세요.** NPM의 `docker-compose.yml`을 열어 봅니다.

- `DB_MYSQL_*` / `DB_POSTGRES_*` 변수가 없고 `data/database.sqlite`가 있으면 → **SQLite**(NPM 기본값)
- `DB_MYSQL_HOST`, `DB_MYSQL_USER`, … 가 있고 보통 별도의 `db` 서비스가 있으면 → **MySQL / MariaDB**
- `DB_POSTGRES_*` → PostgreSQL은 가져오기를 지원하지 않으므로 콘솔에서 호스트를 다시 만드세요

두 경우 모두 NPM의 `data`와 `letsencrypt` 디렉터리를 Corvus에 읽기 전용으로 마운트해야 합니다(인증서
파일이 여기에 있습니다).

```yml
services:
  corvus:
    image: 'ghcr.io/corvus-gateway/gateway:latest'
    restart: unless-stopped
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
    environment:
      CORVUS_IMPORT_DIR: /import
    volumes:
      - ./data:/data
      - /path/to/npm/data:/import/npm/data:ro
      - /path/to/npm/letsencrypt:/import/npm/letsencrypt:ro
```

NPM의 `./data`와 `./letsencrypt` 볼륨의 호스트 경로를 사용하세요. NPM과 Corvus는 둘 다 80/443 포트가
필요하므로 동시에 실행할 수 없습니다.

#### SQLite를 쓰는 NPM(기본값)

```bash
cd /path/to/npm && docker compose down        # NPM 중지(80/443 해제)
cd /path/to/corvus && docker compose up -d     # 위 마운트로 Corvus 시작
```

콘솔에서 *NPM 가져오기*를 열고 **SQLite**를 그대로 둔 채 *미리보기*를 누른 뒤, 가져올 항목을 골라
*선택 항목 가져오기*를 누릅니다. 미리보기에서 `permission denied`가 나오면 Corvus(uid 10001로 실행)가
파일을 읽을 수 있게 하세요: `sudo chmod o+rx /path/to/npm/data && sudo chmod o+r /path/to/npm/data/database.sqlite`.

#### MySQL / MariaDB를 쓰는 NPM

가져오는 동안 Corvus가 NPM의 데이터베이스에 접속해야 하므로, NPM의 프록시 서비스만 중지하고
데이터베이스는 계속 실행해 둡니다.

```bash
cd /path/to/npm && docker compose stop app     # 80/443을 쓰는 NPM 서비스(보통 "app")
cd /path/to/corvus && docker compose up -d

# 가져오기를 위해 Corvus를 NPM의 Docker 네트워크에 연결
docker network ls | grep -i npm                # 예: npm_default
docker network connect npm_default $(docker compose ps -q corvus)
```

*NPM 가져오기*에서 **MySQL / MariaDB**를 고르고 NPM의 데이터베이스 설정을 입력합니다. 호스트 =
데이터베이스 서비스 이름(보통 `db`), 포트 `3306`, 그리고 `DB_MYSQL_NAME`, `DB_MYSQL_USER`,
`DB_MYSQL_PASSWORD`의 값입니다. 연결은 읽기 전용이며 비밀번호는 저장되지 않습니다. 그다음
*미리보기* → *선택 항목 가져오기*를 누릅니다.

가져오기가 끝나면:

```bash
docker network disconnect npm_default $(docker compose ps -q corvus)
cd /path/to/npm && docker compose down         # NPM 데이터베이스도 중지
```

#### 가져온 뒤

- 가져온 호스트를 확인한 다음 `CORVUS_IMPORT_DIR`와 두 개의 `/import` 마운트를 제거하고
  `docker compose up -d`를 다시 실행합니다.
- 인증서는 Corvus에서 계속 갱신됩니다. 지원하지 않는 DNS 공급자를 쓰던 호스트는 자동 갱신 없이
  가져오며, 미리보기에서 경고가 표시됩니다.
- **되돌리기:** Corvus에서 `docker compose down`, 그다음 NPM에서 `docker compose up -d`.

## 성능

GitHub 호스팅 `ubuntu-24.04` 러너(4 vCPU AMD EPYC 9V45)에서 [벤치마크 워크플로](../benchmark.md)로
측정했습니다. Corvus는 **2 vCPU**에 고정하고 wrk와 백엔드는 나머지 두 개에서 실행했으며, keep-alive
연결 64개로 3바이트 응답을 돌려주는 `GET /`를 요청했습니다.

| 시나리오 | req/s | p50 | p99 | 메모리 |
|---|---:|---:|---:|---:|
| nginx 1.28 리버스 프록시(기준) | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus, 기본 호스트 | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus, 규칙 30개 + CIDR 1,000개 목록 + 익스플로잇 차단 + 헤더 + 속도 제한 | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus, HTTPS(ECDSA P-256), keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus, HTTPS, 요청마다 새 TLS 핸드셰이크 | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus, 접속 대기열(입장한 방문자) | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus, 챌린지 규칙(통과 쿠키가 있는 방문자) | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

기본 요청 경로에서 Corvus는 일반 nginx 처리량의 약 4분의 3을 냅니다(업스트림 keep-alive 연결 풀,
워커별로 묶어 보내는 접근 로그). 전체 보호 기능(CIDR 1,000개 목록을 포함한 규칙 30개, 익스플로잇 차단,
보안 헤더, 속도 제한)을 켜면 여기서 약 28 %가 더 듭니다.
**주의:** 부하 생성기가 같은 머신에서 돌아가는 공유 CI 러너이고 응답도 아주 작습니다. 수치는
보수적이며 시나리오와 버전을 비교하기 위한 것이지 처리 용량을 보장하는 값이 아닙니다. 메모리에는
컨트롤 플레인과 내장 Valkey가 포함됩니다.

## 개발

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only, no OpenResty
```

| 경로 | 내용 |
|---|---|
| `gateway/` | 데이터 플레인: OpenResty + Lua (규칙, 본문 검사, 접속 대기열) |
| `control/` | 컨트롤 플레인(Go): 설정 DB, 설정 생성, 배포/롤백, API |
| `web/` | 관리 콘솔(React + Vite + TypeScript) |
| `deploy/` | Dockerfile, 검증된 OpenResty 빌드, Compose 파일, 통합 테스트 |
| `tools/` | 부하 테스트 및 규칙 번들 서명 도구 |

## 보안

취약점은 비공개로 제보해 주세요. 자세한 내용은 [SECURITY.md](../../SECURITY.md)를 참고하세요.

## 라이선스

[Apache License 2.0](../../LICENSE) © 2026 Jikwang Moon.
IP-국가 매핑 데이터는 [DB-IP](https://db-ip.com)(CC BY 4.0)에서 제공합니다. [NOTICE](../../NOTICE)를 참고하세요.
