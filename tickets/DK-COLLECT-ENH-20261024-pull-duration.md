# DK-COLLECT-ENH-20261024-pull-duration: image-pull shows whole seconds when the kubelet knows the exact time

**Date Discovered:** 2026-10-24
**Severity:** MEDIUM
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M1a agent, measured on the kind-nginx fixture
**Filed by:** the M1a agent

## Problem
On `packs/generic/testdata/kind-nginx`, the report shows `image-pull` as 20 s. The kubelet's own
`Pulled` event in the same file says `Successfully pulled image "nginx:1.29.3-alpine" in
19.325s (19.325s including waiting)`.

Cause: `Pulling` and `Pulled` are Kubernetes events whose `firstTimestamp` has 1 s
resolution, and `internal/collect/files/files.go` (`k8sEvents`) uses only the timestamps. The
exact duration is only in the message text, kept in `Event.Raw`. `container-init` has the same
kind of error: `startedAt` is truncated to the second, so it can be up to 1 s too long.

## Why it matters
On small pulls the rounding is a large share (here 3.4%). The `pull-dominant` rule (M3)
compares shares against 35% and 40% thresholds, so a rounding error can tip the severity.

## Options
1. Parse `in (?P<seconds>[0-9.]+)s` from the `Pulled` message into an `Event.Attrs` value
   (`pull_seconds`) in the collector, and show it as an `image-pull` phase attribute. Keep the
   start and end from the event times so the bar still traces to events. Recommended.
2. Replace the phase duration with the parsed value. Makes start + seconds disagree with end.
3. Leave it and document 1 s resolution in the README.

## Related
`DESIGN.md` sections 4.1 (Attrs) and 6.2 (`pull-dominant`).
