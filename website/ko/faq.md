---
description: "Corvus AWPG에 대해 자주 묻는 질문입니다. 무엇인지, 라이선스와 비용, Nginx Proxy Manager와의 차이, WAF와 자동 차단, DDoS, 요구 사항, 이전 방법, 그리고 하지 않는 일을 다룹니다."
faq: true
---

# 자주 묻는 질문

## Corvus 소개

### Corvus AWPG는 무엇인가요?

Corvus AWPG(Active Web Protection Gateway)는 웹 애플리케이션 방화벽(WAF)이 내장된 셀프 호스팅
리버스 프록시입니다. 80·443 포트에서 웹 서비스 앞에 자리 잡고, 무료 Let's Encrypt 인증서를 받고,
도메인마다 알맞은 서비스로 요청을 전달하면서 그 과정에서 공격을 막습니다. 공격 경로 차단, 보안
규칙, 자동 IP 차단, Rate Limit, 브라우저 챌린지, 그리고 트래픽이 몰릴 때를 위한 접속 대기열이
들어 있습니다. Nginx Proxy Manager처럼 웹 콘솔에서 관리합니다. Docker 컨테이너 하나(OpenResty + Go)로
동작하며 amd64와 arm64를 지원합니다.

### Corvus는 무료이고 오픈 소스인가요?

