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

## Let's Encrypt 발급에 실패합니다

- **HTTP-01**: 도메인이 이 서버를 가리키고, 인터넷에서 80 포트로 접속할 수 있어야 합니다(443만으로는
  안 됩니다).
- **와일드카드** 도메인은 DNS-01이 필요합니다.
- 시도가 너무 많았나요? Let's Encrypt에는 발급 횟수 제한이 있습니다. 테스트할 때는 **staging**을 쓰세요.
- 인증서 행에 마지막 오류가 표시됩니다.

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
