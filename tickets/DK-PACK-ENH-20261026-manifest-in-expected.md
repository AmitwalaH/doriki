# DK-PACK-ENH-20261026-manifest-in-expected: A pack fixture does not freeze the manifest it produces

**Date Discovered:** 2026-10-25
**Severity:** MEDIUM
**Status:** CLOSED (2026-10-28)
**Label:** [ticket]
**Found by:** the M1b plan review (promised there, never filed); confirmed by the M2 agent, read
in code
**Filed by:** the M2 agent, on the owner's instruction

## Problem
A pack author changes a fact regex (`engine.version`, `model.ref`) so that it reads the wrong
value, or nothing. `go test ./packs/...` stays green, and the report shows a wrong or missing
engine version or model in its manifest.

Cause: `DESIGN.md` section 5.4 says each fixture's `expected.json` holds the report's `phases`
and `manifest` sections, but `packs/golden_test.go` (`expected`, lines 15 to 22) freezes only
phases, gaps and totals, with a comment that the manifest joins "once M3 fills it". The only
check on facts today is `packs/vllm_test.go`, which shows each fact reads a real line, not that
the pipeline puts the right value in the manifest.

## Why it matters
The manifest is what `diff` and the fingerprint compare. A fact regression would make two
boots of the same deployment look different, or two different ones look the same, and no test
would say so.

## Options
1. At M3, add the manifest to `expected` in `packs/golden_test.go` and re-record every fixture's
   `expected.json` once, after the manifest fields settle (M2 adds container, node and
   accelerator fields). Recommended: one re-record, after the shape is stable.
2. Add it now. Every M2 and M3 manifest field would then re-record every fixture again.

## Related
`DESIGN.md` sections 4.3 and 5.4; M2 task acceptance 5 (manifest fields stay out of
`expected.json`; the CLI JSON goldens cover them); `packs/golden_test.go`.

## Resolution
Option 1, in M3 slice 7, after the manifest fields settled (M2 container, node and accelerator;
M3 `node.kubelet`, `model.source`, `model.loaded_gib`, `caches`). `expected` in
`packs/golden_test.go` now holds the manifest, and every fixture's `expected.json` was
re-recorded once with the owner's marker (`go test ./packs/ -run TestFixtures -update`) after a
side-by-side review: each file only gained its manifest block (21, 21, 19, 39 and 32 lines), and
phases, gaps and totals are unchanged. Checked against the captures: pod namespace, name, UID,
image, digest and owner reference agree with each fixture's `pod.json`, and each fingerprint
equals the one in the CLI goldens (kind-nginx and kind-nginx-warm share one: two boots of one
Deployment). `TestDocumentsValidate` (`schema/schema_test.go`) now also validates each
fixture's manifest against `schema/manifest.v1.json` (owner-approved edit).
