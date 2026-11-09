# Security Policy

Corvus sits in front of your services, so security reports are handled with priority.

## Reporting a vulnerability

Please **do not open a public issue** for a vulnerability.

Report it privately through GitHub:
**[Security → Report a vulnerability](https://github.com/corvus-gateway/corvus/security/advisories/new)**.

Include, if you can:

- the affected version or commit,
- the component (gateway / Lua, control plane / API, admin console, deployment),
- steps to reproduce or a proof of concept,
- the impact you expect (e.g. rule bypass, authentication bypass, information disclosure).

You will get an acknowledgement within 7 days. Once a fix is ready, it is released together with
a GitHub security advisory, and reporters are credited unless they prefer otherwise.

## How reports are handled

1. **Acknowledge** within 7 days and confirm whether the issue reproduces.
2. **Assess** the impact and severity (CVSS) together with the reporter, in a private GitHub
   security advisory.
3. **Fix** in a private fork of the advisory, with a regression test. The target is a release
   within 30 days of confirmation for high and critical issues, sooner when the issue is exploited.
4. **Release** a patched version (release candidate first when time allows), then publish the
   advisory, request a CVE through GitHub, and credit the reporter.
5. **Disclose** details no later than 90 days after the report, or earlier once a fix is out.

Release notes describe security fixes without exploit details.

## Supported versions

Security fixes are released for the latest minor version of 1.x. Upgrade with
`docker compose pull && docker compose up -d`.

## Scope notes

- Detection signatures and virtual-patch templates reduce risk but cannot catch every attack. A
  bypass of a signature is welcome as a report, but it is treated as a rule improvement rather than
  a vulnerability unless it bypasses an explicit guarantee (for example, a *Block* security rule).
- Managed rule bundles are accepted only when signed with the project key. A way to make the
  gateway accept an unsigned or downgraded bundle **is** a vulnerability.

For what Corvus is designed to guarantee, see the
[security assurance case](docs/SECURITY-ASSURANCE.md).
