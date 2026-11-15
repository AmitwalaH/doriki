# 0011: Engines that load the model after Ready are shown in v0.2, as a first-request block

**Date:** 2026-11-15 · **Decided by:** the owner · **Status:** decided, to be implemented in v0.2

Some engines become Ready before they load a model and load it on the first request; Ollama
does this by default. In the fixed phase order (`DESIGN.md` section 6.1) `weight-fetch` and
`weight-load` come before `server-ready`, so weight markers logged after Ready cannot be
placed, and decision 0005 excuses absent engine phases once the pod is Ready. The report ends
at Ready and the first request's load time is not in it
(`tickets/DK-PHASE-ENH-20261026-lazy-load-phase-order.md`).

The ticket's option 1 is chosen: weight phases observed after Ready are reported in a second,
request-scoped block after `server-ready` ("first request"), ending at `first-token`. The boot
keeps the one fixed order that the renderers, the report schema and `diff` rely on. It is built
together with `--probe` (decision 0009), whose single request can trigger the load, and like
`--probe` it lands in v0.2, not v0.1. The `ollama` pack waits for it.

Two alternatives were weighed: letting a pack declare its engine lazy and reordering the phases
for it, which breaks the one fixed order; and not supporting lazy-loading engines at all, which
leaves out the cold-start cost these users came to see.

Until then, v0.1 says so: the README's "What is validated" states that engines which load the
model after Ready are not shown and the report ends at Ready.

Revisit if a lazy-loading engine logs its load in a way that overlaps the boot phases, or if
`--probe` does not land in v0.2, since the first-request block then has no end for an engine
that logs no first token (`tickets/DK-CLI-ENH-20261028-probe.md`).
