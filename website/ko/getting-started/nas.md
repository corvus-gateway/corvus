---
description: NAS에 Corvus 설치하기 — TrueNAS SCALE 앱 카탈로그, 포트, 설정 토큰, 다른 NAS에서 주의할 점.
---

# NAS에 설치

## TrueNAS SCALE

Corvus는 TrueNAS 앱 카탈로그의 **Community** 트레인에 있습니다.

1. **Apps → Discover Apps**에서 **Corvus**를 검색해 **Install**을 누릅니다.
2. 포트는 기본값을 쓰거나 바꿉니다.

    | 항목 | 기본값 | 용도 |
    |---|---|---|
    | Admin Console Port | **30553** | 관리 콘솔 (처음에는 자체 서명 인증서의 HTTPS) |
    | HTTP Port | **30554** | 사이트의 HTTP 접속과 Let's Encrypt HTTP 인증 |
    | HTTPS Port | **30555** | 사이트의 HTTPS 접속 |

    TrueNAS 웹 화면이 이미 80과 443을 쓰기 때문에 앱은 다른 포트를 씁니다.

3. **Data Storage**에서 데이터 위치를 고릅니다. 데이터베이스, 인증서, 암호화 키가 들어 있으니
   다른 앱 데이터처럼 이 데이터셋을 백업하세요.
4. 앱이 시작되면 Apps 화면에서 앱의 **로그**를 열고 `setup_token`이 있는 줄을 찾습니다. 토큰을 직접
   정하려면 설치 전에 **Additional Environment Variables**에 `CORVUS_SETUP_TOKEN`(12자 이상)을 넣으세요.
5. `https://<TrueNAS-IP>:30553`을 열고 인증서 경고를 넘긴 뒤 토큰을 넣고 관리자 계정을 만듭니다.
   이어서 [첫 프록시 호스트](first-host.md)를 만들어 보세요.

### 인터넷에서 사이트에 접속하려면

공유기에서 **80 → 30554**, **443 → 30555**로 TrueNAS 주소에 포트 포워딩하세요. 그러면 방문자와
Let's Encrypt가 평소 포트로 Corvus에 닿습니다. 80 포트를 열 수 없다면 인증서는
[DNS 인증](../proxy/certificates.md)으로 받으세요.

콘솔 포트(30553)는 인터넷에 열지 마세요. [콘솔 보호](../operations/users.md#protect-the-console)를 참고하세요.

### 업데이트

카탈로그에 새 Corvus 버전이 올라오면 TrueNAS가 앱 업데이트를 표시합니다. 데이터는 시작할 때 자동으로
옮겨집니다.

## 다른 NAS

Docker Compose를 쓸 수 있는 NAS(Synology Container Manager, Unraid, QNAP Container Station 등)라면
일반 [설치](install.md) 방법을 그대로 쓰면 됩니다. 주의할 점:

- **NAS가 이미 80/443을 쓰는 경우.** Corvus를 다른 포트(예: `8080:80`, `8443:443`)로 열고 공유기에서
  80/443을 그 포트로 포워딩하거나, NAS 화면의 포트를 바꾸세요.
- **폴더 권한.** `PUID`/`PGID`를 데이터 폴더 주인인 NAS 사용자로 맞추세요([환경 변수](../reference/configuration.md) 참고).
- **설정 토큰**은 컨테이너 로그에 있습니다. `CORVUS_SETUP_TOKEN`으로 직접 정할 수도 있습니다.

여기 없는 NAS에서 쓰고 계신다면 [이슈 #94](https://github.com/corvus-gateway/corvus/issues/94)에 경험을 남겨 주세요.
