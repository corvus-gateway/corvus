---
description: Corvus와 Nginx Proxy Manager(NPM) 비교 — 같은 점, Corvus가 더하는 것(WAF, 자동 차단, Rate Limit, 접속 대기열, 트래픽 관제), 아직 NPM이 앞서는 점.
---

# Corvus와 Nginx Proxy Manager 비교

Corvus는 [Nginx Proxy Manager](https://nginxproxymanager.com/)(NPM)의 간단함을 좋아하면서, 서비스로
들어오는 트래픽도 보고 제어하고 싶은 사람을 위해 만들었습니다. NPM 포크가 아니라 새로 만든
프로젝트입니다. 평소 쓰는 방식은 같습니다. 도메인을 넣고, 대상을 고르고, 인증서를 요청합니다.
여기에 보호와 관제 기능이 같은 콘솔에 들어 있습니다.

## 한눈에 비교

| | NPM | Corvus |
|---|---|---|
| 프록시 호스트, 사용자 지정 위치, 리디렉션, TCP/UDP 스트림 | ✅ | ✅ |
| Let's Encrypt 자동 갱신 | ✅ | ✅ |
| 와일드카드 인증서 (DNS 인증) | ✅ 다양한 DNS 업체 | ✅ 7개 업체(Cloudflare, DigitalOcean, DuckDNS, Gandi, GoDaddy, Hetzner, Namecheap). 토큰을 [저장해 여러 인증서에서 공유](../proxy/certificates.md) |
| 비밀번호 보호(Basic auth)와 IP 접근 제한 | ✅ Access List | ✅ 비밀번호 보호와 [보안 규칙](../security/rules.md) |
| 호스트별 nginx 설정 직접 추가 | ✅ | ✅ |
| 등록되지 않은 호스트 응답 (404, 연결 끊기, 리다이렉트) | ✅ Default Site | ✅ [알 수 없는 호스트](../operations/pages.md#unknown-hosts) |
| 알려진 공격 경로 차단 | ✅ 켜기/끄기 | ✅ 기본으로 켜짐, [유형별 분류와 기록](../security/index.md) |
| 공격을 반복하는 IP 자동 차단 | ❌ (CrowdSec, fail2ban 별도 구성) | ✅ [내장](../security/bans.md) |
| IP·국가·경로·헤더·User-Agent 규칙 | ❌ | ✅ [보안 규칙](../security/rules.md). *기록* 모드로 먼저 시험 가능 |
| Rate Limit | ❌ (직접 설정) | ✅ [호스트별](../security/bans.md#rate-limits) |
| 브라우저 챌린지와 자동 공격 대응 모드 | ❌ | ✅ [내장](../security/challenge.md), 외부 서비스 불필요 |
| 트래픽이 몰릴 때 접속 대기열 | ❌ | ✅ [내장](../access-queue.md) |
| 실시간 트래픽 관제와 요청 로그 | ❌ (접근 로그 파일) | ✅ [호스트별](../operations/traffic.md) |
| 알림 (Slack, Discord, Teams, Telegram 등) | ❌ | ✅ [내장](../operations/alerts.md) |
| 설정 버전 관리와 롤백 | ❌ | ✅ 변경마다 `openresty -t`로 검사하고 [버전으로 남김](../proxy/proxy-hosts.md) |
| 사용자와 역할 | ✅ | ✅ 관리자·운영자·조회자, [SSO(OIDC)](../operations/users.md), API 토큰, 감사 로그 |
| 백업 | 수동 (data 폴더 복사) | ✅ 콘솔에서 [내려받기와 복원](../operations/backup.md) |
| 데이터베이스 | SQLite, MySQL/MariaDB, PostgreSQL | SQLite(기본) 또는 PostgreSQL |

작성 시점의 NPM 기준이며, 이후 NPM에 기능이 추가됐을 수 있습니다.

## 아직 NPM이 앞서는 점

- 와일드카드 인증서용 **DNS 업체가 더 많습니다.** 쓰는 업체가 Corvus 목록에 없다면 HTTP 인증이나
  [직접 올린 인증서](../proxy/certificates.md)를 쓰세요.
- **훨씬 오래 쓰였고** 커뮤니티가 큽니다. Corvus는 아직 초기(1.x)입니다.
- **가볍습니다.** Corvus는 트래픽 수집기와 접속 대기열 저장소도 함께 돌리기 때문에 NPM보다 메모리를
  더 씁니다([성능](../reference/performance.md) 참고).

## NPM에서 옮기기

**NPM 가져오기**는 NPM 설정(프록시 호스트, 리디렉션, 스트림, 인증서, Access List)을 읽기만 하고 NPM은
바꾸지 않으므로 언제든 돌아갈 수 있습니다. [NPM에서 이전](migrate-npm.md)을 참고하세요.
