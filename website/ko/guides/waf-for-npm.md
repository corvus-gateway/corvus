---
description: "Nginx Proxy Manager에는 WAF, 자동 차단, Rate Limit이 없습니다. 이를 더하는 네 가지 방법(NPM 옆에 CrowdSec이나 fail2ban 두기, NPMplus, 직접 쓰는 nginx 설정, 보호 기능이 내장된 프록시로 옮기기)과 각각의 장단점을 설명합니다."
keywords: [Nginx Proxy Manager WAF, NPM 보안 설정, NPM fail2ban 연동, NPM CrowdSec 연동, NPM Rate Limit, Nginx Proxy Manager 보안, WAF 있는 NPM 대안]
mentions:
  - { name: Nginx Proxy Manager, url: "https://github.com/NginxProxyManager/nginx-proxy-manager" }
  - { name: CrowdSec, url: "https://github.com/crowdsecurity/crowdsec" }
  - { name: NPMplus, url: "https://github.com/ZoeyVid/NPMplus" }
---

# Nginx Proxy Manager에 WAF, 차단, Rate Limit을 더하는 방법

Nginx Proxy Manager(NPM)는 프록시와 인증서를 쉽게 만들어 주지만, 보안 기능은 Access List, Basic
auth, 그리고 "Block common exploits" 스위치에서 끝납니다. 자동 차단이 없고, 콘솔에서 설정하는
Rate Limit이 없고, 국가 규칙이 없고, 무엇이 차단되었는지 볼 화면도 없습니다. 이 빈틈을 메우는 네
가지 방법을 소개합니다.

## 1. NPM 옆에 fail2ban이나 CrowdSec 두기

둘 다 NPM의 접근 로그를 읽고 문제를 일으키는 IP를 차단합니다.

- **fail2ban**: 로그 형식에 맞는 필터(정규식)와 jail을 작성합니다. 차단은 호스트 방화벽에
  들어갑니다. Docker를 쓴다면 `DOCKER-USER` 체인에서 차단해야 하며, 그러지 않으면 공개된 포트에는
  차단이 적용되지 않습니다.
- **CrowdSec**: 엔진, NPM 로그용 컬렉션, 방화벽 바운서를 설치합니다. 커뮤니티 차단 목록도 함께
  쓸 수 있습니다.

장점: NPM을 그대로 둡니다. 비용: 관리할 서비스가 하나 더 늘고, 차단이 사후에(로그를 보고)
이루어지며, Cloudflare나 다른 프록시 뒤에서는 클라이언트 IP를 방화벽에서 차단해도 효과가 없습니다.

## 2. NPM에서 nginx 설정 직접 추가하기

호스트의 **Advanced** 탭에서 Rate Limit용 `limit_req`나 `deny` 규칙을 추가할 수 있습니다. 동작은
하지만, Rate Limit 영역(zone)을 별도의 `http` include에 정의해야 하고, 콘솔에는 아무것도 보이지
않으며, 오타 하나로 nginx가 다시 로드되지 않을 수 있습니다.

## 3. NPMplus

[NPMplus](../compare/npmplus.md)는 선택 사항인 CrowdSec 연동, HTTP/3, GoAccess를 갖춘 NPM
포크입니다. NPM의 화면을 그대로 쓰고 싶다면 좋은 선택입니다.

## 4. 보호 기능이 내장된 프록시

NPM에 도구를 세 개씩 붙이고 있다면, 그 기능을 이미 갖춘 프록시가 손이 덜 갈 수 있습니다.
[Corvus](../index.md)는 NPM의 사용 방식을 유지하면서 위 방법들이 더하는 것을 내장하고 있습니다.

| NPM에 없는 것 | Corvus에서 |
|---|---|
| WAF | [공격 경로와 요청 본문을 기본으로 차단](../security/index.md) |
| 자동 차단 | [내장](../security/bans.md). 프록시에서 직접 적용하므로 Cloudflare 뒤에서도 동작합니다 |
| Rate Limit | [호스트별, 콘솔에서 설정](../security/bans.md#rate-limits) |
| 국가, 헤더, User-Agent 규칙 | 기록만 하는 모드가 있는 [보안 규칙](../security/rules.md) |
| 봇 챌린지 | [챌린지와 공격 대응 모드](../security/challenge.md) |
| 무엇이 차단되었는지 보기 | [트래픽 관제와 이벤트](../operations/traffic.md), [알림](../operations/alerts.md) |

NPM을 포기하지 않고도 써 볼 수 있습니다. [NPM 가져오기](../getting-started/migrate-npm.md)는 프록시
호스트, 리디렉션, 스트림, 인증서, Access List를 복사하고 NPM은 건드리지 않습니다.
[Corvus와 NPM 비교](../getting-started/compare-npm.md)에는 NPM이 아직 앞서는 점(더 많은 DNS 업체,
더 오랜 사용 이력, 더 가벼운 자원 사용)이 정리되어 있습니다.

BunkerWeb, SafeLine처럼 WAF가 내장된 다른 선택지는 [비교](../compare/index.md)에 있습니다.
