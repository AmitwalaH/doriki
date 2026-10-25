# DK-PHASE-ENH-20261024-cached-image-pull: A pod whose image is already on the node exits 1 as if still booting

**Date Discovered:** 2026-10-24
**Severity:** MEDIUM
**Status:** CLOSED (2026-10-25)
**Label:** [decision]
**Found by:** the M1a agent, read in code while writing the exit-code rule (decision 0003)
**Filed by:** the M1a agent

## Problem
A user profiles a pod that started on a node that already had its image. The kubelet logs no
`Pulling` event, only `Pulled` with "Container image ... already present on machine". The
report shows `image-pull` as `absent`, and once exit codes are wired (DESIGN.md section 7.3,
decision 0003) the command exits 1, "pod still booting, or events expired", for a pod that
booted fine. A CI job gating on exit 0 fails on every warm node.

Cause: `internal/phase/assign.go` (`specs`, image-pull) starts the phase only at `Pulling`, and
`phase.Expected` counts `image-pull` because every pack can produce it. Read in code; not yet
seen in a capture (the kind-nginx fixture pulled for real).

## Why it matters
Warm nodes are the common case in production. Exit 1 there makes the exit code useless as a
gate, and `absent` reads as "we could not see it" when the kubelet said exactly what happened.

## Options
1. When `Pulled` says the image was already present and there is no `Pulling`, report
   `image-pull` as `complete` with 0 s, start and end at that event, and an attribute
   `cached: true`. Recommended: true to what the kubelet reported, traces to one event, and
   the `pull-dominant` rule (M3) needs "image not previously on node" anyway.
2. Keep `absent`, and make `phase.Expected` skip `image-pull` when such a `Pulled` exists.
   Fixes the exit code but keeps a misleading `absent`.
3. Leave it until M3.

## Decision
Option 1, recorded in `docs/decisions/0004-cached-image-pull.md` (2026-10-24). To be
implemented in M1b together with a real warm-node capture; this ticket stays open until then.

## Related
Decisions 0003 and 0004; `DESIGN.md` sections 4.2, 6.2 (`pull-dominant`) and 7.3.

## Resolution (2026-10-25)
Implemented in M1b with a real warm-node capture, `packs/generic/testdata/kind-nginx-warm/`.
The collector tags a `Pulled` event whose message contains "already present on machine" with
`cached: true` (`internal/collect/files/files.go`, `cachedPullText`); the phase engine accepts
that event as image-pull's start (`internal/phase/assign.go`, `altStarts`) and copies `cached`
onto the phase. Measured on the capture: before, `image-pull` absent and exit 1; after,
complete at 0 s with `cached: true` and exit 0. Kubelet 1.37 words it "... already present on
machine and can be accessed by the pod"; the match is on the shared part.
