# Contributing to Corvus

Thanks for your interest! Every kind of help is welcome — bug reports, false-positive reports,
ideas, translations, documentation fixes and code. You don't need to get everything right the
first time; small and imperfect contributions are fine, and we're happy to help finish them.

Please be kind to each other — see the [Code of Conduct](CODE_OF_CONDUCT.md).

## Asking and reporting

- **Questions, ideas, showing your setup:** [Discussions](https://github.com/corvus-gateway/corvus/discussions).
- **Bugs and feature requests:** [open an issue](https://github.com/corvus-gateway/corvus/issues/new/choose).
  A short description is enough; the version (bottom of the console sidebar, or your image tag)
  helps.
- **A legitimate request was blocked (false positive):** use the *Blocked by mistake* issue form.
  The rule or signature name from *Rule events* / *Events* and a redacted example help most.
- **Security vulnerabilities:** please do **not** open an issue — see [SECURITY.md](SECURITY.md).

## Pull requests

1. Fork the repository and create a branch.
2. Make your change. Keep it to one topic if you can — it makes review faster.
3. Open a pull request (a draft is fine if you want early feedback).

CI checks run automatically (Go vet and tests, TypeScript build, Lua syntax, dependency audits).
If something fails and you're not sure why, just ask in the pull request.

A few things we look for before merging — we can help with any of them:

- **Tests:** new functionality gets automated tests (Go unit tests, fuzz tests for parsers, or a
  case in `deploy/dev/itest.sh`), and bug fixes ideally get a test that fails without the fix.
  Changes to the rules engine, authentication or configuration generation are not merged without
  tests. If you don't know how to test something, open the PR anyway and we'll work it out together.
- **Style:** Go code is formatted with `gofmt`; for TypeScript, follow the style of the surrounding code.
- **Text in the console:** add new strings to the dictionaries in `web/src/locales/`. English is
  enough — other languages are welcome but optional.
- **License:** by contributing you agree that your contribution is licensed under the
  [Apache License 2.0](LICENSE).

All changes, including the maintainer's own, reach `main` through a pull request with passing checks.

## Development setup

Requirements: Go (version from `go.mod`), Node.js 22+, Docker.

```bash
npm --prefix web ci && npm --prefix web run build      # admin UI → embedded into the Go binary
go test ./control/...                                   # unit tests
for f in gateway/test/*_test.lua; do luajit "$f"; done # Lua unit tests (pure helpers)
go build -o bin/corvus-control ./control/cmd/corvus-control
CORVUS_GATEWAY=0 CORVUS_ADMIN_TLS=0 CORVUS_DATA_DIR=./tmp bin/corvus-control   # console only
```

Full image: `docker build -f deploy/Dockerfile -t corvus .` (compiles OpenResty, takes a while).
`deploy/dev/` contains a test stack (Pebble ACME CA, demo backend) and the integration suite
`deploy/dev/itest.sh`.

The HTTP API used by the console is described in [docs/API.md](docs/API.md).
