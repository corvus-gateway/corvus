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
  `GET /api/auth/me`), user management (`/api/users`), token management (`/api/tokens`) and backup/restore
  (`/api/backup`).
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
  | 413 | `too_large` | Upload larger than allowed (backup restore: 200 MB) |
  | 422 | `invalid_backup` | Uploaded backup failed its checks (`message` says why) |
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

### Proxy hosts, redirection hosts, streams, certificates, deployments

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/api/proxy-hosts` | viewer | List proxy hosts |
| GET | `/api/proxy-hosts/{id}` | viewer | Get one proxy host |
| POST | `/api/proxy-hosts` | operator | Create (fields below) |
| PUT | `/api/proxy-hosts/{id}` | operator | Replace |
| DELETE | `/api/proxy-hosts/{id}` | operator | Delete |
| GET | `/api/under-attack` | viewer | Hosts with under attack mode `on` or `auto` and whether it is active now |
| POST | `/api/proxy-hosts/{id}/under-attack/deactivate` | operator | End an automatic activation now (`409` when none is running) |
| GET | `/api/redirection-hosts` | viewer | List redirection hosts |
| GET | `/api/redirection-hosts/{id}` | viewer | Get one redirection host |
| POST | `/api/redirection-hosts` | operator | Create (fields below) |
| PUT | `/api/redirection-hosts/{id}` | operator | Replace |
| DELETE | `/api/redirection-hosts/{id}` | operator | Delete |
| GET | `/api/streams` | viewer | List streams (TCP/UDP port forwarding) |
| GET | `/api/streams/{id}` | viewer | Get one stream |
| POST | `/api/streams` | operator | Create (fields below) |
| PUT | `/api/streams/{id}` | operator | Replace |
| DELETE | `/api/streams/{id}` | operator | Delete |
| GET | `/api/certificates` | viewer | List certificates |
| POST | `/api/certificates` | operator | Upload a custom certificate |
| PUT | `/api/certificates/{id}` | operator | Replace an uploaded certificate in place (`certificate`, `private_key`, optional `name`): validated like an upload, keeps its ID and host assignments, redeploys; ACME certificates → 422 |
| POST | `/api/certificates/acme` | operator | Order a Let's Encrypt certificate (HTTP-01 or DNS-01) |
| POST | `/api/certificates/{id}/renew` | operator | Renew now |
| DELETE | `/api/certificates/{id}` | operator | Delete |
| GET | `/api/acme/dns-providers` | viewer | Supported DNS-01 providers |
| GET | `/api/dns-credentials` | viewer | Saved DNS-01 credentials: `id`, `name`, `provider`, `used_by` (certificates), timestamps; the secret is never returned |
| POST | `/api/dns-credentials` | operator | Save one: `provider`, `credentials` (certbot ini text), optional `name` (default: provider name, "Cloudflare 2", …) |
| PUT | `/api/dns-credentials/{id}` | operator | Rename (`name`) and/or replace the secret (`credentials`, optional `provider`); certificates use a new secret from their next renewal |
| DELETE | `/api/dns-credentials/{id}` | operator | Delete; `422` (`in_use`) while certificates use it |
| GET | `/api/deployments` | viewer | Configuration versions |
| GET | `/api/deployments/{id}/files` | viewer | Generated files of a version |
| POST | `/api/deployments` | operator | Redeploy the current configuration |
| POST | `/api/deployments/{id}/rollback` | operator | Roll back to a version |

Proxy host fields: `domains` (array), `forward_scheme` (`http`/`https`), `forward_host`,
`forward_port`, `websocket`, `ssl_forced`, `http2`, `certificate_id` or `certificate_request`,
`client_ip` (`direct`, `cloudflare`, `trusted_proxies`), `waitroom_id`, `waitroom_path`,
`rate_limit_id`, `access_list_id` or `basic_auth`, `block_exploits`, `body_inspection`
(`off`, `log`, `block`), `body_max_kb`, `security_headers`, `under_attack`, `under_attack_config`,
`max_upload_mb`, `upstream_timeout_s`, `gzip`, `advanced_config`, `locations`, `notes`, `enabled`.
`under_attack`, `under_attack_config`, `locations` and `notes` keep their stored value when omitted
from a `PUT` (`"locations": []` removes all locations).

`notes` (proxy hosts, redirection hosts and streams) is free text for people, at most 1000
characters; it is not used by the gateway configuration.

| Field | Default | Meaning |
|---|---|---|
| `max_upload_mb` | `null` | Largest request body in MB; `null` = global 100 MB, `0` = unlimited (0–1048576). Larger requests get 413 |
| `upstream_timeout_s` | `null` | Upstream read and send timeout in seconds (1–86400); `null` = 60 s (1 h for WebSocket hosts) |
| `gzip` | `true` | Compress text responses (HTML, CSS, JS, JSON, XML, SVG; ≥ 1 KB). Responses the upstream already compressed pass unchanged |

An advanced configuration that sets `client_max_body_size` or `gzip` itself keeps its own value.

`locations` (custom locations, at most 50) route a path prefix to its own upstream:

```json
"locations": [
  {"path": "/api", "forward_scheme": "http", "forward_host": "api", "forward_port": 8080,
   "forward_path": "/v2/", "advanced_config": ""}
]
```

| Field | Meaning |
|---|---|
| `path` | Path prefix (`/` followed by letters, digits, `._~/-`); `/__corvus` and `/.well-known/acme-challenge` are reserved. `/` replaces the host's forward target |
| `forward_scheme`, `forward_host`, `forward_port` | Upstream of the location (default scheme `http`) |
| `forward_path` | Optional; replaces the matched prefix (`/api/x` → `/v2/x` with `/v2/`). Not allowed for `/` |
| `advanced_config` | Optional nginx directives inside the location (same restrictions as the host's `advanced_config`, no nested `location`); administrators only |

A location keeps the host's protections: security rules, bans, exploit and body inspection,
"under attack" mode, rate limit, Basic auth, and the access queue for requests under the
protected path, as well as the host's upload limit, upstream timeout and gzip setting. It takes
precedence over regular-expression locations in `advanced_config`. NPM imports map NPM custom
locations (the path part of NPM's forward host becomes `forward_path`).

`security_headers` (default `off`; NPM imports use `off`) adds browser security headers to the
host's responses, including gateway error pages. A header the upstream already sends is never
replaced:

| Value | Effect |
|---|---|
| `off` | Responses are not changed |
| `basic` | `X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: strict-origin-when-cross-origin`, `Permissions-Policy: browsing-topics=()`; the upstream's `X-Powered-By` is removed |
| `strict` | `basic` + `Content-Security-Policy-Report-Only: default-src 'self'; object-src 'none'; frame-ancestors 'self'; base-uri 'self'; form-action 'self'`, `Cross-Origin-Opener-Policy: same-origin`, and upstream `Set-Cookie` values get `HttpOnly`, `SameSite=Lax` and (on HTTPS) `Secure`, each only when the cookie does not set that attribute |

`Strict-Transport-Security` is independent of the preset: it is sent whenever `ssl_forced` is on.

`under_attack` ("under attack" mode, default `off`) challenges every visitor of the host:

| Value | Effect |
|---|---|
| `off` | No host-wide challenge |
| `on` | Every request without a valid pass gets the challenge page |
| `auto` | Like `on`, but only while corvus-control detects an attack on the host (see below) |

`under_attack_config` (missing values get the defaults):

| Field | Default | Meaning |
|---|---|---|
| `mode` | `js` | Challenge type, as for security rules: `js` (proof of work) or `cookie` |
| `difficulty` | `16` | Proof-of-work bits (12–24, `js` only) |
| `ttl_s` | `3600` | Pass lifetime (300–604800 s) |
| `bind` | `ip` | Pass bound to the client IP (`ip`) or IP and User-Agent (`ip_ua`) |
| `rps` | `200` | `auto`: activate above this many requests per second … |
| `blocked_pct` | `50` | … or when at least this share (%) of the requests was blocked or rate-limited (`0` = not used; needs at least 50 requests in the window) |
| `window_s` | `30` | … measured over this window (10–600 s, complete 10-second intervals) |
| `duration_s` | `900` | Activation period (60–86400 s) |

The challenge is the security-rule challenge (same page, `/__corvus/challenge/verify` endpoint and
`__corvus_ch` pass cookie, virtual rule id `0`; rule log field `ua:challenge`, `ua:challenge_pass`,
`ua:challenge_fail`, shown as `challenge_*` traffic events). Order of evaluation: IP bans → security
rules (block, allow, challenge) → exploit and body blocking → under attack challenge → rate limiting,
Basic auth and the access queue. Never challenged: requests an allow rule matched, certificate
validation (`/.well-known/acme-challenge/`), the gateway's own `/__corvus/` endpoints (challenge
answers, access queue) and client addresses in the automatic-ban exemptions (`exempt` of
`/api/settings/autoban`) and loopback.

In `auto` mode corvus-control checks every 10 seconds; once a threshold is exceeded the mode is active for
`duration_s`, and every round still over a threshold extends it to `duration_s` from then (it ends only after
the traffic stayed below the thresholds for a whole period). Challenged requests count as traffic, so an
ongoing flood keeps the mode active. Activations are pushed to the gateway's shared memory (no redeployment,
each entry also expires on its own), stored so they survive restarts of corvus-control, reported by the
`under_attack` alert rule (on and off) and listed by `GET /api/under-attack`:
`[{ "host_id", "host", "mode", "active", "since", "until", "reason" }]` (`since`/`until`/`reason` for
automatic activations). `deactivate` ends an activation early; the host is then not activated automatically
again for one `duration_s`.

Redirection host fields: `domains` (array), `forward_scheme` (`auto` keeps the request's scheme,
`http`, `https`), `forward_domain` (target `host[:port][/path]`, without `http://`), `http_code`
(`301`, `302`, `303`, `307`, `308`; default `301`), `preserve_path` (append the request path and
query; default `true`), `ssl_forced`, `http2`, `certificate_id` or `certificate_request`,
`block_exploits` (default `true`), `advanced_config`, `enabled`. Domains must not be used by another
proxy or redirection host. IP bans, security rules with scope "all" and exploit blocking apply to
redirection hosts too; rules scoped to specific hosts target proxy hosts only.

