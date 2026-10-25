# 0006: An engine-init phase between container-init and weight-fetch

**Date:** 2026-10-25 · **Decided by:** the owner · **Status:** decided, implemented 2026-10-25 (M1b)

The vLLM 0.32 capture showed `container-init` ending 0.37 s after the container started, at the
first log line, and then 72 s attributed to nothing before `weight-fetch`: the engine importing
its libraries and configuring itself. A gap that large reads as a hole in the waterfall rather
than as the engine's own start-up, which is often where a slow image or a slow node shows. From
now on, a phase `engine-init` sits between `container-init` and `weight-fetch`. It starts at the
first engine log line, exactly where `container-init` ends, and ends at the pack's marker with
the suffix `.engine.init.end` (for vLLM, "Starting to load model"). Packs declare only the end:
the first log line is often not the engine's own (the vLLM fixture's is the image's entrypoint
banner), so no pack-specific regex could mark it. A pack without that marker, such as
`generic`, cannot produce the phase, so it is `absent` and does not count toward the exit code
(decision 0003). Phase names stay append-only; `engine-init` is a new name in the fixed order,
in `internal/model/phase.go`, and DESIGN.md section 4.2's table has its row.

Adding a phase adds one row, `{"name": "engine-init", "state": "absent"}`, to every report under
a pack that cannot produce it, including the two frozen nginx fixtures' `expected.json`. That
re-record is a value-free addition (4 lines added, none removed or changed in each file), made
by owner approval in the same commit as this decision rather than in a commit of its own as
METHOD.md Step 11 would have it.

Revisit if an engine logs nothing before it starts loading weights, since `engine-init` would
then run from the first weight line to itself, or if two engines need the phase to start at a
line of their own.
