# Contributing to doriki

Thank you for helping. doriki is small on purpose, so a few rules keep it that way.

## Before you open a pull request

1. Install Go 1.26 and golangci-lint v2.
2. Run `make gate` (build, lint, test) and make sure it is green. CI runs the same target.
3. Sign off every commit (`git commit -s`). doriki uses the
   [Developer Certificate of Origin](https://developercertificate.org/) instead of a CLA.

## What we accept

- Pull requests may be written with AI tools. Every pull request still needs a summary written
  by a human, and the author must have run the tests locally.
- Parser packs must come with a real fixture captured with `doriki capture` under
  `packs/<engine>/testdata/<version>/`, with a `SOURCE.md`. Synthetic logs are not accepted.
- Fixtures and golden files are never edited by hand. Regenerate them with
  `go test ./... -update` and explain the diff in the pull request.

## What we close

- Pull requests that only reformat code.
- Packs without a real fixture.

## Module boundaries

`make lint` enforces them: `internal/phase`, `internal/rules`, `internal/diff` and
`internal/render` import only the standard library, `internal/model` and their declared
libraries; `internal/collect/*` imports no internal package other than `internal/model`. See
`DESIGN.md` section 8.3.

## Conduct

Everyone taking part follows the [Code of Conduct](CODE_OF_CONDUCT.md).