네. Corvus는 Apache-2.0 라이선스이며 모든 기능이 무료입니다. 유료 에디션도, "프로" 버전을 위해
빼 둔 기능도, 만들어야 하는 계정도 없습니다. 소스 코드는
[GitHub](https://github.com/corvus-gateway/corvus)에 있습니다.

### Corvus는 누구를 위한 것인가요?

웹 앱(Nextcloud, Vaultwarden, Immich, Home Assistant, 블로그, 쇼핑몰, API)을 인터넷에 공개하는
셀프 호스팅 사용자, 홈랩 사용자, 작은 팀을 위한 것입니다. 리버스 프록시, WAF, fail2ban, 로그
뷰어를 손으로 하나씩 엮지 않고도 서비스 앞에 HTTPS와 보호 기능을 두고 싶은 분에게 맞습니다.

### 내 트래픽이 외부 클라우드로 나가나요?

아니요. 요청, 로그, 통계는 내 서버에 그대로 있습니다. 밖으로 나가는 연결은 Let's Encrypt(인증서),
새 릴리스를 확인하려고 하루 두 번 접속하는 GitHub(끌 수 있으며, 설치 환경에 대한 정보는 아무것도
보내지 않습니다), 그리고 직접 설정한 알림 채널과 선택 사항인 규칙 번들뿐입니다.

## 다른 도구와 비교

### Corvus는 Nginx Proxy Manager와 무엇이 다른가요?

평소 쓰는 방식은 같습니다. 도메인을 넣고, 대상을 고르고, 인증서를 요청합니다. Corvus는 NPM이
추가 도구에 맡기는 것을 더합니다. 자동 차단, 보안 규칙(IP, 국가, 경로, 헤더, User-Agent),
Rate Limit, 브라우저 챌린지와 공격 대응 모드, 접속 대기열, 실시간 트래픽 관제, 알림입니다. NPM
포크가 아니며, 기존 NPM 설정을 가져올 수 있습니다.
[Corvus와 NPM 비교](getting-started/compare-npm.md)를 참고하세요.

### 어떤 리버스 프록시를 골라야 하나요?

무엇이 필요한지에 따라 다릅니다. 설정 파일이나 Docker 라벨을 선호한다면 Caddy와 Traefik이
훌륭합니다. 서버에 공인 IP가 없다면 Pangolin, OWASP CRS 기반이나 의미 분석 방식의 WAF를 원한다면
BunkerWeb이나 SafeLine, 커뮤니티 차단 목록이 필요하다면 어떤 프록시든 그 옆에 CrowdSec을 두면
됩니다. Corvus는 웹 콘솔과 보호 기능을 컨테이너 하나로 쓰고 싶은 사람을 위한 것입니다.
[Corvus와 다른 리버스 프록시 비교](compare/index.md)를 참고하세요.

### Corvus가 fail2ban이나 CrowdSec을 대체하나요?

Corvus를 지나는 웹 트래픽에 대해서는 대부분 그렇습니다. 공격을 반복하는 IP를 Corvus가 자체
탐지 결과를 바탕으로 스스로 차단하고, 차단 시간을 점점 늘리며, 예외 네트워크를 지원하고, 로그를
파싱할 필요가 없습니다. CrowdSec에는 Corvus에 없는 것이 하나 있습니다. 많은 사용자의 신고로
만들어지는 공유 차단 목록입니다. 외부 차단 목록은 Corvus의 [IP 목록](security/ip-lists.md)에
불러올 수 있으며, SSH를 비롯해 웹이 아닌 서비스에는 여전히 fail2ban이나 CrowdSec이 알맞은
도구입니다.

### Corvus는 WAF인가요? ModSecurity나 OWASP Core Rule Set을 쓰나요?

Corvus에는 웹 애플리케이션 방화벽이 들어 있지만, ModSecurity나 OWASP Core Rule Set은 쓰지
않습니다. 엔진인 Corvus Sentinel이 요청 경로, 쿼리 문자열, 헤더, 본문을 흔한 공격(SQL 인젝션,
경로 탐색, 원격 코드 실행, 스캐너)에 대한 자체 시그니처로 검사하고, 최근 취약점에 대한 가상 패치
템플릿을 함께 제공합니다. 기본으로 켜 둘 수 있도록 오탐이 적게 나오게 조정되어 있습니다.
paranoia level까지 포함한 OWASP CRS 전체가 필요하다면 BunkerWeb이나 ModSecurity/Coraza 구성을
살펴보세요.

## 보안

### Corvus가 DDoS 공격을 막을 수 있나요?

애플리케이션 계층의 대량 요청은 일부 막을 수 있습니다. Rate Limit, 자동 차단, 브라우저 챌린지,
공격 대응 모드가 봇과 HTTP 요청 폭주를 막고, 접속 대기열은 실제 방문자가 몰릴 때 앱을 계속 쓸 수
있게 해 줍니다. 인터넷 회선 자체를 가득 채우는 대용량 공격은 어떤 셀프 호스팅 소프트웨어로도
막을 수 없습니다. 이 경우에는 호스팅 업체나 앞에 둔 CDN이 필요합니다.
[트래픽 폭주와 DDoS 대응](guides/ddos.md)을 참고하세요.

### Corvus는 CAPTCHA 없이 어떻게 봇을 막나요?

브라우저가 스스로 푸는 챌린지를 씁니다. 작은 JavaScript 작업 증명이나 쿠키 확인입니다. CAPTCHA
업체도 외부 서비스도 필요 없습니다. 통과한 방문자는 서명된 쿠키를 받고, 기본값으로 24시간 동안
다시 챌린지를 받지 않습니다. [챌린지와 공격 대응 모드](security/challenge.md)를 참고하세요.

### 국가를 차단할 수 있나요?

네. 보안 규칙은 방문자의 국가(국가 데이터베이스가 내장되어 있습니다)를 조건으로 요청을 차단하거나,
챌린지를 걸거나, 기록만 할 수 있습니다. [보안 규칙](security/rules.md)을 참고하세요.

### 관리 콘솔이 인터넷에 노출되나요?

콘솔은 사이트와 분리된 자체 포트(9443)에서 동작합니다. **관리 콘솔 접근 허용 네트워크**로 신뢰하는
네트워크에서만 접속하게 제한하거나, 사설 주소에만 공개하거나, VPN을 쓰세요. 2단계 인증, 통합
로그인(OIDC), 역할을 지원합니다. ufw와 firewalld는 Docker가 공개한 포트를 걸러 내지 않는다는 점에
주의하세요. [설치](getting-started/install.md)를 참고하세요.

## Corvus 운영

### Corvus를 실행하려면 무엇이 필요한가요?

Docker와 Compose 플러그인이 설치된 Linux 서버(amd64 또는 arm64), 그리고 80·443 포트가 필요합니다.
작은 VPS, NAS(TrueNAS SCALE, Unraid), 64비트 Raspberry Pi면 충분합니다. 벤치마크에서 Corvus는
메모리를 약 70~90 MB 사용했습니다. [설치](getting-started/install.md)를 참고하세요.

### Nginx Proxy Manager에서 어떻게 옮기나요?

콘솔의 **NPM 가져오기**를 사용하세요. NPM의 프록시 호스트, 리디렉션, 스트림, 인증서, Access List를
읽기만 하고 NPM은 바꾸지 않으므로 언제든 돌아갈 수 있습니다.
[NPM에서 이전](getting-started/migrate-npm.md)을 참고하세요.

### Docker로 돌리지 않는 서비스에도 전달할 수 있나요?

네. Corvus는 닿을 수 있는 주소라면 어디로든 전달합니다. 다른 컨테이너, 같은 서버의 서비스(Docker
호스트 주소를 통해), 네트워크 안의 다른 기기 모두 가능합니다.
[동작 방식](getting-started/how-it-works.md)을 참고하세요.

### Corvus는 Cloudflare나 터널 뒤에서도 동작하나요?

네. Corvus는 Cloudflare와 그 밖의 신뢰하는 프록시에서 실제 클라이언트 IP를 받아 오고, 앞에 TCP
터널이나 로드 밸런서가 있을 때는 PROXY protocol에서 받아 옵니다. 서버에 공인 IP가 없다면
터널(예: Cloudflare Tunnel, Pangolin, WireGuard를 쓰는 VPS)을 운영하고 그 끝을 Corvus로 향하게
하세요.

### Corvus는 와일드카드 인증서를 지원하나요?

네, DNS 인증으로 지원합니다. 저장해 두고 쓰는 DNS 인증 정보는 Cloudflare, DigitalOcean, DuckDNS,
Gandi, GoDaddy, Hetzner, Namecheap을 지원합니다. 직접 가진 인증서를 올릴 수도 있습니다.
[인증서](proxy/certificates.md)를 참고하세요.

## Corvus가 하지 않는 일

### Corvus는 Kubernetes ingress로 동작하거나 Docker 라벨을 읽나요?

아니요. Corvus는 웹 콘솔이나 HTTP API로 설정하며, Kubernetes 리소스나 Docker 라벨로 설정하지
않습니다. 컨테이너 라벨만으로 경로가 자동으로 생기기를 원한다면 Traefik이나 Docker 플러그인을
붙인 Caddy가 더 잘 맞습니다.

### Corvus는 HTTP/3를 지원하나요?

아직 지원하지 않습니다. Corvus는 TLS 위에서 HTTP/1.1과 HTTP/2를 제공합니다.

### Corvus로 앱 앞에 로그인(SSO)을 붙일 수 있나요?

Corvus는 호스트를 비밀번호(Basic auth), IP 허용 목록, 챌린지로 보호할 수 있습니다. 뒤에 있는 앱을
위한 forward-auth나 ID 공급자(IdP)는 들어 있지 않으며, SSO는 Corvus 콘솔에만 적용됩니다. 앱에
SSO가 필요하다면 Authelia, Authentik, 또는 앱 자체의 로그인을 앞에 두세요.