Stream fields: `incoming_port` (1–65535, the container port to listen on), `tcp_forwarding`
(default `true`), `udp_forwarding` (default `false`; at least one of the two), `forward_host`
(container name, DNS name or IPv4; `127.0.0.1`/`localhost` mean the Docker host, as for proxy hosts),
`forward_port`, `certificate_id` (optional TLS termination, TCP only: TLS from clients, plain TCP to
the forward host; the TCP listener opens once the certificate is issued), `enabled` (default `true`).
Ports Corvus listens on itself (HTTP, HTTPS, admin, internal status/ACME ports, the bundled Valkey)
are refused (`422`, field `incoming_port`), and an enabled stream may not share a port and protocol
with another enabled stream. On a Docker bridge network the port must also be published in compose.
Streams have no access log; bans and security rules do not apply to them yet.

`certificate_request` orders a Let's Encrypt certificate for the host's domains: `email`, `challenge`
(`http-01` or `dns-01`), `dns_provider`, `dns_credentials` (e.g. `{"api_token": "…"}` for Cloudflare), `staging`.
For DNS-01, `dns_credential_id` picks a saved DNS credential instead; new `dns_credentials` are saved
as a DNS credential (named `dns_credential_name`, or automatically) and the certificate references it.
The same fields work for `POST /api/certificates/acme`.

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
| GET / POST | `/api/ip-lists` | viewer / operator | IP and User-Agent lists |
| PUT / DELETE | `/api/ip-lists/{id}` | operator | Update / delete |
| POST | `/api/ip-lists/{id}/refresh` | operator | Fetch a URL-fed list now; returns `{ "ip_list" }` (a failed fetch is reported in `fetch_error`) |
| GET / POST | `/api/bans` | viewer / operator | IP bans |
| DELETE | `/api/bans/{id}` | operator | Remove a ban |
| POST | `/api/bans/{id}/permanent` | operator | Make a ban permanent |
| GET / PUT | `/api/settings/autoban` | viewer / operator | Automatic ban settings (threshold, first and longest ban, permanent ban from offense N, bad behaviour; see below) |
| GET | `/api/settings/pages` | viewer | Block and error pages: `{ "pages": { "block" \| "error": { "custom", "html", "default_html" } }, "max_bytes", "placeholders" }` |
| PUT | `/api/settings/pages/{page}` | operator | Set a custom page (`page` = `block` or `error`, body `{ "html" }`, at most 64 KB of UTF-8) and deploy |
| POST | `/api/settings/pages/{page}/reset` | operator | Restore the built-in page and deploy |
| GET / POST | `/api/rate-limits` | viewer / operator | Rate limits |
| PUT / DELETE | `/api/rate-limits/{id}` | operator | Update / delete |
| GET / POST | `/api/access-lists` | viewer / operator | Password protection lists |
| PUT / DELETE | `/api/access-lists/{id}` | operator | Update / delete |

