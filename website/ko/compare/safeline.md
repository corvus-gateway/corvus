---
description: "Corvus와 SafeLine 비교: SafeLine은 의미 분석 엔진을 갖춘 셀프 호스팅 WAF로, 미리 빌드된 이미지로 배포되고 유료 에디션이 있습니다. Corvus는 WAF가 내장된 완전한 오픈 소스(Apache-2.0) 리버스 프록시입니다. 언제 무엇을 고를지 설명합니다."
title: "Corvus와 SafeLine 비교"
keywords: [SafeLine 대안, SafeLine Corvus 비교, 오픈 소스 WAF, 셀프 호스팅 WAF, Chaitin SafeLine, 완전 오픈 소스 WAF]
mentions:
  - { name: SafeLine, url: "https://github.com/chaitin/SafeLine" }
---

# Corvus와 SafeLine 비교

Chaitin이 만든 [SafeLine](https://waf.chaitin.com/)은 웹 콘솔을 갖춘 셀프 호스팅 WAF이자 리버스
프록시입니다. 탐지 엔진은 정규식 시그니처가 아니라 요청에 대한 **의미 분석**을 사용합니다. Rate
Limit, 안티봇 챌린지, 비밀번호 챌린지, 동적 페이지 암호화, 접속 대기열도 갖추고 있습니다. 무료
에디션과 함께 상용 에디션(Lite, Pro)이 있습니다.

## "오픈 소스"에 대해

SafeLine 저장소는 GPL-3.0이며 관리 서비스, SDK, 도구가 들어 있습니다. 하지만 탐지 엔진과 프록시는
저장소의 소스가 아니라 **미리 빌드된 컨테이너 이미지**로 제공됩니다(2026년 11월 기준). 모든 부분을
직접 읽고, 감사하고, 빌드해야 한다면 이 점이 중요합니다. Corvus는 모든 부분이 Apache-2.0으로
[저장소](https://github.com/corvus-gateway/corvus)에 있으며, 릴리스 이미지는 서명되어 있고
저장소에서 빌드됩니다.

## SafeLine을 고를 때

- 시그니처가 놓치는 난독화된 페이로드를 잡도록 설계된 **의미 분석 엔진**을 원합니다.
- 동적 HTML/JS 암호화 같은 부가 기능이나 ingress-nginx, Kong, Traefik용 플러그인을 원합니다.

## Corvus를 고를 때

- 직접 감사하고 빌드할 수 있는 **완전한 오픈 소스** 게이트웨이를 원합니다.
- 에디션에 따른 앱 수나 기능 제한 없이 **모든 기능을 무료로** 쓰고 싶습니다.
- NPM 스타일의 **완전한 리버스 프록시 콘솔**을 원합니다. [프록시 호스트](../proxy/proxy-hosts.md),
  [리디렉션](../proxy/redirection-hosts.md), [TCP/UDP 스트림](../proxy/streams.md),
  [DNS 인증을 지원하는 인증서](../proxy/certificates.md), [NPM 가져오기](../getting-started/migrate-npm.md).
- [실시간 트래픽, 요청 로그, 방문자 통계](../operations/traffic.md)와 Slack, Discord, Teams,
  Telegram 등으로 보내는 [알림](../operations/alerts.md)을 원합니다.

## 주요 차이점

| | SafeLine | Corvus |
|---|---|---|
| 소스 코드 | 관리 서비스는 저장소에, 엔진은 미리 빌드된 이미지로 | 모두 저장소에 |
| 라이선스 | 저장소는 GPL-3.0, 유료 Lite/Pro 에디션 | Apache-2.0, 유료 에디션 없음 |
| WAF 엔진 | 의미 분석 | 시그니처와 가상 패치 |
| Rate Limit, 봇 챌린지, 접속 대기열 | ✅ (어느 에디션인지 확인 필요) | ✅ |
| 리디렉션 호스트, TCP/UDP 스트림, NPM 가져오기 | 주된 목적이 아님 | ✅ |
| 실행 형태 | 여러 컨테이너 | 컨테이너 하나 |

[전체 비교](index.md)
