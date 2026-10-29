# DK-PHASE-BUG-20261029-gap-after-nested-phase: A phase inside a longer one can produce a gap the longer phase covers

**Date Discovered:** 2026-10-29
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the lv4-fixture-canary agent, read in code while weighing where kv-alloc sits in
llama.cpp's weight-load at `-lv 4`
**Filed by:** the lv4-fixture-canary agent, on the owner's instruction

## Problem
An engine fetches and loads at once, or a pack nests one phase inside another (DESIGN.md
section 4.2 allows overlapping phases). Say weight-load runs from 0 s to 30 s and kv-alloc from
5 s to 6 s, and server-ready starts at 30 s. The report lists a 24 s `gap` between kv-alloc and
server-ready, although weight-load covers that time, and the terminal draws it as unattributed.

Cause: `gaps` (`internal/phase/assign.go:441`) measures from `prev.End`, where `prev` is the
last complete phase in the fixed order, not the phase that ends latest so far.

No fixture shows it today: the `-lv 4` llama.cpp pack ends weight-load where kv-alloc starts,
so the two do not nest.

## Why it matters
A gap says "doriki does not know where this time went". Reported under a phase that visibly
covers it, it is false, and it would mislead exactly on the overlapping boots section 4.2
describes.

## Options
1. Measure each gap from the latest end among the complete phases so far, so time inside any
   earlier phase is never a gap. Test with a synthetic nested pair and the property test's
   "totals never exceed the span". Recommended.
2. Leave it; document that gaps are measured from the previous phase in the fixed order.

## Related
Decision 0010; `packs/llamacpp/11889.yaml` (kv-alloc at `-lv 4`).
