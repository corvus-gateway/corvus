---
description: "Corvus와 BunkerWeb 비교: BunkerWeb은 ModSecurity와 OWASP Core Rule Set, Kubernetes와 Swarm 자동 설정, PRO 에디션을 갖춘 nginx 기반 WAF이고, Corvus는 자체 WAF를 갖춘 더 단순한 리버스 프록시 콘솔이며 모두 무료입니다. 언제 무엇을 고를지 설명합니다."
title: "Corvus와 BunkerWeb 비교"
keywords: [BunkerWeb 대안, BunkerWeb Corvus 비교, 오픈 소스 WAF, ModSecurity, OWASP CRS, 셀프 호스팅 WAF, BunkerWeb PRO]
mentions:
  - { name: BunkerWeb, url: "https://github.com/bunkerity/bunkerweb" }
---

# Corvus와 BunkerWeb 비교

[BunkerWeb](https://www.bunkerweb.io/)(AGPL-3.0, 유료 PRO 에디션 있음)은 nginx 위에 만든
WAF입니다. 목적이 Corvus와 가장 가까운 프로젝트이며, 보안 기능의 폭은 BunkerWeb이 더 넓습니다.
OWASP Core Rule Set을 쓰는 ModSecurity, 나쁜 행동 차단, 요청 속도와 연결 수 제한, 다양한 안티봇
옵션(JavaScript, captcha, reCAPTCHA, hCaptcha, Turnstile…), 국가·ASN·DNSBL 차단, 위협 정보 공유를
갖추고 있습니다. 환경 변수, Docker/Swarm 라벨, Kubernetes ingress 컨트롤러, 또는 웹 UI로
설정합니다.

## BunkerWeb을 고를 때

- 예를 들어 규정 준수 체크리스트 때문에 paranoia level을 갖춘 **ModSecurity와 OWASP Core Rule
  Set**이 필요합니다.
- **Kubernetes, Swarm, Docker 라벨**로 배포합니다.
- 가장 많은 설정 스위치를 원하고, CRS 오탐을 직접 조정하는 것도 괜찮습니다.
- 일부 기능이 PRO에 있어도 괜찮습니다(모니터링과 리포트, 로드 밸런싱, 사용자 지정 페이지, 점검
  모드, S3 백업, OpenID Connect, 와일드카드 인증서 등).

## Corvus를 고를 때

- 직접 설정해야 하는 WAF보다는, 기본으로 켜져 있고 오탐이 적게 나오도록 조정된 보호를 갖춘
  **리버스 프록시 콘솔이 먼저**입니다.
- **모든 것을 무료로** 쓰고 싶습니다. Corvus에서는 [트래픽 관제](../operations/traffic.md),
  [사용자 지정 페이지와 점검 페이지](../proxy/proxy-hosts.md#serve-a-page-instead-of-a-backend),
  [여러 백엔드](../proxy/proxy-hosts.md#several-backends-round-robin),
  [백업](../operations/backup.md), [콘솔용 SSO](../operations/users.md),
  [와일드카드 인증서](../proxy/certificates.md)에 유료 등급이 없습니다.
- 트래픽이 몰릴 때를 위한 [접속 대기열](../access-queue.md)이나
  [자동 공격 대응 모드](../security/challenge.md#under-attack-mode)가 필요합니다.
- NPM에서 옮겨 오는 중입니다. Corvus는 [NPM 설정을 가져옵니다](../getting-started/migrate-npm.md).

## 주요 차이점

| | BunkerWeb | Corvus |
|---|---|---|
| WAF 엔진 | ModSecurity + OWASP CRS | 자체 시그니처(Corvus Sentinel), 가상 패치 템플릿 |
| 설정 방식 | 환경 변수, 라벨, Kubernetes, 웹 UI | 웹 콘솔, HTTP API |
| 차단, Rate Limit, 국가 규칙 | ✅ | ✅ |
| 봇 챌린지 | ✅ 서드파티 captcha를 포함한 다양한 옵션 | ✅ 작업 증명 또는 쿠키, 외부 서비스 없음 |
| 접속 대기열 | ❌ | ✅ |
| 모니터링, 리포트 | 무료는 기본 기능, PRO에서 더 많이 | ✅ 모두 무료 |
| Kubernetes | ✅ | ❌ |
| 라이선스 | AGPL-3.0 + PRO | Apache-2.0, 유료 에디션 없음 |

[전체 비교](index.md)
