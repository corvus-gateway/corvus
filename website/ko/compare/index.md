---
description: "셀프 호스팅 리버스 프록시와 WAF를 솔직하게 비교합니다. Nginx Proxy Manager, NPMplus, Caddy, Traefik, Zoraxy, BunkerWeb, SafeLine, CrowdSec, Pangolin, 그리고 Corvus. 라이선스, 웹 UI, 내장 보안 기능, 어떤 구성에 어떤 도구가 맞는지 다룹니다."
title: "Corvus와 다른 리버스 프록시·WAF 비교"
keywords: [리버스 프록시 비교, Nginx Proxy Manager 대안, 셀프 호스팅 WAF, Caddy Traefik 비교, BunkerWeb 대안, SafeLine 대안, 오픈 소스 WAF, 홈랩 리버스 프록시]
mentions:
  - { name: Nginx Proxy Manager, url: "https://github.com/NginxProxyManager/nginx-proxy-manager" }
  - { name: NPMplus, url: "https://github.com/ZoeyVid/NPMplus" }
  - { name: Caddy, url: "https://github.com/caddyserver/caddy" }
  - { name: Traefik, url: "https://github.com/traefik/traefik" }
  - { name: Zoraxy, url: "https://github.com/tobychui/zoraxy" }
  - { name: BunkerWeb, url: "https://github.com/bunkerity/bunkerweb" }
  - { name: SafeLine, url: "https://github.com/chaitin/SafeLine" }
  - { name: CrowdSec, url: "https://github.com/crowdsecurity/crowdsec" }
  - { name: Pangolin, url: "https://github.com/fosrl/pangolin" }
---

# Corvus와 다른 리버스 프록시·WAF 비교

