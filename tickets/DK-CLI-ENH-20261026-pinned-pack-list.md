# DK-CLI-ENH-20261026-pinned-pack-list: Every new pack must also edit two CLI tests that list the packs

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M2 agent, measured: adding `packs/llamacpp/11889.yaml` failed `TestPackList`
and `TestVersionDefaults`
**Filed by:** the M2 agent, on the owner's instruction

## Problem
A contributor adds a pack with a real fixture, and `make gate` fails in two CLI tests that
have nothing to do with the pack: `internal/cli/profile_test.go` (`TestPackList`) and
`internal/cli/version_test.go` (`TestVersionDefaults`) compare the output of `doriki pack list`
and `doriki version` with a literal list of the embedded packs. Both are judges, so each new
pack needs an approved edit to them.

## Why it matters
It adds a judge edit to every pack PR, the kind of change community fixtures should not need.
The owner chose on 2026-10-26 to keep the tests pinned for now: a pinned list also catches a
pack embedded by mistake.

## Options
1. Compare with `packs.Names()` and keep one pinned case with a sentinel `packNames` (as
   `TestPackList` already does for its second case). New packs then need no CLI test edit.
2. Keep the literal lists and accept the edit per pack.

## Related
M2 slice 5 (`llamacpp/11889`); `packs/doc.go` (`Names`).
