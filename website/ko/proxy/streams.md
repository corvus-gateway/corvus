---
description: "스트림으로 HTTP가 아닌 서비스(데이터베이스, SSH, MQTT, VPN, 게임 서버)의 TCP·UDP 포트를 전달합니다."
---

# 스트림 (TCP/UDP)

스트림은 HTTP를 쓰지 않는 서비스(데이터베이스, SSH, MQTT, VPN, 게임 서버)를 위해 TCP 또는 UDP
포트를 다른 호스트로 전달합니다.

![스트림](../img/streams.png){ .shot }

**스트림 추가**를 누르고 다음을 설정합니다.

![스트림 입력 화면](../img/stream-form.png){ .shot .panel }

| 필드 | 의미 |
|---|---|
| **수신 포트** | Corvus가 수신할 포트 |
| **프로토콜** | TCP, UDP 또는 둘 다 |
| **전달 대상** | 서비스의 호스트 / IP와 포트(프록시 호스트와 같은 [전달 대상](proxy-hosts.md#forward-targets)) |
| **인증서** | 선택 사항, TCP 전용: Corvus가 TLS를 종료하고 평문 TCP로 전달합니다. 인증서가 발급되면 TCP 포트가 열립니다 |
| **메모** | 선택 사항, 사람이 보는 용도([프록시 호스트와 같음](proxy-hosts.md#notes)) |
| **PROXY protocol** | 선택 사항, TCP 전용: 터널에서 받거나 서비스로 보냅니다([아래](#proxy-protocol)) |

## 포트 공개 { #publish-the-port }

기본 브리지 네트워크에서는 Docker가 `docker-compose.yml`에 적힌 포트만 전달합니다. 스트림 포트를
모두 여기에 추가하고 `docker compose up -d`로 적용하세요.

```yaml hl_lines="6 7"
    ports:
      - '80:80'
      - '443:443'
      - '9443:9443'
      - '5432:5432'       # stream: PostgreSQL (TCP)
      - '1194:1194/udp'   # stream: OpenVPN (UDP)
```

`network_mode: host`라면 따로 할 일이 없습니다.

## PROXY protocol { #proxy-protocol }

스트림은 연결 내용을 읽지 않고 그대로 넘기므로 서비스에는 Corvus의 주소가 보이고, TCP 터널 뒤에서는
Corvus에도 터널의 주소만 보입니다. [PROXY protocol](https://www.haproxy.org/download/2.9/doc/proxy-protocol.txt)은
연결 맨 앞의 헤더로 방문자 주소를 전달합니다. 스트림마다 이를 위한 스위치가 두 개 있습니다(TCP 전용:
UDP만 쓰는 스트림에서는 보이지 않고, **TCP + UDP**에서는 TCP 수신에만 적용되며 UDP는 그대로입니다).

| 스위치 | 하는 일 |
|---|---|
| **PROXY protocol 받기** | 이 포트의 모든 연결은 터널이나 로드 밸런서(FRP, HAProxy, 클라우드 로드 밸런서)가 보낸 PROXY protocol 헤더(v1 또는 v2)로 시작해야 합니다. **신뢰할 PROXY protocol 출처**에서 온 연결이면 헤더의 방문자 주소를 사용합니다 |
| **백엔드에 PROXY protocol 보내기** | Corvus가 방문자 주소를 PROXY protocol(v1) 헤더로 서비스에 보냅니다. 서비스가 이 헤더를 기대하도록 설정되어 있어야 하며, 아니면 연결이 실패합니다 |

**받기**를 켜려면 **네트워크** 페이지의 **신뢰할 PROXY protocol 출처**에 주소가 하나 이상 있어야
합니다([HTTP/HTTPS PROXY protocol 포트](proxy-hosts.md#behind-a-tcp-tunnel-or-load-balancer-proxy-protocol)와
같은 목록이며, 어떤 주소를 넣을지는 그곳을 참고하세요). 네트워크 페이지의 **PROXY protocol 받기**
스위치는 필요 없습니다. 그 스위치는 HTTP/HTTPS 포트만 추가합니다. PROXY protocol을 받는 스트림이
있는 동안에는 신뢰 목록을 비울 수 없습니다. 다른 주소에서 온 연결도 헤더를 보내야 하지만 자기 주소를
그대로 씁니다. PROXY protocol을 받는 포트는 헤더 없는 연결을 거부하므로, 직접 접속하는 클라이언트용으로는
스트림(포트)을 따로 두세요.

**받기**만 켜면 헤더를 떼고 연결을 넘기므로 서비스에는 여전히 Corvus의 주소가 보입니다. 방문자 주소를
서비스까지 전달하려면 PROXY protocol을 이해하는 서비스에 대해 **보내기**도 켜세요. Postfix
(`postscreen_upstream_proxy_protocol = haproxy` 또는 `smtpd_upstream_proxy_protocol = haproxy`),
Dovecot(리스너에 `haproxy = yes`와 `haproxy_trusted_networks`), HAProxy(`accept-proxy`),
nginx(`listen … proxy_protocol`) 등이 지원합니다. 그쪽에서는 Corvus의 주소만 신뢰하세요.

스트림에 TLS 인증서가 있으면 헤더는 TLS 핸드셰이크 앞에 오고(터널이 보내는 방식), 서비스에는 복호화된
데이터보다 먼저 보내집니다.

=== "FRP"

    Corvus 옆의 `frpc.toml`(frp 0.52 이상). 메일 서버로 전달하는 2525번 포트 스트림에 **받기**와
    **보내기**를 켠 경우입니다.

    ```toml
    [[proxies]]
    name = "corvus-smtp"
    type = "tcp"
    localIP = "127.0.0.1"      # Corvus 호스트(또는 Corvus 컨테이너 이름)
    localPort = 2525           # 스트림의 수신 포트
    remotePort = 25
    transport.proxyProtocolVersion = "v2"
    ```

=== "HAProxy"

    ```
    frontend smtp_in
        mode tcp
        bind :25
        default_backend corvus_smtp

    backend corvus_smtp
        mode tcp
        server corvus 192.168.1.10:2525 send-proxy-v2
    ```

HTTP/HTTPS 포트와 마찬가지로, 터널이 같은 호스트에 있으면 스트림 포트를 `127.0.0.1`에만 공개하고,
아니면 방화벽으로 터널 주소만 허용하세요.

## 제한 사항

- Corvus가 직접 쓰는 포트(80, 443, 9443, 내부 포트, 그리고 켜져 있을 때의 [PROXY protocol
  포트](proxy-hosts.md#behind-a-tcp-tunnel-or-load-balancer-proxy-protocol))는 사용할 수 없고, 스트림을 Corvus 자신의 관리
  콘솔이나 내부 포트로 전달할 수도 없습니다.
- 사용 중인 두 스트림이 같은 포트와 프로토콜을 함께 쓸 수 없습니다.
- 트래픽이 없는 연결은 TCP는 1시간, UDP는 60초 뒤에 닫힙니다.
- 스트림에는 요청 로그가 없고, IP 차단과 보안 규칙도 아직 적용되지 않습니다. 필요하면 방화벽으로
  접근을 제한하세요.

스트림도 [NPM에서 가져옵니다](../getting-started/migrate-npm.md).
