---
description: "자주 겪는 Corvus 문제 해결: 콘솔이 안 열릴 때, 502·504·413 오류, Let's Encrypt 실패, 차단된 방문자, 스트림, 로그 위치."
---

# 문제 해결

## 콘솔이 열리지 않습니다

- **https**로 접속하세요: `https://<server>:9443`. 처음 인증서는 자체 서명이라 브라우저가 경고합니다.
- 컨테이너가 실행 중인가요? `docker compose ps`를 실행한 다음 `docker compose logs corvus`를 확인하세요.
- 방화벽이나 **관리 콘솔 접근 허용 네트워크** 때문에 막혔을 수 있습니다. 복구하려면
  `CORVUS_ADMIN_ALLOW_ANY=1`로 한 번 시작해 목록을 고치세요.
- 설정 토큰을 잃어버렸나요? `docker compose logs corvus | grep setup_token`(첫 관리자를 만들기
  전까지만 표시됩니다).

## 비밀번호를 잊었거나 2단계 인증을 잃어버렸습니다 { #i-forgot-the-password-or-lost-two-step-verification }

다른 관리자가 있다면 **사용자** 화면에서 비밀번호를 재설정하거나 **MFA 초기화**를 해 줄 수 있습니다.
관리자가 한 명뿐이라면 서버에서 다음을 실행하세요.

```bash
docker compose exec corvus corvus-control reset-admin
```

하나뿐인 활성 관리자에게 새 무작위 비밀번호를 만들어 출력하고, 그 계정의 세션을 끝내고, 그 계정이
만든 API 토큰을 삭제합니다. 인증 앱과 복구 코드도 잃어버렸다면 `--disable-mfa`를, 관리자가 여럿이면
`--email you@example.com`을 붙이세요. 로그인한 뒤 **계정 보안** 화면에서 비밀번호를 바꾸고 2단계 인증을
다시 설정하세요. 재설정은 감사 로그에 남습니다. Corvus가 실행 중일 때도 쓸 수 있고 SQLite와
PostgreSQL 모두 지원합니다.

## 502 Bad Gateway

Corvus가 전달 대상에 접속하지 못합니다.

