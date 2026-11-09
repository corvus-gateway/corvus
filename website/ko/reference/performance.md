---
description: "2 vCPU에서 측정한 Corvus 벤치마크: 기본 프록시, 보안 규칙, HTTPS, 접속 대기열, 챌린지의 초당 요청 수, 지연 시간, 메모리."
---

# 성능

GitHub 호스팅 `ubuntu-24.04` 러너(4 vCPU AMD EPYC 9V45)에서
[벤치마크 워크플로](https://github.com/corvus-gateway/corvus/blob/main/docs/benchmark.md)로 측정했습니다.
Corvus는 **2 vCPU**에 고정하고, 부하 생성기와 백엔드는 나머지 두 개에서 실행했으며, keep-alive 연결
64개로 3바이트 응답을 주는 `GET /`를 요청했습니다.

| 시나리오 | req/s | p50 | p99 | 메모리 |
|---|---:|---:|---:|---:|
| nginx 1.28 리버스 프록시(기준) | 97,227 | 0.63 ms | 1.16 ms | 17 MB |
| Corvus, 기본 호스트 | 75,473 | 0.79 ms | 1.83 ms | 72 MB |
| Corvus, 규칙 30개 + CIDR 1,000개 목록 + 공격 차단 + 헤더 + Rate Limit | 54,585 | 1.12 ms | 2.29 ms | 77 MB |
| Corvus, HTTPS (ECDSA P-256), keep-alive | 59,697 | 1.00 ms | 2.18 ms | 80 MB |
| Corvus, HTTPS, 요청마다 새 TLS 핸드셰이크 | 4,348 handshakes/s | 5.88 ms | 11.62 ms | 88 MB |
| Corvus, 접속 대기열(입장한 방문자) | 57,954 | 1.05 ms | 2.22 ms | 84 MB |
| Corvus, 챌린지 규칙(통과 쿠키가 있는 방문자) | 62,901 | 0.96 ms | 1.94 ms | 88 MB |

Corvus는 순수 nginx 처리량의 약 4분의 3을 내며, 보호 기능을 모두 켜면 여기서 약 28 %가 더
줄어듭니다. 러너는 공유 환경이고 응답이 매우 작으므로, 이 수치는 시나리오와 버전을 비교하기 위한
것이지 처리 용량을 보장하는 값이 아닙니다. 메모리에는 컨트롤 플레인과 내장 접속 대기열 저장소가
포함됩니다.
