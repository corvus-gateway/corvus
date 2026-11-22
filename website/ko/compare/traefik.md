---
description: "Corvus와 Traefik 비교: Traefik은 Docker 라벨과 Kubernetes 리소스에서 경로를 만들고, Corvus는 무료 내장 WAF, 차단, 봇 챌린지, 접속 대기열을 갖추고 웹 콘솔에서 관리합니다. 언제 무엇을 고를지 설명합니다."
title: "Corvus와 Traefik 비교"
keywords: [Traefik 대안, Traefik Corvus 비교, Traefik WAF, Traefik 웹 UI, Traefik CrowdSec 연동, 셀프 호스팅 리버스 프록시]
mentions:
  - { name: Traefik, url: "https://github.com/traefik/traefik" }
---

# Corvus와 Traefik 비교

[Traefik Proxy](https://traefik.io/traefik/)(MIT)는 Go로 만든 클라우드 네이티브 리버스 프록시입니다.
강점은 **서비스 디스커버리**입니다. Docker 라벨, Kubernetes Ingress, 자체 CRD를 읽어 경로를
자동으로 만듭니다. 대시보드는 경로를 보여 주지만 편집하지는 못합니다. Corvus는 반대 방식입니다.
콘솔에서 호스트를 추가하고, 보안 계층이 내장되어 있습니다.

## Traefik을 고를 때

- 서비스가 컨테이너이고, 시작할 때 **라벨만으로 경로가 생기기를** 원합니다.
- **Kubernetes**나 Docker Swarm을 운영하거나, 모든 것을 Git에 둡니다.
- ForwardAuth(Authelia나 Authentik용), 서킷 브레이커, 동시 처리 요청 수 제한 같은 미들웨어가
  필요합니다.
- HTTP/3를 원합니다.

## Corvus를 고를 때

- 라벨과 YAML 대신 **웹 콘솔에서 호스트를 관리**하고 싶거나, 컨테이너가 아닌 서비스로 프록시합니다.
- **유료 에디션 없이 WAF**를 쓰고 싶습니다. Traefik에서 네이티브 WAF 미들웨어는 Traefik Hub에
  포함되어 있고, 그렇지 않으면 플러그인(Coraza WASM, CrowdSec 바운서)을 써야 합니다. Corvus에는
  [공격 경로 차단](../security/index.md), [자동 차단](../security/bans.md),
  [국가와 헤더 규칙](../security/rules.md), [브라우저 챌린지](../security/challenge.md)가 내장되어
  있고 무료입니다.
- 같은 UI에서 [실시간 트래픽 관제, 요청 로그](../operations/traffic.md),
  [알림](../operations/alerts.md)을 쓰고 싶습니다.
- 트래픽이 몰릴 때를 위한 [접속 대기열](../access-queue.md)이 필요합니다.

## 주요 차이점

| | Traefik Proxy | Corvus |
|---|---|---|
| 설정 방식 | Docker 라벨, Kubernetes, 파일 | 웹 콘솔, HTTP API |
| 대시보드 | 경로를 읽기 전용으로 표시 | 모든 관리 가능 |
| Let's Encrypt, DNS 인증 | ✅ 다양한 업체 | ✅ 7개 업체 |
| Rate Limit, IP 허용 목록 | ✅ 미들웨어 | ✅ |
| WAF | Traefik Hub(유료) 또는 플러그인 | ✅ 내장 |
| 자동 차단, 국가 규칙, 봇 챌린지 | 플러그인 | ✅ 내장 |
| UI의 트래픽 관제, 요청 로그 | ❌ (다른 도구를 위한 로그와 메트릭) | ✅ |
| Kubernetes ingress | ✅ | ❌ |
| 라이선스 | MIT (Hub는 상용) | Apache-2.0, 유료 에디션 없음 |

둘을 함께 쓸 수도 있습니다. 클러스터 안의 디스커버리는 Traefik이 맡고, 그 앞에 Corvus를 보호된
진입점으로 두는 방식입니다. [전체 비교](index.md)
