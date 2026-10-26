# DK-PHASE-ENH-20261026-lazy-load-phase-order: An engine that loads its model after Ready cannot be shown in the fixed phase order

**Date Discovered:** 2026-10-26
**Severity:** MEDIUM
**Status:** OPEN, v0.2
**Label:** [decision]
**Found by:** the M2 agent, read in code while planning the Ollama pack
**Filed by:** the M2 agent

## Problem
A user profiles a pod running an engine that becomes Ready before it loads any model, and loads
the model on the first request (Ollama does this; other servers can be configured to). The
waterfall shows `server-ready` complete and the weight phases absent. Then a request arrives,
the engine spends real time fetching and loading the model, and none of it is in the report.

Cause: `DESIGN.md` section 6.1 step 3 assigns phases in one fixed order, each starting at or
after the previous one's start (`internal/phase/assign.go`, `assignAttempt`, the `cursor`). In
that order `weight-fetch` and `weight-load` come before `server-ready`. For a lazy-loading
engine, the weight markers come after Ready, so `server-ready` (found later in the order, but
earlier in time) cannot start after them, and one side is marked absent. Decision 0005 also
excuses absent engine phases once the pod is Ready, so the run exits 0 with the load unseen.

## Why it matters
For these engines the model load is the cold-start cost a user is trying to see. A report that
says the pod was ready in seconds, when the first request then waited a minute, is misleading.

## Options
1. A request-scoped stage after `server-ready`: weight phases observed after Ready are reported
   in a second block ("first request"), ending at `first-token`. Keeps the fixed order for the
   boot and makes the lazy load visible. Recommended, together with `--probe` (M3), which can
   trigger the load.
2. Let a pack declare its engine "lazy" and reorder phases for it. Breaks the one fixed order
   that renderers, the schema and `diff` rely on.
3. Do not support lazy-loading engines: document that the report ends at Ready.

## Related
M2 task acceptance 8 (Ollama, cut from M2 on 2026-10-26 by the owner); `DESIGN.md` sections
4.2, 5.5 and 6.1; decision 0005; the open `--probe` decision in `PROJECT.md` section 5.
