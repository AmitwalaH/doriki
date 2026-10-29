# DK-REL-ENH-20261024-forbidigo-canary-ci: A later lint config edit can silently stop the purity bans

**Date Discovered:** 2026-10-24
**Severity:** LOW
**Status:** CLOSED (2026-10-29), by `build(lint): canary for the purity bans`
**Label:** [ticket]
**Found by:** the M1a agent, measured by running a canary script against `.golangci.yml`
**Filed by:** the M1a agent

## Problem
`.golangci.yml` bans clock reads and I/O in `phase`, `rules`, `diff`, `render` and `normalize`.
If someone narrows a pattern, renames a package, or upgrades golangci-lint and the semantics
change, `make lint` still says `0 issues.` and nobody notices that the bans no longer work.

This already happened once, while the bans were being written: a pattern matched against the
import path (`os/exec`) instead of the package name (`exec`) and silently let `exec.Command`,
`http.Get` and `filepath.WalkDir` through. Only a scratch canary script caught it. That script
plants 13 calls in each pure package, in `cli`, and in `_test.go` files, and checks that
exactly the 11 forbidden ones are flagged and only where they should be. It was not committed.

## Why it matters
The bans keep phase output independent of when the command ran. If they stop working, a
`time.Now()` in the core makes golden tests flake and totals change between runs on the same
pod, and lint still reports green.

## Options
1. Commit the canary as `test/lintcanary/` (Go source kept out of the build with a build tag,
   plus a script), and run it as a `make lint-canary` target in CI. Recommended: it tests the
   real linter against the real config.
2. Rely on golangci-lint's own tooling. `golangci-lint config verify` only checks the
   config's shape, not what it catches; I know of no fixture-based mode that would, but did
   not search beyond v2.15.0's `--help`.

## Related
`DK-PHASE-ENH-20261024-forbidigo-purity` (closed); `DK-PHASE-ENH-20261024-nondeterminism-pure`.

## Resolution
Option 1. `test/lintcanary/` holds Go source behind the `lintcanary` build tag: every call the
forbidigo bans name, planted once where `internal/phase` lives, one call per ban and one
disallowed import in each other pure package (`rules`, `diff`, `normalize`, `render`), and the
same calls unflagged in a pure package's `_test.go` and in a `cli` position. Each line that
must be flagged says so in a `// want <linter>` comment. `TestPurityBans` runs the real
golangci-lint with `.golangci.yml` (or `LINT_CANARY_CONFIG`) and fails on any line flagged and
not planted, or planted and not flagged; it also fails when the linter on PATH is not the
version `ci.yml` pins. `make lint-canary` runs it, and CI's `gate` job calls it after `make
gate`. A copy of `.golangci.yml` with `rules` dropped from forbidigo's scope made it fail on
exactly the four `rules` plants (2026-10-29).
