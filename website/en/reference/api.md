# HTTP API

The admin console is a client of a JSON API, so everything you can do in the console can be
automated. The API is served on the admin port under `/api/`.

1. Create a token on the **API tokens** page (choose the smallest role that works).
2. Send it as a Bearer token:

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

Every change goes through the same validation, test and versioned deployment as in the console.

**Full reference:** [docs/API.md](https://github.com/corvus-gateway/corvus/blob/main/docs/API.md),
with every endpoint, field, error code and the Prometheus metrics.