**Security rule fields**: `name`, `description`, `enabled`, `action` (`block`, `allow`, `log`,
`challenge`), `status_code` (block only: `401`, `403`, `404`, `410`, `429`, `444`), `skip_basic_auth`
(allow only), `scope` (`all` or `hosts` with `host_ids`), `expression` (`{ "groups": [[condition, …], …] }`:
any group whose conditions all match), and for `challenge`:

| Field | Default | Description |
|---|---|---|
| `challenge_mode` | `js` | `js`: the browser solves a SHA-256 proof of work; `cookie`: the page only posts its signed token back (no computation) |
| `challenge_difficulty` | `16` | Leading zero bits the proof of work must reach (12–24, `js` only; each step doubles the work) |
| `challenge_ttl_s` | `86400` | How long a visitor who passed is not challenged again (300–604800 s) |
| `challenge_bind` | `ip` | Pass cookie bound to the client IP (`ip`) or to the IP and User-Agent (`ip_ua`) |

A **challenge** answers matching requests with a challenge page (status `403`, `Cache-Control: no-store`,
`X-Corvus-Challenge: js|cookie`, localized client-side in 8 languages, no external resources). The page
posts its answer to `/__corvus/challenge/verify` (POST only, at most 4 KB, rate-limited per IP); on success
the gateway sets the `__corvus_ch` cookie (HttpOnly, SameSite=Lax, Secure on HTTPS) and redirects (303) back
to the original URL — same-host paths only, and `/` for requests other than GET/HEAD, whose body cannot be
replayed. Requests with a valid pass skip challenge rules until it expires; block rules still apply. A pass
from a `js` challenge also satisfies `cookie` challenges and challenges of equal or lower difficulty.
Challenges and passes are HMAC-signed with a key derived from the access queue secret (`CORVUS_WR_SECRET`,
rotation via `CORVUS_WR_SECRET_PREV`) and bound to the client IP and host, so the gateway keeps no state;
a challenge must be answered within 5 minutes. Non-browser clients (API clients, feeds, monitoring, search
engine crawlers) cannot pass — put an allow rule above the challenge for them. Challenge pages are shown in
rule events (`challenge`, `challenge_pass`, `challenge_fail`) and traffic events (`challenge_issued`,
`challenge_passed`, `challenge_failed`); they are not counted as blocked requests, security detections,
scanner 4xx responses or bad behaviour. The challenge page cannot be customized yet.

