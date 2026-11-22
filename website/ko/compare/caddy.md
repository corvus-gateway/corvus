---
description: "Corvus와 Caddy 비교: Caddy는 Caddyfile로 설정하는, 자동 HTTPS를 갖춘 빠른 웹 서버이고, Corvus는 WAF, 차단, Rate Limit, 접속 대기열이 내장되어 있고 웹 콘솔에서 관리하는 리버스 프록시입니다. 언제 무엇을 고를지 설명합니다."
title: "Corvus와 Caddy 비교"
keywords: [Caddy 대안, Caddy Corvus 비교, Caddy WAF, Caddy 웹 UI, Caddy Rate Limit, 셀프 호스팅 리버스 프록시]
mentions:
  - { name: Caddy, url: "https://github.com/caddyserver/caddy" }
---

# Corvus와 Caddy 비교

[Caddy](https://caddyserver.com/)는 Go로 만든 웹 서버이자 리버스 프록시(Apache-2.0)로, HTTPS를
기본으로 켜 주는 것으로 유명합니다. 사이트를 짧은 `Caddyfile`이나 JSON으로 기술하며, 공식 웹 UI는
없습니다. Corvus는 Caddy가 하는 일 가운데 리버스 프록시 부분을 맡고, 웹 콘솔에서 관리하며, 보안
계층이 내장되어 있습니다.

## Caddy를 고를 때

- 버전 관리되는 파일에 설정을 두는 것과 두 줄짜리 사이트 설정을 좋아합니다.
- 프록시뿐 아니라 **웹 서버**(정적 파일, FastCGI를 통한 PHP, 템플릿)도 필요합니다.
- **HTTP/3**, 온디맨드 TLS, 내부 이름을 위한 로컬 CA를 원합니다.
- 추가 기능을 위해 모듈을 넣어 Caddy를 빌드(`xcaddy`)하는 데 익숙합니다.

## Corvus를 고를 때

- 설정 파일보다 **웹 콘솔**을 선호하거나, 여러 사람이 프록시를 관리합니다.
- **모듈을 컴파일하지 않고** 보호 기능을 쓰고 싶습니다. Caddy의 Rate Limit
  ([caddy-ratelimit](https://github.com/mholt/caddy-ratelimit))과 WAF
  ([coraza-caddy](https://github.com/corazawaf/coraza-caddy))는 서드파티 모듈입니다. Corvus에는
  공격 경로 차단, [자동 차단](../security/bans.md), [Rate Limit](../security/bans.md#rate-limits),
  [보안 규칙](../security/rules.md), [브라우저 챌린지](../security/challenge.md)가 내장되어
  있습니다.
- **트래픽을 보고** 싶습니다. Corvus에는 [실시간 트래픽 관제와 요청 로그](../operations/traffic.md),
  [알림](../operations/alerts.md)이 있습니다. Caddy는 다른 도구가 읽을 로그를 남깁니다.
- 트래픽이 몰릴 때를 위한 [접속 대기열](../access-queue.md)이 필요합니다.

## 주요 차이점

| | Caddy | Corvus |
|---|---|---|
| 설정 방식 | Caddyfile / JSON / admin API | 웹 콘솔 / HTTP API |
| 자동 HTTPS | ✅ (Let's Encrypt, ZeroSSL, 로컬 CA) | ✅ (Let's Encrypt) |
| DNS 인증 | DNS 업체별 모듈 | 7개 업체 내장 |
| HTTP/3 | ✅ | ❌ |
| 정적 파일 서버, FastCGI | ✅ | ❌ ([정적 페이지](../proxy/proxy-hosts.md#serve-a-page-instead-of-a-backend)만) |
| WAF, 차단, Rate Limit | 서드파티 모듈 | ✅ 내장 |
| 트래픽 관제, 알림 | ❌ | ✅ |
| 라이선스 | Apache-2.0 | Apache-2.0 |

Caddy는 더 가볍고 커뮤니티가 훨씬 큽니다. Corvus는 그 일부를 내주고 콘솔과 보호 계층을 컨테이너
하나에 담았습니다. [전체 비교](index.md)
