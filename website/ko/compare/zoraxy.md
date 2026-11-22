---
description: "Corvus와 Zoraxy 비교: 둘 다 웹 콘솔을 갖춘 셀프 호스팅 리버스 프록시입니다. Zoraxy는 가동 상태 모니터와 플러그인을 갖춘 단일 Go 바이너리이고, Corvus는 WAF, 자동 차단, 봇 챌린지, 접속 대기열을 더합니다. 언제 무엇을 고를지 설명합니다."
title: "Corvus와 Zoraxy 비교"
keywords: [Zoraxy 대안, Zoraxy Corvus 비교, 홈랩 리버스 프록시, 리버스 프록시 웹 UI, Zoraxy WAF]
mentions:
  - { name: Zoraxy, url: "https://github.com/tobychui/zoraxy" }
---

# Corvus와 Zoraxy 비교

[Zoraxy](https://zoraxy.aroz.org/)(AGPL-3.0)는 Go로 만든 홈랩용 리버스 프록시입니다. 바이너리
하나, 웹 콘솔, ACME 인증서, TCP/UDP 스트림, IP와 국가 블랙리스트/화이트리스트, Rate Limit, 가동
상태 모니터, 플러그인 시스템을 갖추고 있습니다. Corvus는 같은 종류의 콘솔을 갖추고 있으며, 공격
차단에 더 무게를 둡니다.

## Zoraxy를 고를 때

- Docker 없이 쓰는 **단일 바이너리**를 원합니다.
- **가동 상태 모니터**와 홈 네트워크용 부가 기능이 마음에 들고, 플러그인으로 확장하고 싶습니다.
- 서비스가 대부분 비공개이고, 주로 라우팅과 인증서가 필요합니다.

## Corvus를 고를 때

- 서비스가 인터넷에 노출되어 있고 **WAF**를 원합니다. Zoraxy는 현재 공격 페이로드를 차단하지
  않습니다(README에는 공격 방어 모듈이 메인테이너를 찾는 중이라고 나와 있습니다). Corvus는
  [공격 경로와 페이로드](../security/index.md)를 기본으로 차단하고
  [공격을 반복하는 IP를 차단](../security/bans.md)합니다.
- [브라우저 챌린지와 공격 대응 모드](../security/challenge.md),
  [접속 대기열](../access-queue.md), [알림](../operations/alerts.md)을 원합니다.
- 호스트별 [요청 로그와 보안 이벤트](../operations/traffic.md)를 원합니다.
- NPM에서 옮겨 오는 중입니다. Corvus는 [NPM 설정을 가져옵니다](../getting-started/migrate-npm.md).

## 주요 차이점

| | Zoraxy | Corvus |
|---|---|---|
| 실행 형태 | 단일 바이너리 또는 컨테이너 | 컨테이너 |
| 콘솔 | ✅ | ✅ |
| Let's Encrypt, DNS 인증 | ✅ | ✅ |
| IP / 국가 목록, Rate Limit | ✅ | ✅ |
| WAF (공격 페이로드) | ❌ | ✅ |
| 자동 차단, 챌린지, 접속 대기열 | ❌ | ✅ |
| 가동 상태 모니터 | ✅ | ❌ (연결 확인과 업스트림 오류 경고) |
| 플러그인 | ✅ | ❌ |
| 라이선스 | AGPL-3.0 | Apache-2.0 |

[전체 비교](index.md)
