---
description: "Corvus와 Pangolin 비교: Pangolin은 포트를 열지 않고 WireGuard 터널로 서비스를 공개하고 앱 앞에 SSO를 더합니다. Corvus는 공인 IP가 있는 서버를 위한, WAF, 차단, 접속 대기열을 갖춘 리버스 프록시입니다. 언제 무엇을 고를지, 둘을 함께 쓰는 방법을 설명합니다."
title: "Corvus와 Pangolin 비교"
keywords: [Pangolin 대안, Pangolin Corvus 비교, Cloudflare Tunnel 대안, 셀프 호스팅 터널, 제로 트러스트 리버스 프록시, WAF]
mentions:
  - { name: Pangolin, url: "https://github.com/fosrl/pangolin" }
---

# Corvus와 Pangolin 비교

[Pangolin](https://pangolin.net/)은 터널 방식의 리버스 프록시이자 접근 플랫폼입니다. 서비스 옆에
둔 작은 커넥터가 Pangolin 서버(보통 VPS)로 나가는 WireGuard 터널을 열기 때문에, **공인 IP나 열린
포트 없이** 서비스를 공개할 수 있고, 앞에 SSO, 역할, PIN이나 이메일 코드를 둘 수 있습니다.
내부적으로 Traefik을 사용합니다. 코드는 AGPL-3.0이며 일부 파일은 상용 라이선스(Enterprise
에디션)입니다.

둘은 서로 다른 문제를 풉니다. Pangolin은 서비스에 **도달하게 하고** **출입을 통제하는** 것이고,
Corvus는 공개된 웹 트래픽을 **보호하는** 것입니다.

## Pangolin을 고를 때

- 서버가 **CGNAT** 뒤에 있거나, 포트를 열 수 없거나 열고 싶지 않습니다.
- 비공개 앱 앞에 **신원 기반 접근 제어**(SSO, 사용자, 역할)를 원합니다. Access를 곁들인 Cloudflare
  Tunnel의 셀프 호스팅 대안입니다.

## Corvus를 고를 때

- 서비스가 **공개**되어 있고(누구나 방문할 수 있음) 이를 노리는 공격을 막고 싶습니다.
  [WAF](../security/index.md), [자동 차단](../security/bans.md),
  [Rate Limit](../security/bans.md#rate-limits), [브라우저 챌린지](../security/challenge.md),
  [접속 대기열](../access-queue.md)이 있습니다.
- 서버에 공인 IP와 80·443 포트가 있습니다.

## 둘을 함께 쓰기

Pangolin이 트래픽을 Corvus까지 실어 나를 수 있습니다. Pangolin 리소스가 Corvus를 향하게 하고,
보호 적용과 앱으로의 라우팅은 Corvus에 맡기세요. Corvus가 실제 방문자 IP를 볼 수 있도록
[PROXY protocol이나 신뢰하는 프록시](../proxy/proxy-hosts.md#client-ip)를 켜세요.

## 주요 차이점

| | Pangolin | Corvus |
|---|---|---|
| 공인 IP / 열린 포트 필요 | 아니요 (터널) | 예 |
| 앱 앞의 SSO와 역할 | ✅ | ❌ (콘솔만) |
| WAF | ❌ | ✅ |
| 자동 차단 | CrowdSec 사용 시 | ✅ 내장 |
| 국가 규칙 | ✅ | ✅ |
| 챌린지, 접속 대기열 | ❌ | ✅ |
| 라이선스 | AGPL-3.0 + 상용 라이선스 파일 | Apache-2.0 |

[전체 비교](index.md)
