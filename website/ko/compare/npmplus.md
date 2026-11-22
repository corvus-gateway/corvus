---
description: "Corvus와 NPMplus 비교: NPMplus는 HTTP/3, GoAccess, 선택 사항인 CrowdSec을 갖춘 NPM 포크이고, Corvus는 자체 WAF, 자동 차단, Rate Limit, 챌린지, 접속 대기열을 콘솔에 갖춘 별개의 프로젝트입니다. 언제 무엇을 고를지 설명합니다."
title: "Corvus와 NPMplus 비교"
keywords: [NPMplus 대안, NPMplus Corvus 비교, Nginx Proxy Manager 포크, NPM CrowdSec 연동, NPM WAF]
mentions:
  - { name: NPMplus, url: "https://github.com/ZoeyVid/NPMplus" }
  - { name: Nginx Proxy Manager, url: "https://github.com/NginxProxyManager/nginx-proxy-manager" }
---

# Corvus와 NPMplus 비교

[NPMplus](https://github.com/ZoeyVid/NPMplus)(AGPL-3.0)는 Nginx Proxy Manager의 포크입니다. NPM의
콘솔을 유지하면서 HTTP/3, GoAccess 통계, 선택 사항인 CrowdSec 연동을 더합니다. Corvus는 포크가
아닙니다. 사용 방식이 같은 자체 콘솔이 있고, 보호 기능을 내장하고 있습니다.

## NPMplus를 고를 때

- **NPM의 화면**에 최대한 가깝게 머물고 싶습니다.
- **HTTP/3**가 필요합니다.
- **CrowdSec**을 원합니다. 커뮤니티 차단 목록과, OWASP Core Rule Set을 쓰는 AppSec 컴포넌트입니다.
  NPMplus에서는 CRS 검사가 out-of-band로 실행된다는 점에 주의하세요. 일치한 요청 자체는 차단되지
  않지만, 여러 규칙에 걸린 IP는 차단됩니다.

## Corvus를 고를 때

- 프록시 옆에 **CrowdSec을 돌리지 않아도** 동작하는 보호를 원합니다. 일치한 요청은
  [도착하는 즉시 차단](../security/index.md)되고, 공격을 반복하는 IP는
  [자동으로 차단](../security/bans.md)됩니다.
- nginx 설정을 직접 쓰는 대신 **콘솔에서 Rate Limit과 국가 규칙**을 쓰고 싶습니다.
  [Rate Limit](../security/bans.md#rate-limits), 그리고 IP, 국가, 경로, 헤더, User-Agent 기준의
  [보안 규칙](../security/rules.md)이 있으며 기록만 하는 모드를 지원합니다.
- [브라우저 챌린지와 자동 공격 대응 모드](../security/challenge.md),
  [접속 대기열](../access-queue.md), [알림](../operations/alerts.md)을 원합니다.
- 허용 범위가 넓은 라이선스(Apache-2.0)를 선호합니다.

## 주요 차이점

| | NPMplus | Corvus |
|---|---|---|
| 콘솔 | NPM의 콘솔 | 자체 콘솔, 같은 사용 방식 |
| NPM에서 가져오기 | NPM 그 자체 (data 폴더 마이그레이션) | ✅ [NPM 가져오기](../getting-started/migrate-npm.md) |
| HTTP/3 | ✅ | ❌ |
| WAF | 선택 사항인 CrowdSec AppSec (OWASP CRS, out-of-band) | ✅ 내장, 인라인으로 차단 |
| 자동 차단 | CrowdSec 사용 시 | ✅ 내장 |
| Rate Limit, 국가 규칙 | nginx 설정 직접 작성 | ✅ 콘솔에서 |
| 통계 | GoAccess | 실시간 트래픽, 요청 로그, 방문자 통계, GoAccess용 내보내기 |
| 챌린지, 접속 대기열, 알림 | ❌ | ✅ |
| 라이선스 | AGPL-3.0 | Apache-2.0 |

[Corvus와 NPM 비교](../getting-started/compare-npm.md) · [전체 비교](index.md)
