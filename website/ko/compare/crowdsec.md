---
description: "Corvus와 CrowdSec: CrowdSec은 커뮤니티 차단 목록을 갖춘 탐지 엔진으로 프록시에 바운서가 필요하고, Corvus는 차단과 WAF가 내장된 리버스 프록시입니다. 둘이 어떻게 다른지, Corvus에서 차단 목록을 쓰는 방법을 설명합니다."
title: "Corvus와 CrowdSec"
keywords: [CrowdSec 대안, CrowdSec Corvus 비교, fail2ban 대안, 커뮤니티 차단 목록, 자동 IP 차단, 리버스 프록시 바운서]
mentions:
  - { name: CrowdSec, url: "https://github.com/crowdsecurity/crowdsec" }
---

# Corvus와 CrowdSec

[CrowdSec](https://www.crowdsec.net/)(MIT)은 리버스 프록시가 아닙니다. 로그(AppSec 컴포넌트를 쓰면
HTTP 요청도)를 읽어 공격을 탐지하고 어떤 IP를 차단할지 결정합니다. 그 결정은 nginx, Traefik,
방화벽 등에 따로 설치하는 **바운서**가 적용합니다. 가장 잘 알려진 기능은 많은 설치 환경의 신고로
만들어지는 **커뮤니티 차단 목록**입니다.

Corvus는 탐지와 적용을 **프록시 안에서** 합니다. 공격 요청을 도착하는 즉시 차단하고,
[공격을 반복하는 IP를 차단](../security/bans.md)하며, 로그 파싱이나 별도 서비스가 필요 없습니다.

## CrowdSec을 쓸 때

- **커뮤니티 차단 목록**을 원합니다. 다른 사용자가 신고한 IP가 나에게 닿기 전에 차단됩니다.
- SSH, 메일, VPN, 웹 등 **여러 서비스**를 여러 서버에 걸쳐 엔진 하나로 다루고 싶습니다.
- 바운서가 있는 프록시를 이미 운영하고 있고, 바꾸고 싶지 않습니다.

## Corvus를 쓸 때

- 설치하고, 설정하고, 업데이트해야 하는 **서비스를 하나 더 두지 않고 차단과 WAF**를 쓰고 싶습니다.
- 차단마다 **이유**를 같은 콘솔에서 보고 싶습니다.
  [보안 이벤트와 요청 로그](../operations/traffic.md), 그리고 [알림](../operations/alerts.md)이
  있습니다.
- 웹 서비스에 Rate Limit, [브라우저 챌린지](../security/challenge.md),
  [접속 대기열](../access-queue.md)도 쓰고 싶습니다.

## Corvus의 차단 목록

Corvus는 URL에서 차단 목록을 [IP 목록](../security/ip-lists.md#lists-from-a-url)으로 불러와
백그라운드에서 갱신할 수 있습니다. 수백만 항목의 [대형 목록](../security/ip-lists.md#large-blocklists)은
작은 크기로 저장되고 모든 워커가 공유합니다. 이 목록을 [보안 규칙](../security/rules.md)에서 사용해
차단하거나 챌린지를 걸 수 있습니다.

SSH를 비롯해 웹이 아닌 서비스에는 CrowdSec이나 fail2ban을 계속 쓰세요. Corvus는 자신을 지나가는
트래픽만 봅니다.

[전체 비교](index.md)
