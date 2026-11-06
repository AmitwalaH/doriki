# DK-PHASE-ENH-20261024-unattributed-before-ready: Time between the first log line and Ready is in no phase and no gap

**Date Discovered:** 2026-10-24
**Severity:** MEDIUM
**Status:** CLOSED (2026-11-06)
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

## Resolution (2026-11-06)
Option 1, in `internal/phase/assign.go` (`gaps`, `between`). When the pod is Ready, the time
from the latest end among complete phases to Ready is a gap with `before: ready`
(`model.PhaseReady`, a gap bound only, never a phase) when it is over 1 s. On a later attempt
it splits into "earlier attempts" and unattributed time like any other gap (decision 0007). The
report schema's `Gap.before` gains the value `ready` and nothing else (`schema/schema.go`,
`readyBoundsGaps`). The terminal draws it as a `gap` row under its `after` phase with no change
to the renderer. A property test (`TestBarsAddUpWhenReady`) checks on random inputs that the
gaps account for the time no complete phase covers up to Ready. Measured on the six fixtures:
the time after the last complete phase is under 1 s on every one (kind-nginx 0.69 s, now the
largest), so no fixture shows the gap; the 40 s case is tested synthetically. Two limits found
on the way are their own tickets: `DK-PHASE-ENH-20261106-split-gap-at-attempt-boundary` and
`DK-COLLECT-ENH-20261106-engine-log-whole-second-times`.
