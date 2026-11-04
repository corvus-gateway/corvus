# Contributing to Corvus

Thanks for your interest! Bug reports, false-positive reports for security rules, translations
and pull requests are all welcome.

## Reporting

- **Bugs and feature ideas:** open an issue with your version (bottom of the console sidebar, or the image tag),
  what you did, what you expected and what happened.
- **A legitimate request was blocked (false positive):** include the rule or signature name from
  the *Rule events* / *Events* view and a redacted example of the request.
- **Security vulnerabilities:** do **not** open an issue — see [SECURITY.md](SECURITY.md).

## Development setup

Requirements: Go (version from `go.mod`), Node.js 22+, Docker.

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only
```

Full image: `docker build -f deploy/Dockerfile -t corvus .` (compiles OpenResty, takes a while).
`deploy/dev/` contains a test stack (Pebble ACME CA, demo backend) and the integration suite
`deploy/dev/itest.sh`.

## Pull requests

- Keep changes focused; one topic per pull request.
- CI must pass: `go vet`, `go test -race`, `govulncheck`, TypeScript build, `npm audit`, Lua syntax check.
- New user-facing text goes through the i18n dictionaries in `web/src/locales/` — add the English
  string at least; other languages are welcome.
- **Tests:** new functionality must come with automated tests (Go unit tests, fuzz tests for
  parsers and validators, or a case in `deploy/dev/itest.sh` for gateway behaviour). Bug fixes
  should add a test that fails without the fix. Security-relevant changes (rules engine, auth,
  config generation) are not merged without tests.
- All changes reach `main` through a pull request with passing checks (go, web, lua) — this
  applies to maintainers too.
- By contributing you agree that your contribution is licensed under the
  [Apache License 2.0](LICENSE).