**Automatic ban settings** (`/api/settings/autoban`):

| Field | Default | Description |
|---|---|---|
| `enabled` | `true` | Automatic bans (security detections, scanners and bad behaviour) |
| `threshold`, `window_s` | `20`, `600` | Security detections from one IP within the window (3–100000, 60–86400 s) |
| `duration_s`, `max_duration_s` | `900`, `604800` | First ban; doubles for each repeat offense up to the longest ban (at most 30 days) |
| `permanent_after` | `0` | Ban permanently from this offense on (2–100, `0` = never) |
| `include_scans`, `scan_threshold` | `false`, `200` | Also ban IPs with many 4xx responses (at least half of their requests) within the window |
| `exempt` | `[]` | CIDRs never banned automatically (loopback is always exempt) |
| `bad_behaviour` | `false` | Ban on bad behaviour immediately (requires `enabled`) |
| `bad_behaviour_codes` | `[400, 401, 403, 404, 405, 429, 444]` | Response status codes counted (400–599) |
| `bad_behaviour_threshold`, `bad_behaviour_window_s` | `10`, `60` | Counted responses per client IP **and proxy host** within the window (3–100000, 10–3600 s) |

Bad behaviour is counted in the gateway, so the IP is refused from the next request on, without
waiting for corvus-control's 15-second evaluation. Requests from banned IPs, from exempt networks,
requests a security rule allowed, challenge pages and the gateway's own endpoints (ACME challenges,
access queue, challenge answers) are not counted. corvus-control then records the ban (source `auto`, reason like
`bad behaviour: 12×404 on example.com in 60s`, audit action `ban.auto`) with the usual escalation;
the first gateway-local ban lasts `duration_s`.

