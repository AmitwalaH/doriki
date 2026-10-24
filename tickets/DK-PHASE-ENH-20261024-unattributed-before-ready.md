# DK-PHASE-ENH-20261024-unattributed-before-ready: Time between the first log line and Ready is in no phase and no gap

**Date Discovered:** 2026-10-24
**Severity:** MEDIUM
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M1a agent, read in code while planning, then measured on the kind-nginx fixture
**Filed by:** the M1a agent

## Problem
A user runs `doriki --from-files` on a pod whose engine has no pack. The report shows
`container-init` ending at the first log line, then nothing until `Ready`. `pending_to_ready`
includes that time, but no phase and no gap does, so the bars add up to less than the total
with no explanation. On `packs/generic/testdata/kind-nginx` it is 0.69 s (first log line
14:58:13.306, Ready 14:58:14); on an LLM engine it would be most of the boot.

Cause: `internal/phase/assign.go` (`gaps`) reports gaps only between two complete phases, as
`DESIGN.md` section 6.1 step 5 says. Under the generic pack every phase after `container-init`
is absent, including `server-ready`, whose end is Ready, so there is no complete phase after
the first log line to bound a gap.

## Why it matters
`PROJECT.md` section 8 says every number traces to an event. Here the largest share of the boot
traces to nothing on screen, and a user reads the waterfall as "the pod was ready right after
the container started".

## Options
1. Add a trailing gap from the last complete phase's end to Ready, labelled `before: ready`
   (a new value for `Gap.Before`, append-only). Recommended: honest, small, keeps 6.1's rule
   for gaps between phases.
2. Show it only in the text renderer (M1b) as "unattributed", not in JSON.
3. Leave it: the total already includes it.

## Related
Decision 0003 (generic pack exit code); `DESIGN.md` sections 4.2 and 6.1.
