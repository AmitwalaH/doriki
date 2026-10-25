# DK-PACK-ENH-20261024-per-line-skew: An engine log clock running ahead shows up as container-init time

**Date Discovered:** 2026-10-24
**Severity:** MEDIUM
**Status:** CLOSED (2026-10-25)
**Label:** [ticket]
**Found by:** the M1a agent, read in `DESIGN.md` section 4.1 while planning clock-skew correction
**Filed by:** the M1a agent

## Problem
A user profiles a vLLM pod whose engine writes its own timestamps (`INFO 10-24 18:02:11`) from a
clock 60 s ahead of the node. The waterfall shows `container-init` as about 60 s longer than it
really was, and no `clock-skew` warning appears.

Cause: `internal/normalize/normalize.go` (`engineLogSkew`) has only one anchor, the container
`started` time. A first log line before `started` is impossible, so that direction is corrected;
a first line after `started` is real container-init time, so a clock running ahead cannot be
told apart from it. This was the approved option A for M1a; `DESIGN.md` section 4.1 says so.

## Why it matters
`container-init` is the phase a user reads as "the engine took long to start". A wrong number
there points them at the wrong fix, and the report gives no hint that the clock is off.

## Options
1. When a pack parses an in-line engine timestamp (M1b, `timestamp:` in the pack schema), keep
   both that time and the runtime's `kubectl logs --timestamps` time on the event, and measure
   skew per line as their difference (median over the first lines). Both directions are then
   detectable. Recommended: it needs no new data source, only the two clocks the log already
   carries.
2. Keep option A only and document the limit in the README.

## Related
The M1a plan, question Q1 (task files are not committed); `DESIGN.md` sections 4.1 and 5.2; the M1b vLLM pack.

## Resolution (2026-10-25)
Option 1, implemented in M1b in `internal/collect/enginelog` (`Apply`). With runtime
timestamps (`kubectl logs --timestamps`) the runtime time is every event's time, so a fast
engine clock no longer stretches `container-init`; the engine's own in-line time is read with
the pack's `timestamp` regex and compared line by line, and a difference over 2 s on any line
is a `log-inline-skew` warning. Without runtime timestamps the in-line time is used, with the
year from the container start, and a `log-inline-time` warning says so. Measured on the vLLM
0.32 fixture: 212 lines carry an in-line time, the largest difference is 0.994 s (the 1 s
resolution of the in-line time), so no warning. The maximum was chosen over the median the
ticket suggested so a clock that jumps partway through is still reported.