**Lists** (`/api/ip-lists`) are referenced from security rules as `list:<id>`. Fields: `name`,
`description`, `kind` (`ip`, default, or `user_agent`; cannot change while rules use the list),
`items`, `source_url`, `refresh_s`; read-only: `rules` (number of rules using it), `fetched_at`
(last successful fetch), `checked_at` (last attempt), `fetch_error`, `fetch_warning`.

- `ip` lists hold IPv4/IPv6 addresses and CIDRs (at most 50,000) and are used with
  `{"field": "ip.src", "operator": "in" | "not_in", "value": ["list:<id>", …]}`.
- `user_agent` lists hold plain substrings (3–256 bytes each, at most 10,000), matched
  case-insensitively against the User-Agent — no wildcards or regular expressions, so a list cannot
  cause catastrophic backtracking. They are used with
  `{"field": "http.user_agent_list", "operator": "in" | "not_in", "value": ["list:<id>", …]}`;
  `in` matches when the User-Agent contains any entry, a request without a User-Agent never does.
  The gateway scans entries linearly (stopping at the first hit), which is cheap for the few
  thousand entries typical lists have.
- With `source_url` set, the list is fetched by corvus-control and `items` in requests are ignored.
  Only `https://` URLs are accepted, without credentials; the address is checked again after DNS
  resolution at connect time and loopback, private, link-local, CGNAT and other non-public
  addresses are refused (redirects are followed only under the same rules, environment proxies
  are not used). `refresh_s` is 300–604800 seconds (default 3600; failed fetches are retried after
  at most 15 minutes). The body (at most 5 MB) has one entry per line; empty lines and lines
  starting with `#` or `;` are ignored. For IP feeds the first word of each line is used, so
  `203.0.113.0/24 ; SBL123` and `198.51.100.7 # comment` work; for User-Agent feeds the whole line
  is used and backslash escapes (`Indy\ Library`) are removed. Invalid lines are skipped
  (`fetch_warning`), entries beyond the limit are dropped. A failed fetch, or a body without any
  valid entry, keeps the previous entries and sets `fetch_error`. The configuration is redeployed
  only when the set of entries changes. Saving a list with a new URL fetches it in the background.

The **block page** is shown when a security rule, a ban or attack-pattern blocking refuses a
request; the **error page** when a proxy host's upstream cannot be reached or times out (nginx's
own 502/503/504; error responses the upstream sends itself are passed through, and a rate limit
answering 503 keeps its JSON response). Pages are served as static HTML, never as configuration;
`{{STATUS}}`, `{{RID}}` (request id), `{{HOST}}`, `{{IP}}` and `{{TIME}}` are replaced with
HTML-escaped values. Custom pages are written into the configuration version directory, so they are
tested, deployed and rolled back with the rest of the configuration.

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
| POST | `/api/import/npm/preview`, `/apply` | admin | Import from Nginx Proxy Manager (`/apply` takes `proxy_host_ids`, `redirection_host_ids`, `stream_ids`, `certificate_ids`, `access_list_ids`; omitted = all) |

### Backup and restore

Admin with a signed-in session only (API tokens get `403 session_required`): the archive contains
the secrets. See *Data and backups* in the README for what is included.

| Method | Path | Role | Description |
|---|---|---|---|
| GET | `/api/backup` | admin (session) | Download `corvus-backup-<version>-<timestamp>.tar.gz` (`Cache-Control: no-store`, audited as `backup.download`) |
| GET | `/api/backup/restore` | admin (session) | Staged restore: `{ "state": "" \| "uploaded" \| "pending", "summary", "current_engine", "current_version", "restart_command" }` |
| POST | `/api/backup/restore` | admin (session) | Upload a backup (`multipart/form-data`, field `file`, at most 200 MB); validates it and returns the status with `summary` (`version`, `created_at`, `db_engine`, `has_db`, `files`, `bytes`, `secrets`, `certs`) and `state: "uploaded"` |
| POST | `/api/backup/restore/confirm` | admin (session) | Apply the uploaded backup on the next restart (`state: "pending"`) |
| DELETE | `/api/backup/restore` | admin (session) | Discard an uploaded or pending restore |

The archive is a `.tar.gz` of the data directory layout: `manifest.json`
(`{ "format": 1, "version", "created_at", "db_engine", "files": [{ "path", "size", "sha256" }] }`),
`corvus.db` (SQLite only; with PostgreSQL a `README.txt` instead), `secrets/`, `certs/` and `rules/`.

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
