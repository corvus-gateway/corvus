---
description: "Corvus AWPG는 웹 방화벽(WAF)을 내장한 무료 오픈소스 셀프 호스팅 리버스 프록시입니다. Nginx Proxy Manager처럼 쉬운 프록시 호스트와 무료 SSL에 공격 차단, 자동 차단, 요청 속도 제한, 트래픽 관제, 접속 대기열을 더했습니다."
title: Corvus AWPG
hide:
  - navigation
---

# Corvus AWPG

**Corvus는 웹 방화벽(WAF)을 내장한 무료 오픈소스 셀프 호스팅 리버스 프록시입니다.** Nginx Proxy
Manager처럼 쉽게 운영할 수 있고(도메인 연결, 전달, 무료 SSL을 몇 번의 클릭으로), 지나가는 트래픽을
**관제하고, 보호하고, 제어**합니다. 모든 것이 내 서버에서 동작하며, 트래픽이 외부 클라우드로 나가지
않습니다. Apache-2.0 라이선스, Docker 컨테이너 하나, 모든 기능 무료입니다.

Corvus가 80·443 포트로 요청을 받아 뒤에 있는 알맞은 앱이나 웹 서버로 넘겨 주므로, 서버 한 대에서 여러
사이트를 HTTPS와 보호 기능을 갖춘 채 운영할 수 있습니다. 이미 쓰던 웹 서버 앞에 Corvus를 두는 방법까지
[동작 방식](getting-started/how-it-works.md)에서 설명합니다.

![Corvus 데모](img/demo.gif){ .shot }

<div class="grid cards" markdown>

- :material-rocket-launch: **[5분 만에 설치](getting-started/install.md)**

    컨테이너 하나, `docker-compose.yml` 하나, 그리고 브라우저만 있으면 됩니다.

- :material-swap-horizontal: **[NPM을 쓰고 계신가요?](getting-started/migrate-npm.md)**

    [Corvus와 무엇이 다른지](getting-started/compare-npm.md) 확인하고 프록시 호스트, 인증서, 스트림,
    접근 목록을 가져오세요. NPM 자체는 변경하지 않습니다.

- :material-shield-check: **[보호](security/index.md)**

    호스트를 추가하는 순간부터 공격 차단이 동작합니다. 규칙, 차단, 챌린지는 필요할 때 쓰면 됩니다.

- :material-account-clock: **[접속 대기열](access-queue.md)**

    출시나 한정 판매 때 감당할 수 있는 만큼만 방문자를 입장시키고, 나머지는 공정하게 대기시킵니다.

</div>

## 주요 기능

| | |
|---|---|
| **프록시와 TLS** | [여러 백엔드](proxy/proxy-hosts.md#several-backends-round-robin)(라운드 로빈과 장애 조치)로 요청을 나누고 [메모](proxy/proxy-hosts.md#notes)를 남길 수 있으며 [연결 확인](proxy/proxy-hosts.md#check-connection)과 [업스트림 오류 경고](proxy/proxy-hosts.md#upstream-errors-in-the-list)를 갖춘 프록시 호스트, [사용자 지정 위치](proxy/locations.md), [백엔드 없이 표시하는 페이지](proxy/proxy-hosts.md#serve-a-page-instead-of-a-backend)(준비 중, 점검 중, 주차된 도메인 또는 직접 작성한 HTML), [리디렉션](proxy/redirection-hosts.md), [TCP/UDP 스트림](proxy/streams.md)(TCP는 [PROXY protocol](proxy/streams.md#proxy-protocol) 지원), 자동 갱신되는 [Let's Encrypt 인증서](proxy/certificates.md)와 저장해 두고 다시 쓰는 DNS 인증 정보, Cloudflare·프록시·TCP 터널([PROXY protocol](proxy/proxy-hosts.md#behind-a-tcp-tunnel-or-load-balancer-proxy-protocol)) 뒤에서도 [실제 클라이언트 IP](proxy/proxy-hosts.md#client-ip) 확인, [알 수 없는 호스트](operations/pages.md#unknown-hosts) 응답 선택(404, 연결 끊기, 리디렉션) |
| **보안** | 알려진 공격 패턴 기본 차단, [보안 규칙](security/rules.md), [IP·User-Agent 목록](security/ip-lists.md)(수백만 항목의 [대형 차단 목록](security/ip-lists.md#large-blocklists) 포함), [브라우저 챌린지와 공격 대응 모드](security/challenge.md), [자동 차단](security/bans.md) |
| **접속 대기열** | 트래픽 급증에 대비한 [정원 기반 대기실](access-queue.md) |
| **관제** | [실시간 트래픽, 요청 로그, 보안 이벤트](operations/traffic.md), 유입 출처·UTM과 GoAccess용 내보내기를 갖춘 [방문자 통계](operations/traffic.md#visitor-statistics), Slack·Discord·Teams·Telegram 등으로 보내는 [알림](operations/alerts.md) |
| **운영** | 모든 변경은 검사 후 버전으로 관리, [백업과 복원](operations/backup.md), [역할·MFA·SSO](operations/users.md), [새 버전 알림](reference/configuration.md) |

## 더 알아보기

- **가이드:** [인터넷에 공개한 셀프 호스팅 서비스 보안](guides/secure-self-hosted.md) ·
  [봇과 스캐너 차단](guides/block-bots.md) · [트래픽 폭주와 DDoS](guides/ddos.md) ·
  [Nginx Proxy Manager에 WAF 더하기](guides/waf-for-npm.md)
- **비교:** [Corvus와 다른 리버스 프록시·WAF](compare/index.md): Nginx Proxy Manager, NPMplus, Caddy,
  Traefik, Zoraxy, BunkerWeb, SafeLine, CrowdSec, Pangolin, 그리고 Corvus가 맞지 않는 경우
- **[자주 묻는 질문](faq.md):** 라이선스, 요구 사항, Corvus가 하는 일과 하지 않는 일

!!! tip "안전한 기본값"
    시작할 때 규칙을 하나도 쓸 필요가 없습니다. 새 호스트마다 기본 공격 탐지가 켜져 있고,
    고급 옵션은 모두 선택 사항입니다.
