# Security assurance case

Corvus is the first server that receives Internet traffic, so a weakness in it exposes everything
behind it. This document states what Corvus is meant to guarantee, what it is not, and why we
believe the guarantees hold. Vulnerabilities are reported as described in
[SECURITY.md](../SECURITY.md).

## Security requirements

What users can expect:

1. **Only the configured traffic is forwarded.** Requests reach a service only through a proxy host
   or stream the administrator created; unknown host names get no response from a service.
2. **Configured protections are applied as documented.** A *Block* security rule, an IP ban, an IP
   list, password protection, the under-attack challenge, rate limits and the access queue are
   enforced for every request they cover, in the order described in
   [How protection works](https://corvusgateway.org/security/).
3. **Only authorized administrators change the configuration.** The console and API require a
   signed-in user with a sufficient role; optional TOTP two-factor authentication and OIDC
   single sign-on are supported.
4. **A broken configuration never replaces a working one.** Changes are tested by nginx before
   they are activated; failed changes leave the running configuration as it was.
5. **Secrets stay secret.** Private keys, DNS provider tokens and TOTP secrets are not shown in the
   console, not written to logs, and stored encrypted or with file permissions limited to the
   service user.
6. **Released images are what the source says.** Images are built from tagged source by CI and
   signed, with SBOM and build provenance (see [Verify the image](https://corvusgateway.org/reference/hardened/)).

What Corvus does **not** promise:

- Detection signatures and virtual-patch templates reduce risk but cannot stop every attack on an
  application; they are not a replacement for updating vulnerable software.
- It does not protect against volumetric network floods that exceed the host's bandwidth.
- Anyone with access to the Docker host, the `/data` volume or an administrator account can
  change everything.

## Threat model and trust boundaries

| Boundary | Untrusted side | Main threats | Controls |
|---|---|---|---|
| Internet → gateway (80/443, streams) | Any client | Exploits against services, abusive traffic, spoofed client IPs, header/body tricks, challenge forgery | Lua checks before proxying; real client IP taken only from configured trusted proxies or Cloudflare ranges, otherwise `X-Forwarded-For` is overwritten; body inspection including HTTP/2 bodies without length; HMAC-signed challenge and queue cookies bound to client and host |
| Network → admin console (9443) | Anyone who can reach the port | Credential guessing, session theft, CSRF, XSS | HTTPS only; bcrypt password hashes; per-IP login throttling; TOTP with attempt limits; `HttpOnly`, `Secure`, `SameSite=Strict` session cookies; CSRF header and Origin check on every state change; strict security headers; optional allow-list of admin networks |
| Administrator → configuration | Signed-in users | Privilege escalation, configuration injection into nginx | Role checks on every API route; all fields validated (`control/internal/validate`) before rendering; advanced nginx snippets limited to administrators and tested with `openresty -t` |
| Internet → control plane (feeds, ACME, rule bundles) | Remote servers | Tampered rule bundles, downgrade | Rule bundles accepted only with a valid Ed25519 signature and a newer version; TLS certificate verification for all outbound HTTPS |
| Build → image | Upstream sources | Supply-chain compromise | OpenResty, OpenSSL and PCRE2 built from sources with pinned SHA-256 and verified signatures; actions pinned to commit SHAs; dependency and image scanning |

## Secure design principles applied

- **Least privilege:** all processes run as an unprivileged user; a hardened compose file runs with
  a read-only root filesystem, no capabilities and no privilege escalation. Redis listens only on
  127.0.0.1 and requires a password.
- **Fail-safe defaults:** new hosts block known exploit paths; the console requires a one-time setup
  token before the first account exists; invalid configurations are rejected rather than partly
  applied.
- **Complete mediation:** every request passes the same check order in the gateway, and every API
  call passes authentication, role and CSRF checks in one middleware chain.
- **Economy of mechanism:** one container, one configuration source (the database), a small set of
  features (see [ROADMAP.md](../ROADMAP.md)).
- **Separation:** the data plane keeps serving if the control plane is slow; log delivery uses a
  datagram socket that drops lines instead of blocking requests.

## Common weaknesses countered

| Weakness | How it is addressed |
|---|---|
| Injection (CWE-74, CWE-89) | Parameterized SQL only; nginx configuration rendered from validated values; no shell commands built from user input |
| Cross-site scripting (CWE-79) | React escaping; no `dangerouslySetInnerHTML` with user data; Content-Security-Policy on the console |
| CSRF (CWE-352) | Custom request header plus Origin check; `SameSite=Strict` cookies |
| Broken authentication (CWE-287, CWE-307) | bcrypt, login throttling, TOTP, session expiry and revocation |
| Sensitive data exposure (CWE-200, CWE-312) | Secrets encrypted with AES-256-GCM using a data key outside the database; secrets never returned by the API |
| Use of vulnerable components (CWE-1104) | `govulncheck`, `npm audit` and Trivy in CI; Dependabot; weekly upstream checks for OpenResty and OpenSSL |
| Memory safety (CWE-119 family) | Control plane in Go and gateway logic in Lua; the C components are upstream nginx/OpenSSL, kept current |

## Cryptography

Corvus uses standard libraries only (Go `crypto`, OpenSSL through OpenResty): TLS 1.2 and 1.3,
AES-256-GCM, HMAC-SHA-256, SHA-256, Ed25519 and bcrypt. Keys and credentials are generated at
first start and stored under `/data/secrets`, separate from code and configuration, so they can be
replaced without a new release. Certificates of outbound TLS connections are always verified.

## Verification

- **Tests:** Go unit, API and fuzz tests (statement coverage of the control plane kept at 80% or
  more by CI) and Lua unit tests run on every pull request; an
  integration suite (`deploy/dev/itest.sh`) exercises a real gateway, and every release image is
  smoke-tested in CI before it is published. Bug fixes come with a test that fails without the fix.
- **Static analysis:** `go vet`, `gofmt`, CodeQL code scanning, TypeScript strict mode.
- **Dependencies and image:** `govulncheck`, `npm audit`, Trivy (image and configuration).
- **Supply chain:** OpenSSF Scorecard; signed releases with provenance.
- **Reviews:** the code has had full security reviews (most recently in November 2026); findings
  are fixed through the normal pull request process.
