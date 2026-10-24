# DK-PHASE-ENH-20261024-forbidigo-purity: Lint does not stop clock or I/O calls in the pure packages

**Date Discovered:** 2026-10-24
**Severity:** MEDIUM
**Status:** CLOSED (2026-10-24)
**Label:** [ticket]
**Found by:** the M0 agent, read in config while writing `.golangci.yml`
**Filed by:** the M0 agent

## Problem
`phase`, `rules`, `diff` and `render` must do no I/O and read no clock (`CLAUDE.md`,
`DESIGN.md` section 8.3). The `depguard` rules in `.golangci.yml` (`pure-core`, `pure-render`)
allow `$gostd`, which includes `time`, `os`, `net` and `os/exec`. So a call to `time.Now()` or
`os.ReadFile` in `internal/phase` passes `make lint` today; only review catches it.

## Why it matters
A hidden clock read makes phase output depend on when the command ran, so golden tests flake
and the same events give different waterfalls. A user would see totals that change between
two runs on the same pod.

## Options
1. Enable `forbidigo` for the four pure packages, forbidding `time.Now`, `time.Since`,
   `time.Until`, `os.*` and `net.*` in non-test files. Recommended: one rule, enforced in lint,
   catches calls rather than imports.
2. Add a `deny` list of `os`, `net`, `os/exec`, `io/ioutil` to the existing `depguard` rules.
   Cheap, but cannot catch `time.Now` without banning `time.Duration` too.
3. Rely on review. No change, and no enforcement.

Recommendation: option 1, before the first logic lands in `internal/phase` (M1). It is a new
linter, so it needs the owner's approval under the project's dependency and lint rules.

## Related
`.golangci.yml` depguard rules; `docs/decisions/0001-docs-site-cut.md` (same task, unrelated);
the M0 skeleton task.
