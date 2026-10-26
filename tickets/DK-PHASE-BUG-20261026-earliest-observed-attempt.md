# DK-PHASE-BUG-20261026-earliest-observed-attempt: With two or more restarts, the earliest attempt shown takes an older attempt's pull and run time

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M2 agent, walked: `doriki pod/restart-once --all-attempts -o json` against the
live kind cluster, on the slice 2 fixture pod after it had restarted a second time
**Filed by:** the M2 agent

## Problem
A user profiles a pod that has restarted twice or more and asks for `--all-attempts` (or
`--attempt` with the earliest number shown). The pod object keeps only the current container
and the previous one, so the attempts shown are, say, 1 and 2; attempt 0 is not observed.
Attempt 1 then shows:

- an `image-pull` at the pod's very first pull (11:50:42 in the walk), which was attempt 0's;
- a plain `gap` with no reason (3 s in the walk), which was attempt 0 running and crashing.

The default report, the latest attempt, is correct: it starts at the previous container's end
and labels the time before it "earlier attempts" (decision 0007).

Cause: `internal/phase/assign.go`, `splitAttempts`, gives the earliest observed attempt every
event before its start (`out[0].from` is zero), on the assumption that it is attempt 0. When
its restart count is above 0, that history belongs to attempts that are no longer in the pod
object. Its own re-pull is also not recoverable when the kubelet merged the `Pulled` events
(only the first and last time survive; recorded as the known limitation in
`DK-PHASE-BUG-20261024-attempt-boundary-repull`).

## Why it matters
`--all-attempts` is how a user compares boots after a crash loop. Showing attempt 0's pull as
attempt 1's, and its run as unattributed time, misstates what attempt 1 cost.

## Options
1. When the earliest observed attempt's number is above 0, give it a boundary at its own start
   (the previous end is not known), so pod-level phases still show, earlier time is labelled
   "earlier attempts", and an image-pull it cannot see is absent rather than borrowed.
   Recommended; it is decision 0007's rule applied to the one attempt it skipped.
2. Leave it and document that the earliest attempt shown may include older history.

## Related
Decision 0007; `DK-PHASE-BUG-20261024-attempt-boundary-repull` (closed); M2 slice 3b
(`--all-attempts` wired).