좋은 셀프 호스팅 리버스 프록시는 많고, 저마다 다른 것을 얻고 다른 것을 내줍니다. 이 페이지는
Corvus 메인테이너가 썼으니 그 점을 감안하고 읽어 주세요. 다른 도구가 더 나은 선택인 경우에는
그렇다고 밝히려고 했습니다. 사실 관계는 2026년 11월에 각 프로젝트의 공식 문서와 저장소를 기준으로
확인했습니다. 프로젝트는 계속 바뀌므로 중요한 내용은 각 프로젝트의 문서에서 확인하고, 여기에
오래된 내용이 있으면 [알려 주세요](https://github.com/corvus-gateway/corvus/discussions).

## 빠른 답

| 원하는 것 | 살펴볼 도구 |
|---|---|
| 웹 콘솔에 **더해** WAF, 차단, Rate Limit, 봇 챌린지, 접속 대기열, 트래픽 관제를 컨테이너 하나로, 모두 무료로 | **Corvus** |
| 프록시와 인증서를 위한 가장 단순한 웹 콘솔, 그 이상은 필요 없음 | [Nginx Proxy Manager](../getting-started/compare-npm.md) |
| NPM의 콘솔에 HTTP/3, GoAccess, CrowdSec | [NPMplus](npmplus.md) |
| 설정 파일 하나, 자동 HTTPS와 HTTP/3, UI 없음 | [Caddy](caddy.md) |
| Docker 라벨이나 Kubernetes 리소스에서 만들어지는 경로 | [Traefik](traefik.md) |
| 가동 상태 모니터를 갖춘 단일 바이너리 콘솔, 홈랩 스타일 | [Zoraxy](zoraxy.md) |
| OWASP Core Rule Set을 쓰는 ModSecurity, Kubernetes/Swarm 자동 설정 | [BunkerWeb](bunkerweb.md) |
| 의미 분석 방식의 WAF 엔진 | [SafeLine](safeline.md) |
| 이미 쓰는 프록시 옆에 두는 커뮤니티 차단 목록 | [CrowdSec](crowdsec.md) |
| **공인 IP나 열린 포트 없이** 서비스를 공개하고 앞에 SSO 두기 | [Pangolin](pangolin.md) |

## 나란히 비교

✅ 내장 · 🧩 플러그인, 추가 도구 또는 직접 설정 필요 · 💲 유료 에디션 전용 · ❌ 지원하지 않음

| | Corvus | NPM | NPMplus | Caddy | Traefik | Zoraxy | BunkerWeb | SafeLine | Pangolin |
|---|---|---|---|---|---|---|---|---|---|
| 라이선스 | Apache-2.0 | MIT | AGPL-3.0 | Apache-2.0 | MIT | AGPL-3.0 | AGPL-3.0 + PRO | 저장소는 GPL-3.0, 엔진은 바이너리만 제공 | AGPL-3.0 + 상용 라이선스 파일 |
| 유료 에디션 | 없음 | 없음 | 없음 | 없음 | Traefik Hub | 없음 | PRO | Lite / Pro | Enterprise |
| 웹 UI에서 관리 | ✅ | ✅ | ✅ | ❌ | 읽기 전용 대시보드 | ✅ | ✅ | ✅ | ✅ |
| Docker 라벨 / Kubernetes | ❌ | ❌ | ❌ | 🧩 | ✅ | ❌ | ✅ | 🧩 | ❌ |
| Let's Encrypt 자동 갱신 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| HTTP/3 | ❌ | ❌ | ✅ | ✅ | ✅ | ❓ | ❓ | ❓ | ❓ |
| WAF (공격 시그니처) | ✅ 자체 엔진 | ❌ | 🧩 CrowdSec AppSec | 🧩 Coraza | 💲 / 🧩 | ❌ | ✅ ModSecurity + CRS | ✅ 의미 분석 엔진 | ❌ |
| 자동 IP 차단 | ✅ | ❌ | 🧩 CrowdSec | ❌ | 🧩 | ❌ | ✅ | ❓ | 🧩 CrowdSec |
| Rate Limit | ✅ | 🧩 직접 설정 | 🧩 직접 설정 | 🧩 | ✅ | ✅ | ✅ | ✅ | ❓ |
| 국가 차단 | ✅ | ❌ | 🧩 | 🧩 | 🧩 | ✅ | ✅ | ❓ | ✅ |
| CAPTCHA 서비스 없는 봇 챌린지 | ✅ | ❌ | ❌ | ❌ | 🧩 | ❌ | ✅ | ✅ | ❌ |
| 트래픽이 몰릴 때 접속 대기열 | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ (에디션 불명확) | ❌ |
| UI의 요청 로그 / 트래픽 화면 | ✅ | ❌ | ✅ GoAccess | ❌ | ❌ | ❓ | ✅ | ✅ | ✅ 접근 로그 |
| 앱 앞의 SSO | ❌ | ❌ | ❌ | 🧩 forward_auth + IdP | 🧩 ForwardAuth + IdP | ❓ | 💲 | ❓ | ✅ |
| 터널, 포트를 열 필요 없음 | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ | ✅ |

❓ = 문서에 명확히 나와 있지 않음. CrowdSec은 리버스 프록시가 아니어서 표에 없습니다. 쓰고 있는
프록시에 탐지와 커뮤니티 차단 목록을 더해 주는 도구입니다([자세히](crowdsec.md)).

## Corvus가 맞지 않는 경우

- **모든 것을 코드로 설정합니다**(Docker 라벨, Kubernetes, GitOps). Corvus는 콘솔이나
  [HTTP API](../reference/api.md)로 관리합니다. Traefik이나 Caddy가 더 잘 맞습니다.
- **HTTP/3가 필요하거나**, 앱 앞에 SSO가 필요합니다. Corvus에는 현재 둘 다 없습니다.
- 규정 준수를 위해 paranoia level을 갖춘 **OWASP Core Rule Set이 필요합니다.** Corvus는 기본으로
  켜 둘 수 있게 조정한, 더 작은 자체 시그니처 세트를 씁니다.
- **서버에 공인 IP가 없습니다.** Corvus는 80·443 포트로 접속할 수 있어야 합니다. Pangolin이나
  Cloudflare Tunnel 같은 터널을 앞에 두거나, Corvus 대신 쓰세요.
- **오랜 사용 이력을 원합니다.** Corvus는 아직 초기(1.x)이고 메인테이너가 한 명입니다. NPM, Caddy,
  Traefik은 여러 해 동안 운영 환경에서 쓰였고 커뮤니티가 큽니다.

## Corvus가 돋보이는 점

- **모든 것이 무료입니다.** 유료 에디션을 위해 빼 둔 기능이 없습니다.
- **첫 호스트부터 보호가 켜져 있습니다.** 공격 경로 차단, 자동 차단, 요청 본문 검사는 설정이 필요
  없고, 규칙과 챌린지는 필요할 때 쓰면 됩니다.
- **보통 따로 붙이는 것들이 콘솔 하나에 있습니다.** WAF, 차단, Rate Limit, 봇 챌린지, 자동 공격
  대응 모드, 접속 대기열, 실시간 트래픽과 요청 로그, 알림.
- **NPM에서 쉽게 써 볼 수 있습니다.** NPM을 바꾸지 않고 NPM 설정을
  [가져옵니다](../getting-started/migrate-npm.md).
