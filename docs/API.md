# Corvus HTTP API

The admin console is a client of this JSON API, so everything you can do in the console can be
automated. The API is served on the admin port (default `https://<server>:9443`) together with the
console, under `/api/`, and is subject to the same *Admin console allowed networks* setting.

## Authentication

**API tokens** (recommended for automation): create one in *Settings → API tokens* and send it as

```
Authorization: Bearer cvs_…
```

- A token has a role (`viewer`, `operator` or `admin`) and never more than its creator's current
  role. Disabling or deleting the creator invalidates the token.
- Tokens cannot call session-only endpoints: sign-in, password and MFA (`/api/auth/*`, except
  `GET /api/auth/me`), user management (`/api/users`) and token management (`/api/tokens`).
- Only a hash of the token is stored; the value is shown once when it is created.

**Browser sessions** (used by the console): `POST /api/auth/login` sets a session cookie.
State-changing requests with a session cookie must send `X-Corvus-Request: 1` (CSRF protection);
Bearer requests don't need it.

## Requests and responses

- Request and response bodies are JSON (`Content-Type: application/json`), at most 1 MiB.
  Unknown fields are rejected.
- Errors share one shape:

  ```json
  { "error": "validation", "message": "…", "fields": [{ "field": "forward_port", "code": "range", "message": "…" }] }
  ```

  | Status | `error` | Meaning |
  |---|---|---|
  | 400 | `bad_request` | Malformed JSON or parameters |
  | 401 | `unauthorized` | Missing or invalid session/token |
  | 403 | `forbidden`, `csrf`, `session_required` | Role too low, missing `X-Corvus-Request`, or a token used on a session-only endpoint |
  | 404 | `not_found` | Unknown resource or endpoint |
  | 409 | `conflict` | Conflicts with existing data (e.g. a domain already in use) |
  | 429 | `rate_limited` | Too many attempts (sign-in, MFA) |
  | 422 | `validation` | Invalid fields (see `fields`) |
  | 422 | `config_rejected` | The gateway rejected the generated configuration; nothing was changed (`output` contains the message) |

- Every configuration change is validated, written as a new configuration version, tested with
  `openresty -t` and applied atomically; on failure the previous version stays live.

## Example

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

## Endpoints

Role: **public** = no authentication · **viewer** = any signed-in user or token ·
**operator** = may change configuration · **admin** = users and system settings ·
**session** = admin with a browser session (no tokens).

### System and authentication

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/api/status` | public | `{ "setup_required": bool, "version": "…" }` |
| POST | `/api/setup` | public | Create the first administrator (requires the setup token from the logs) |
| POST | `/api/auth/login` | public | `{ "email", "password" }` → session (or MFA challenge) |
| POST | `/api/auth/login/mfa` | public | Complete sign-in with a TOTP or recovery code |
| GET | `/api/auth/sso` | public | Whether OIDC single sign-on is enabled |
| GET | `/api/auth/oidc/start`, `/api/auth/oidc/callback` | public | OIDC sign-in flow |
| GET | `/api/auth/me` | viewer | Current user (or token principal) |
| POST | `/api/auth/logout`, `/api/auth/password` | viewer (session) | Sign out, change password |
| POST | `/api/auth/mfa/setup`, `/enable`, `/disable` | viewer (session) | Manage TOTP for the current user |
| GET | `/api/overview` | viewer | Dashboard summary |
| GET | `/api/audit` | viewer | Audit log |

### Proxy hosts, certificates, deployments

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/api/proxy-hosts` | viewer | List proxy hosts |
| GET | `/api/proxy-hosts/{id}` | viewer | Get one proxy host |
| POST | `/api/proxy-hosts` | operator | Create (fields below) |
| PUT | `/api/proxy-hosts/{id}` | operator | Replace |
| DELETE | `/api/proxy-hosts/{id}` | operator | Delete |
| GET | `/api/certificates` | viewer | List certificates |
| POST | `/api/certificates` | operator | Upload a custom certificate |
| POST | `/api/certificates/acme` | operator | Order a Let's Encrypt certificate (HTTP-01 or DNS-01) |
| POST | `/api/certificates/{id}/renew` | operator | Renew now |
| DELETE | `/api/certificates/{id}` | operator | Delete |
| GET | `/api/acme/dns-providers` | viewer | Supported DNS-01 providers |
| GET | `/api/deployments` | viewer | Configuration versions |
| GET | `/api/deployments/{id}/files` | viewer | Generated files of a version |
| POST | `/api/deployments` | operator | Redeploy the current configuration |
| POST | `/api/deployments/{id}/rollback` | operator | Roll back to a version |

Proxy host fields: `domains` (array), `forward_scheme` (`http`/`https`), `forward_host`,
`forward_port`, `websocket`, `ssl_forced`, `http2`, `certificate_id` or `certificate_request`,
`client_ip` (`direct`, `cloudflare`, `trusted_proxies`), `waitroom_id`, `waitroom_path`,
`rate_limit_id`, `access_list_id` or `basic_auth`, `block_exploits`, `body_inspection`
(`off`, `log`, `block`), `body_max_kb`, `advanced_config`, `enabled`.

`certificate_request` orders a Let's Encrypt certificate for the host's domains: `email`, `challenge`
(`http-01` or `dns-01`), `dns_provider`, `dns_credentials` (e.g. `{"api_token": "…"}` for Cloudflare), `staging`.

