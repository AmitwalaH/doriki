# DK-CLI-ENH-20261029-capture-writes-checksums: A capture becomes a fixture only after its checksums are written by hand

**Date Discovered:** 2026-10-29
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the lv4-fixture-canary agent, walked: turning the first `doriki capture` bundle
into `packs/llamacpp/testdata/11889-lv4/`
**Filed by:** the lv4-fixture-canary agent, on the owner's instruction

## Problem
A contributor captures a pod with `doriki capture` to donate a fixture. The bundle's
`SOURCE.md` says what was removed, but `packs/fixtures_test.go` also needs a `## Checksums`
section with the sha256 of every captured file. The contributor computes and types them, and
the hand-written part and the part `capture` wrote end up in one file, marked apart by prose
(as in the `11889-lv4` fixture).

Cause: `sourceMD` (`internal/cli/capture.go`) writes no checksums; the fixture convention was
made for `kubectl`-made captures, before `capture` existed.

## Why it matters
Hand-typed hashes are where a fixture goes wrong quietly, and they are the step a first-time
contributor is most likely to give up on.

## Options
1. `capture` writes a `## Checksums` section in the fixture format for every file it wrote
   except the derived `report.json`, so the hand-written part is only the narrative above it.
   Recommended.
2. A `doriki pack` subcommand that adds the section to an existing bundle.

## Related
`packs/llamacpp/testdata/11889-lv4/SOURCE.md`; `DK-REL-ENH-20261026-editor-rewrites`.
