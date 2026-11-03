# DK-CLI-ENH-20261029-capture-writes-checksums: A capture becomes a fixture only after its checksums are written by hand

**Date Discovered:** 2026-10-29
**Severity:** LOW
**Status:** CLOSED (2026-11-03), by `feat(cli): capture writes the fixture Checksums section into SOURCE.md`
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

## Resolution
Option 1. A bundle's `SOURCE.md` ends with a `## Checksums` section: one line per file
`capture` wrote except `SOURCE.md` and the derived `report.json`, sorted, from the bytes as
written (after strict's hashing). The format, the files that are not captures and the check
live in `internal/fixture`, which `packs/fixtures_test.go` and `capture` both use. A file the
contributor adds (the applied manifest) gets a line in a hand-written `## Checksums` section,
from `sha256sum`; every such section counts (`docs/fixtures.md` step 4). Bundles from
v0.1.0-rc.2 and older have no section and need every line by hand the same way. The existing
capture-made fixture, `11889-lv4`, was not re-captured. Tests: `TestCaptureChecksums`
(default, strict, restart), `TestCaptureBundleIsFixture`, `internal/fixture`'s `TestVerify`,
`TestVerifyMergesSections` and `TestLines`.