### Protection

| Method | Path | Role | Description |
|---|---|---|---|
| GET / POST | `/api/security-rules` | viewer / operator | List / create security rules |
| PUT / DELETE | `/api/security-rules/{id}` | operator | Update / delete |
| PUT | `/api/security-rules/order` | operator | Reorder rules |
| GET | `/api/security-rules/events` | viewer | Recent rule matches |
| GET | `/api/rule-templates` | viewer | Virtual-patch and rule templates |
| POST | `/api/security-rules/from-template` | operator | Create rules from a template |
| GET | `/api/security-rules/geoip` | viewer | GeoIP database status |
| POST | `/api/security-rules/geoip/refresh` | admin | Update the GeoIP database |
| GET | `/api/managed-rules` | viewer | Active managed rule bundle |
| PUT | `/api/settings/managed-rules` | admin | Managed-rule update settings |
| POST | `/api/managed-rules/check`, `/upload` | admin | Check for / upload a signed bundle |
| GET / POST | `/api/ip-lists` | viewer / operator | IP lists |
| PUT / DELETE | `/api/ip-lists/{id}` | operator | Update / delete |
| GET / POST | `/api/bans` | viewer / operator | IP bans |
| DELETE | `/api/bans/{id}` | operator | Remove a ban |
| POST | `/api/bans/{id}/permanent` | operator | Make a ban permanent |
| GET / PUT | `/api/settings/autoban` | viewer / operator | Automatic ban settings (threshold, first and longest ban, permanent ban from offense N) |
| GET / POST | `/api/rate-limits` | viewer / operator | Rate limits |
| PUT / DELETE | `/api/rate-limits/{id}` | operator | Update / delete |
| GET / POST | `/api/access-lists` | viewer / operator | Password protection lists |
| PUT / DELETE | `/api/access-lists/{id}` | operator | Update / delete |

### Access queue

| Method | Path | Role | Description |
|---|---|---|---|
| GET / POST | `/api/waitrooms` | viewer / operator | Access queues |
| PUT / DELETE | `/api/waitrooms/{id}` | operator | Update / delete |
| GET | `/api/waitrooms/{id}/live` | viewer | Live active and waiting counts |
| GET | `/preview/wait/{room}` | viewer | Preview the waiting page |

### Traffic and alerts

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/api/traffic/summary`, `/timeseries`, `/hosts`, `/top-paths`, `/events` | viewer | Traffic statistics |
| GET | `/api/traffic/logs` | viewer | Request log search (`q`, `host`, `status`, `limit`, `format`) |
| GET | `/api/traffic/archives` | operator | Rotated request log files |
| GET | `/api/traffic/archives/{name}` | operator | Download a log file |
| GET / PUT | `/api/settings/retention` | admin | Retention and log rotation |
| GET / POST | `/api/alerts/channels` | viewer / operator | Alert channels (Slack, Mattermost, Discord, Teams, Telegram, webhook) |
| PUT / DELETE | `/api/alerts/channels/{id}` | operator | Update / delete |
| POST | `/api/alerts/channels/{id}/test` | operator | Send a test alert |
| GET / PUT | `/api/alerts/rules` | viewer / operator | Alert rules |
| GET | `/api/alerts/history`, `/api/alerts/active` | viewer | Alert history / currently firing |

### Settings, users, tokens, import

| Method | Path | Role | Description |
|---|---|---|---|
| GET / PUT | `/api/settings/network` | viewer / admin | Trusted proxies, real IP header, admin allowlist |
| POST | `/api/settings/network/cloudflare/refresh` | admin | Refresh Cloudflare IP ranges |
| GET / PUT | `/api/settings/sso` | admin | OIDC settings |
| POST | `/api/settings/sso/test` | admin | Test OIDC discovery |
| GET / POST | `/api/users` | admin (session) | Users |
| PUT / DELETE | `/api/users/{id}` | admin (session) | Update / delete |
| POST | `/api/users/{id}/reset-mfa` | admin (session) | Reset a user's MFA |
| GET / POST | `/api/tokens` | session | API tokens (`{ "name", "role", "expires_days" }`) |
| DELETE | `/api/tokens/{id}` | session | Revoke a token |
| POST | `/api/import/npm/preview`, `/apply` | admin | Import from Nginx Proxy Manager |

## Prometheus metrics

`GET /metrics` (any valid API token, `Authorization: Bearer cvs_…`) returns the Prometheus text
format:

| Metric | Description |
|---|---|
| `corvus_build_info` | Version information |
| `corvus_config_version` | Active configuration version |
| `corvus_gateway_up`, `corvus_redis_up` | Component health |
| `corvus_requests_total` | Requests by host and status class |
| `corvus_blocked_total` | Blocked requests by kind |
| `corvus_request_duration_seconds` | Request latency |
| `corvus_security_rule_hits_*` | Security rule matches |
| `corvus_access_queue_waiting`, `corvus_access_queue_active` | Access queue counts |
| `corvus_certificate_expiry_seconds` | Time until certificate expiry |

Example scrape configuration:

```yaml
scrape_configs:
  - job_name: corvus
    scheme: https
    authorization: { credentials: cvs_... }
    static_configs: [{ targets: ["corvus.example.com:9443"] }]
```

## Stability

Within 1.x, endpoints and fields are only added, not removed or renamed; a breaking change goes
into a new major version and is listed in the release notes.
