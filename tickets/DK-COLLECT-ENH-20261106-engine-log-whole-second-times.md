# DK-COLLECT-ENH-20261106-engine-log-whole-second-times: An engine-log time read from the engine's own whole-second timestamp is not marked as whole-second

**Date Discovered:** 2026-11-06
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the time-near-ready agent, read in code while deciding which sources are whole-second
**Filed by:** the time-near-ready agent

## Problem
A user runs doriki on logs collected without `--timestamps`. Lines then have no runtime
timestamp, and `internal/collect/enginelog/enginelog.go` (`inlineTime`) dates them from the
engine's own timestamp, in the pack's layout. vLLM's layout, for example, has whole seconds. A
phase under 1 s bounded by such a line prints a figure such as `0.0s` that reads as exact,
while a phase bounded by a whole-second Kubernetes time prints `<1s`
(`DK-PHASE-ENH-20261025-server-ready-resolution`).

Cause: `internal/phase/assign.go` (`wholeSecondSources`, `wholeSecond`) decides by source, and
every engine-log event is `SourceEngineLog`, whether its time came from the runtime
(nanoseconds) or from the engine's line. The event does not record which.

Not measured on a fixture: all six fixtures' logs have runtime timestamps.

## Why it matters
The same false precision the resolution ticket removed, on a less common path (logs saved
without `--timestamps`).

## Options
1. `enginelog` marks an event it dated from the engine's line with an attribute naming the
   layout's resolution, and `wholeSecond` reads it. Recommended: the event knows, the phase
   engine stays a pure reader.
2. Mark every engine-log time whose fraction is zero. A runtime time at an exact second would be
   marked too; rare, but wrong.
3. Leave it.

## Related
`DK-PHASE-ENH-20261025-server-ready-resolution`; `DK-COLLECT-ENH-20261024-pull-duration`.
