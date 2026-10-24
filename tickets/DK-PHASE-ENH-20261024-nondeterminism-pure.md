# DK-PHASE-ENH-20261024-nondeterminism-pure: Nothing stops random or map-order output in the pure packages other than phase

**Date Discovered:** 2026-10-24
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M1a agent, read in `.golangci.yml` while adding forbidigo
**Filed by:** the M1a agent

## Problem
A user runs `doriki --from-files ...` twice on the same files and gets two reports that differ:
advice in a different order, or a warning list reshuffled. Nothing in lint or tests prevents
this today in `normalize`, `rules`, `diff` or `render`.

Cause: forbidigo in `.golangci.yml` bans clock reads and I/O in the pure packages, but not
`math/rand`, `math/rand/v2` or `crypto/rand`. And neither forbidigo nor depguard can catch the
commonest source, ranging over a map to build output. Only `phase` has a guard: the
`testing/quick` property test in `internal/phase/property_test.go` checks that the same events
in any order give the same report.

## Why it matters
Golden tests flake, and `diff` (v0.2) would report changes between two runs of the same pod.
A user comparing reports in CI would see noise and stop trusting the numbers.

## Options
1. Add `math/rand`, `math/rand/v2` and `crypto/rand` to the forbidigo I/O rule, and give each
   pure package's top-level function a property test like phase's (shuffled input, same
   output) as it gains logic. Recommended: the lint part is one line, and the tests come with
   the packages.
2. Lint only. Cheap, but misses map iteration, which is the likelier bug.

## Related
`DK-PHASE-ENH-20261024-forbidigo-purity` (closed); `DK-REL-ENH-20261024-forbidigo-canary-ci`;
`DESIGN.md` section 9.4.
