# AI declaration

Corvus is developed with the help of an AI coding assistant, [Claude](https://www.anthropic.com/claude)
by Anthropic, used through Claude Code. Commits it worked on carry a `Co-Authored-By: Claude`
trailer, so its involvement is visible in the history. This page says where AI is used, how much,
and how its work is checked. The structure follows [ai-declaration.md](https://ai-declaration.md/).

## Where AI is used

| Area | Involvement | What that means here |
|---|---|---|
| Design | Pair | Scope, priorities and the main decisions are made by the maintainer. Designs are worked out together and written down as architecture decision records before larger changes. |
| Implementation | Generated | The maintainer describes the change; the assistant writes most of the code; the maintainer reviews and corrects it before it is merged. |
| Testing | Generated | Unit, Lua and integration tests are mostly written by the assistant, to cases the maintainer asks for. Problems found on a live server become issues and tests first. |
| Documentation | Generated | Docs, README translations and release notes are drafted by the assistant and edited by the maintainer. |
| Review | Pair | The maintainer reviews every pull request; AI-assisted code reviews and security reviews of the whole code base are used in addition. |
| Deployment | Generated | CI, release signing and the build pipeline were written by the assistant and checked by the maintainer. |

Levels: *Hint* (AI suggests, a person does the work), *Assisted* (AI does part, a person most),
*Pair* (about half each), *Generated* (a person directs and reviews, AI writes it).

## How changes are checked

AI-written code is held to the same bar as any other code. Nothing reaches `main` directly, the
maintainer's own changes included:

- Every change is a pull request that must pass the required checks: Go vet and tests, the
  TypeScript build, Lua tests, and an **integration test** that builds the image and runs 368 checks
  against a real gateway, plus an upgrade test from the latest release.
- CodeQL runs on every pull request; dependency audits and the OpenSSF Scorecard run on `main` and
  on a schedule, and a daily job reports new versions of OpenResty, OpenSSL, PCRE2 and zlib.
- The whole code base had independent security reviews; the findings were fixed (see for example the
  [1.9.1 release notes](https://github.com/corvus-gateway/corvus/releases/tag/v1.9.1)).
- Larger changes are tried on a live server as release candidates before the final release.
- Releases are built in CI, signed (cosign), and come with an SBOM and build provenance, so you can
  verify that an image was built from this repository ([how](https://corvusgateway.org/reference/hardened/)).

## Other AI use

- The narration of the [intro video](https://youtu.be/wtd2yBssBUk) is AI-generated (Kokoro TTS).
  The demo in it uses example data only.

## Contributions

Contributions written with AI help are welcome under the same rules: open a pull request, make the
checks pass, and say in the description if AI wrote a significant part. You are responsible for
code you submit, however it was written. See [CONTRIBUTING.md](CONTRIBUTING.md).

Questions about this page are welcome in [Discussions](https://github.com/corvus-gateway/corvus/discussions).
