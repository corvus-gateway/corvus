---
description: "JSON HTTP API로 Corvus를 자동화합니다. API 토큰을 만들고 관리 콘솔이 쓰는 것과 같은 엔드포인트를 호출합니다."
---

# HTTP API

관리 콘솔은 JSON API의 클라이언트이므로, 콘솔에서 할 수 있는 모든 작업을 자동화할 수 있습니다.
API는 관리 포트의 `/api/` 아래에서 제공됩니다.

1. **API 토큰** 페이지에서 토큰을 만듭니다(필요한 만큼의 가장 낮은 역할을 고르세요).
2. Bearer 토큰으로 보냅니다.

```bash
TOKEN=cvs_...
API=https://corvus.example.com:9443/api

# list proxy hosts
curl -s -H "Authorization: Bearer $TOKEN" $API/proxy-hosts

# create a proxy host with a Let's Encrypt certificate
curl -s -H "Authorization: Bearer $TOKEN" -H "Content-Type: application/json" \
  -d '{"domains":["app.example.com"],"forward_scheme":"http","forward_host":"app","forward_port":8080,
       "ssl_forced":true,"certificate_request":{"email":"admin@example.com"}}' \
  $API/proxy-hosts
```

모든 변경은 콘솔과 똑같이 검증, 검사, 버전 관리 배포를 거칩니다.

**전체 레퍼런스:** [docs/API.md](https://github.com/corvus-gateway/corvus/blob/main/docs/API.md)에
모든 엔드포인트, 필드, 오류 코드와 Prometheus 메트릭이 정리되어 있습니다.
