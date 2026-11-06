# DK-PHASE-ENH-20261106-split-gap-at-attempt-boundary: Up to 2 s of unattributed time can vanish where a later attempt begins

**Date Discovered:** 2026-11-06
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the time-near-ready agent, read in code while writing the "bars add up" property
**Filed by:** the time-near-ready agent

## Problem
A user runs doriki on a pod that restarted. On the latest attempt, the stretch between a
pod-level phase (scheduling) and the next complete phase, or Ready, is cut at the attempt's
boundary into an "earlier attempts" piece and an unattributed piece. Each piece is reported
only when it is over 1 s. A 1.8 s stretch that the boundary splits 0.9 s / 0.9 s shows no gap
at all, so the bars fall short of the total by up to 2 s with nothing on screen.

Cause: `internal/phase/assign.go`, `between` (moved there from `gaps` in this task, unchanged):
the threshold `gapOver` is applied to each piece, not to the whole stretch.
`internal/phase/property_test.go` (`owed`) allows for it explicitly with a 2 s slack per
stretch.

Not measured on a fixture: kind-restart's earlier-attempts gap is 3 s and is reported.

## Why it matters
Small, and only on restarted pods, but the same "every number traces to an event" promise
(`PROJECT.md` section 8): the missing time is in the total and in no row.

## Options
1. Apply the threshold to the whole stretch, and report both pieces when the stretch is over
   1 s, even if one piece is 1 s or less. Recommended: the property's slack drops to 1 s.
2. Report the larger piece only, carrying the whole stretch.
3. Leave it: under 2 s, on restarts only.

## Related
`DK-PHASE-ENH-20261024-unattributed-before-ready`; decision 0007;
`DK-PHASE-BUG-20261029-gap-after-nested-phase`.
