# DK-PHASE-BUG-20261024-attempt-boundary-repull: A restart's image re-pull is counted in the previous attempt

**Date Discovered:** 2026-10-24
**Severity:** HIGH (raised from MEDIUM on 2026-10-25 by the owner, after the M1b spike findings below)
**Status:** OPEN, scheduled for M2 (live collector)
**Label:** [ticket]
**Found by:** the owner, reviewing the M1a slice 3 report; read in code
**Filed by:** the M1a agent

## Problem
A user profiles a pod with `imagePullPolicy: Always` that crashed and restarted. The kubelet
pulls the image again before starting the new container. The report for the latest attempt
shows `image-pull` as absent, even though the restart spent time pulling. That pull time is
attributed to the previous attempt's window, where it is ignored because that attempt has
already matched its own pull.

Cause: `internal/phase/assign.go` (`splitAttempts`) cuts the event stream at each
`ContainerStarted` event. A later attempt runs from its own container start, so every event
that comes before the start, such as `Pulling`/`Pulled` for the re-pull, falls into the
previous attempt.

## Why it matters
The M3 `restart-loop` rule says "each restart repays compile/graph"; if re-pull time is
missing from each restart, the report understates what a restart costs, and `image-pull`
appears to take no time on restarted pods.

## Options
1. Recommended (the owner's direction): the boundary of attempt N is the previous
   container's termination time (`lastState.terminated.finishedAt` from the pod object,
   emitted by the collector as its own event kind), with attempt N's container start as the
   fallback when no termination time is known. Must land before the M3 `restart-loop` rule.
2. Keep the start as the boundary and document the limit.

## Related
`DESIGN.md` section 6.1 step 7; M1a slice 3 (`feat(phase): pure assignment with attempts,
gaps and totals`); M3 `restart-loop` rule.

## Update (2026-10-25, M1b agent, measured in the M1b spike)
The bug is wider than `imagePullPolicy: Always`. In the M1b spike a vLLM pod crash-looped
(3 restarts, default `IfNotPresent`), and before every restart the kubelet wrote a cached
`Pulled` ("already present on machine"), which decision 0004 now turns into a 0 s image-pull.
That event comes before the restart's container start, so it still falls into the previous
attempt, and the latest attempt shows `image-pull` absent. Since image-pull is an expected
phase, every restarted pod would exit 1 once the restart is the reported attempt. This is not
a regression (before decision 0004 that `Pulled` matched nothing), but it raises the stakes.

Second finding from the same capture: the kubelet aggregates repeated events. The spike's
events had one `Pulled` with `count: 3`, `firstTimestamp` 08:55:55 and `lastTimestamp`
08:58:37, and one `Started` with `count: 4`. The collector reads only `firstTimestamp`
(`internal/collect/files/files.go`, `k8sEvents`), so for a restarted pod it sees the first
restart's time, not the latest one's. Option 1 above should take the time from
`lastTimestamp` (or the series) when choosing the event for a later attempt.