- 컨테이너 이름이 대상이라면: Corvus와 같은 Docker 네트워크에 있어야 합니다.
- 대상이 `127.0.0.1` / `localhost`라면 **Docker 호스트**를 뜻합니다. 서비스가 `127.0.0.1`만이 아니라
  `0.0.0.0`(또는 Docker 브리지)에서 수신해야 하고, 호스트 방화벽이 Docker 네트워크를 허용해야
  합니다. `127.0.0.1`에만 묶인 서비스는
  [호스트 네트워크](../getting-started/install.md#run-it-with-host-networking-optional)가 필요합니다.
- `upstream sent too big header`: 앱이 보내는 쿠키나 헤더가 너무 큽니다. 호스트의
  **고급 nginx 설정**에 `proxy_buffer_size 32k;`를 추가하세요.

## 504 Gateway Timeout

서비스가 **업스트림 타임아웃**(기본 60초)보다 오래 걸렸습니다. 보고서나 내보내기처럼 느린
엔드포인트라면 *고급*에서 값을 늘리세요.

## 413 Request Entity Too Large

업로드가 **최대 업로드 크기**(기본 100 MB)보다 큽니다. *고급*에서 값을 늘리세요(`0`은 무제한).
[강화 설치](hardened.md)라면 `/tmp` 크기도 확인하세요.

## Let's Encrypt 발급에 실패합니다 { #lets-encrypt-fails }

- **HTTP-01**: 도메인이 이 서버를 가리키고, 인터넷에서 80 포트로 접속할 수 있어야 합니다(443만으로는
  안 됩니다).
- **와일드카드** 도메인은 DNS-01이 필요합니다.
- 테스트할 때는 **staging**을 쓰세요.

아래 오류라면 인증서 행에 한 줄 안내와 이 페이지로 가는 링크가 표시됩니다(전체 오류는 상태에 마우스를
올리면 보입니다). *인증서 발급/갱신 실패* 알림에도 같은 안내가 붙습니다.

### 발급 한도에 걸렸습니다 { #acme-rate-limited }

`urn:ietf:params:acme:error:rateLimited`. Let's Encrypt는 같은 이름 조합의 발급 횟수(주 5회)와
시간당 검증 실패 횟수를 제한합니다. 재시도를 멈추고 원인을 고친 다음, 오류에 표시된 시각 이후에 다시
시도하세요. 테스트는 한도가 훨씬 넉넉한 **staging**으로 하세요.

### 네임서버가 SERVFAIL 또는 REFUSED로 응답합니다 { #dns01-servfail }

`propagation: time limit exceeded … NS ns1.example.net. returned SERVFAIL for _acme-challenge…`.
도메인의 권한 네임서버가 TXT 조회에 오류로 응답합니다. 흔한 원인은 깨진 DNSSEC(등록 기관의 DS 레코드가
현재 영역과 맞지 않음), 영역을 관리하지 않는 공급자를 가리키는 등록 기관의 NS 레코드, 일부 네트워크의
조회를 거부하는 공급자입니다. `dig +dnssec _acme-challenge.example.com TXT @ns1.example.net`이나
온라인 DNSSEC 검사 도구로 확인하고 등록 기관에서 위임 설정을 고치세요.

### TXT 레코드가 제때 보이지 않습니다 { #dns01-propagation }

`propagation: time limit exceeded`, 또는 `unauthorized … Incorrect TXT record … _acme-challenge`.
레코드는 만들어졌지만 네임서버가 10분 안에 내보내지 않았습니다. 도메인의 NS 레코드가 (예전 공급자가
아니라) DNS 인증 정보의 공급자를 가리키는지 확인한 뒤 **지금 갱신**을 누르세요. 느린 공급자도 있어서
다시 시도하면 대개 됩니다.

### DNS 공급자가 자격 증명을 거부했습니다 { #dns01-credential }

`401`, `403`, `unauthorized`, `zone could not be found`가 포함된 `error presenting token: …`.
API 토큰이 틀렸거나 만료됐거나 권한이 없습니다. Cloudflare라면 해당 영역에 **Zone → DNS → Edit**
(와 **Zone → Read**) 권한이 있는 API 토큰을 쓰세요. **인증서 → DNS 인증 정보**에서 바꿀 수 있습니다.

### 네임서버를 찾지 못했습니다 { #dns01-lookup }

`could not find the start of authority`, `NS call failed`, `initial recursive nameserver`.
Corvus가 도메인의 네임서버를 조회하지 못했습니다. 도메인이 아직 등록·위임되지 않았거나, Corvus 서버가
DNS 리졸버에 접속하지 못합니다(`/etc/resolv.conf`와 외부 53번 포트를 확인하세요).

### CAA 레코드가 Let's Encrypt를 막습니다 { #acme-caa }

`urn:ietf:params:acme:error:caa`. 도메인(또는 상위 도메인)의 CAA 레코드가 다른 인증 기관만 허용합니다.
`0 issue "letsencrypt.org"`(와일드카드는 `issuewild`도)를 추가하거나 CAA 레코드를 지우세요.

### 허용되지 않는 이름입니다 { #acme-rejected-name }

`urn:ietf:params:acme:error:rejectedIdentifier`. Let's Encrypt는 공개 도메인 이름에만 발급합니다.
IP 주소, `localhost`, `nas.lan` 같은 내부 이름은 안 됩니다. 공개 도메인을 쓰거나 인증서를 직접
업로드하세요.

### 계정 이메일이 거부되었습니다 { #acme-email }

`urn:ietf:params:acme:error:invalidContact`. 이메일 도메인이 메일을 받지 않거나 `example.com` 같은
예시 주소입니다. 실제 주소를 쓰세요.

### 도메인에 DNS 레코드가 없습니다 { #acme-dns }

`urn:ietf:params:acme:error:dns`(예: *NXDOMAIN looking up A*). Let's Encrypt가 이름을 조회하지
못했습니다. 철자를 확인하고, A/AAAA 레코드가 내부망이 아닌 공개 DNS에 있는지 확인하세요.

### 80번 포트에 접속할 수 없습니다 { #acme-port-80 }

`urn:ietf:params:acme:error:connection`, *Timeout during connect (likely firewall problem)*.
방화벽에서 80번 포트를 열고 공유기에서 Corvus로 포워딩하세요. A/AAAA 레코드가 공인 주소를 가리키는지도
확인하세요(다른 곳을 가리키는 AAAA 레코드가 흔한 원인입니다). 80번 포트를 열 수 없다면 DNS-01을 쓰세요.

### HTTP-01 요청이 다른 서버에 도착했습니다 { #acme-wrong-server }

`urn:ietf:params:acme:error:unauthorized … Invalid response from http://…/.well-known/acme-challenge/…`.
요청은 도착했지만 Corvus가 아닌 곳이었습니다. 도메인이 다른 서버를 가리키거나, 다른 프록시가 80번
포트를 받거나, CDN(예: Cloudflare 프록시)이 경로를 리디렉션·캐시합니다. 도메인이 Corvus를 가리키게
하거나 DNS-01을 쓰세요.

### Let's Encrypt에 연결할 수 없습니다 { #acme-ca-unreachable }

`get directory at 'https://acme-v02.api.letsencrypt.org/directory'`. Corvus 서버가 외부 HTTPS에
접속하지 못하거나 이름을 조회하지 못합니다. DNS, 방화벽, 외부 프록시 설정을 확인하세요.

## 변경이 거부됩니다 { #config-rejected }

게이트웨이는 모든 변경을 적용하기 전에 검사하며, 거부된 변경은 실행 중인 설정에 영향을 주지 않습니다.
오류에 nginx 출력이 표시되고 **배포 이력**에도 남습니다.

### 포트가 이미 사용 중입니다 { #deploy-port-in-use }

*게이트웨이가 이전 설정을 유지했습니다*(`reload_rejected`). 검사는 통과했지만 nginx가 변경을 적용하지
못했습니다. 대개 스트림 포트(또는 80/443)를 서버의 다른 프로그램이나 호스트 네트워크를 쓰는 다른
컨테이너가 이미 쓰고 있기 때문입니다. 포트를 비우거나 다른 포트를 고르세요. 정확한 `bind()` 오류는
`docker compose logs corvus`에서 볼 수 있습니다.

### 고급 설정 오류 { #deploy-advanced-config }

`hosts/<id>.conf`에서 `[emerg] unknown directive …`, `… is not allowed here`,
`invalid number of arguments`. 호스트의 **고급 nginx 설정**에 nginx가 받아들이지 않는 내용이
있습니다. 파일 이름의 숫자가 호스트 ID이니, 그 호스트의 설정을 고치거나 지우세요.

## 외부 연결에 실패합니다 { #outbound-errors }

목록 피드, 국가 데이터베이스, 관리형 규칙 업데이트, 알림 채널, Cloudflare 대역, SSO 디스커버리, NPM
가져오기는 Corvus 서버에서 외부로 접속합니다. 이 오류들에는 안내가 함께 표시됩니다.

| 오류에 포함된 내용 | 의미와 해결 방법 |
| --- | --- |
| `no such host`, `server misbehaving` | 이름을 조회할 수 없습니다. URL과 서버의 DNS를 확인하세요. |
| `connection refused` | 그 주소에서 수신하는 서비스가 없습니다. 호스트, 포트, 서비스 실행 여부를 확인하세요. |
| `i/o timeout`, `deadline exceeded` | 응답이 없습니다. 외부 방화벽이나 프록시를 확인하세요. |
| `x509:` | 인증서를 신뢰할 수 없습니다. URL을 확인하고 공인 인증 기관의 인증서를 쓰세요. |
| `is not allowed`, `non-public address` | Corvus는 루프백, 링크 로컬, (목록 피드는) 사설 주소로 접속하지 않습니다. |
| `HTTP 401`, `HTTP 403` | 인증 정보가 거부되었습니다. 토큰, 웹훅 URL, 비밀번호를 확인하세요. |
| `HTTP 404`, `404 Not Found` | URL이 틀렸습니다. SSO는 `/.well-known/openid-configuration`을 뺀 발급자 URL을 입력하세요. |
| `Error 1045` | NPM 가져오기: MySQL 사용자 이름이나 비밀번호가 틀렸습니다. |
| `Error 1049` | NPM 가져오기: MySQL 데이터베이스가 없습니다(NPM 기본값은 `npm`). |
| `did not match the issuer URL` | SSO: 발급자 URL이 공급자의 `issuer`와 다릅니다(대개 끝의 슬래시). 그대로 복사하세요. |

## 방문자가 차단되었다고 합니다

차단 페이지에는 방문자의 IP와 시각이 표시됩니다. 그 시각 전후로 **트래픽 관제 → 요청 로그**에서
IP를 검색하면, 어떤 규칙이나 차단에 막혔는지 행에 표시됩니다. **IP 차단**에서 해제하거나, **허용**
규칙 또는 예외 네트워크를 추가하세요.

## API 클라이언트가 "cookie-check"와 함께 429를 받습니다

호스트에 [접속 대기열](../access-queue.md)이 있는데 클라이언트가 입장권 쿠키를 보내지 않았습니다.
오류가 아니라 대기열이 기다리라고 요청하는 것입니다. 브라우저 경로만 보호하거나, API를 대기열이 없는
호스트로 옮기세요.

## 스트림이 응답하지 않습니다

브리지 네트워크에서는 `docker-compose.yml`에서도 포트를 공개해야 합니다.
[스트림](../proxy/streams.md#publish-the-port)을 참고하세요.

## 로그에 `sendfile() failed (9: Bad file descriptor)`가 보입니다

무시해도 됩니다. 백엔드가 업로드를 다 읽기 전에 응답한 것이며, 요청 자체는 정상적으로 끝났습니다.

## 로그는 어디에 있나요?

```bash
docker compose logs -f corvus
```

콘솔의 **배포 이력** 페이지에서 거부된 설정의 출력을 볼 수 있습니다.

그래도 해결되지 않나요? Corvus 버전과 관련 로그 줄을 담아
[이슈를 등록하세요](https://github.com/corvus-gateway/corvus/issues)(공유하고 싶지 않은 도메인과 IP는
지우세요).
