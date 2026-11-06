# DK-PHASE-ENH-20261025-server-ready-resolution: server-ready shows 0 s when the server took up to a second

**Date Discovered:** 2026-10-25
**Severity:** LOW
**Status:** CLOSED (2026-11-06)
**Label:** [ticket]
**Found by:** the M1b agent, measured on the vLLM 0.32 fixture
**Filed by:** the M1b agent

## Problem
On `packs/vllm/testdata/0.32`, `server-ready` starts at `Application startup complete.`
(09:31:16.038564038, runtime timestamp, nanoseconds) and ends at the pod's Ready condition
(`lastTransitionTime` 09:31:16, whole seconds). The end is 0.039 s before the start, which is
inside the 1 s tolerance, so the report shows `server-ready` complete at 0 s. The real time from
the server listening to the kubelet marking the pod Ready is somewhere between 0 and about 1 s,
plus the readiness probe period (2 s in the fixture).

Cause: `Ready`'s `lastTransitionTime` has 1 s resolution
(`internal/collect/files/files.go`, `podEvents`), and `internal/phase/assign.go` clamps any end
within 1 s before its start to 0 s. Same class as `DK-COLLECT-ENH-20261024-pull-duration`
(Kubernetes event times truncated to the second).

## Why it matters
Small on its own, but a 0 s phase reads as "free"; on a fast server most of server-ready is
probe latency the user could tune (`periodSeconds`), and the report cannot show it.

## Options
1. Show the resolution honestly: when a phase's end or start has whole-second resolution and the
   phase is under 1 s, render it as `<1s` in text and add an attribute such as
   `resolution: "1s"` in JSON. Recommended: cheap, honest, no new data source.
2. Take a finer Ready time from a source that has one (the readiness probe's first success in
   kubelet logs or events with `eventTime`), when available. Better data, more work; M2 or later.
3. Leave it.

## Related
`DK-COLLECT-ENH-20261024-pull-duration`; `DESIGN.md` sections 4.2 and 6.1 step 4.

## Resolution (2026-11-06)
Option 1, in `internal/phase/assign.go` (`assignPhase`, `wholeSecond`) and
`internal/render/text/text.go`. A complete phase under 1 s with a bound from a whole-second
source gets `attrs.resolution: "1s"`, and the terminal prints `<1s` for it. The whole-second
sources, read from the collectors: `container-status` (creation, container start and end, the
Ready and PodScheduled conditions; all `metav1.Time`, written in whole seconds) and
`k8s-event` (`firstTimestamp` and `lastTimestamp`), unless the time has a fraction, as an
`eventTime` (microseconds) does. A phase that starts and ends at one event, a cached pull
(decision 0004), keeps its exact 0 s. The rule is wider than this ticket's server-ready: on the
six fixtures it marks 13 rows, `scheduling` on all six, `container-init` on five and
`server-ready` on vLLM and llama.cpp `-lv 4`. Their CLI goldens, `expected.json` files and
the `-lv 4` `report.json` change on those rows only, re-recorded in their own commit after the
owner's approval. Option 2 (a finer Ready time) stays open for later.
