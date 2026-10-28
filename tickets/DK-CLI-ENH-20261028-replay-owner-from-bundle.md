# DK-CLI-ENH-20261028-replay-owner-from-bundle: A replayed capture can name a different owner than the live report

**Date Discovered:** 2026-10-28
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M3 agent, read in code while writing `capture` (slice 5)
**Filed by:** the M3 agent, on the owner's instruction

## Problem
A user captures a pod whose ReplicaSet belongs to a Deployment whose name does not follow the
`<deployment>-<pod-template-hash>` pattern (or whose ReplicaSet was adopted). Live, `doriki`
walks the ReplicaSet and the manifest says `owner: Deployment/real-name`. `doriki --from-files
<bundle>` says `Deployment/<inferred-name>`, and the fingerprint differs from the live one.

Cause: `capture` (`internal/cli/capture.go`) writes the walked owner only into `manifest.json`
and `report.json`. On replay, `files.LoadDir` (`internal/collect/files/files.go`) reads
`pod.json` and infers the owner from the ReplicaSet name (`inferOwner`,
`internal/collect/files/manifest.go`); it never reads the bundle's `manifest.json`.

## Why it matters
Two reports of one boot disagree on a manifest field, and `diff` (v0.2) would list the owner as
changed. Timing is not affected, and for Deployment-made ReplicaSets the inference is right.

## Options
1. `LoadDir` reads `manifest.json` when present and passes its `pod.owner` as the walked owner
   (`Objects.Owner`), so replay matches live, with the existing owner-mismatch warning when the
   pod says otherwise. Recommended: one field, read where the bundle is read.
2. `capture` records the walked owner in an annotation on the bundle's pod.json. Writes doriki
   data into a Kubernetes object; rejected.

## Related
M3 slice 5 (`capture`); `inferOwner`.
