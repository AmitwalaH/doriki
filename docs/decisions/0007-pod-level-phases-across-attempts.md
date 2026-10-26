# 0007: Pod-level phases are shown on every boot attempt

**Date:** 2026-10-26 · **Decided by:** the owner · **Status:** decided, implemented 2026-10-26 (M2)

A pod whose container restarts has several boot attempts (`DESIGN.md` section 6.1 step 7), and
the report shows the latest by default. Scheduling, and node provisioning when a node was made
for the pod, happen once per pod, not once per container start. Before this decision a later
attempt showed `scheduling` as `absent`, which under decision 0003 made every restarted pod
exit 1 even when the restart itself booted cleanly.

Pod-level phases (`scheduling`, `node-provision`) belong to the pod and are shown on every
attempt, with the same times. Attempt-scoped phases (`image-pull` onward) start at the attempt
boundary: the previous container's end (`lastState.terminated.finishedAt`), or the attempt's
own start when that is unknown (`DK-PHASE-BUG-20261024-attempt-boundary-repull`). Totals are
measured from pod creation, so `pending_to_ready` on a later attempt includes the earlier
attempts. The list of pod-level phases lives in `internal/phase/assign.go` (`podLevel`).

Options considered: excuse pod-level phases from the exit code on later attempts while still
showing them absent (rejected: "absent" reads as "not observed" for something that was), or a
new phase state (rejected: a schema and renderer change for one case). Revisit if a phase is
found that happens once per pod but is not first in the fixed order, or if users read the
pod-level rows on a later attempt as time that attempt spent.
