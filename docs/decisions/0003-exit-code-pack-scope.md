# 0003: Exit code 1 counts only phases the selected pack can produce

**Date:** 2026-10-24 · **Decided by:** the owner · **Status:** decided

`DESIGN.md` section 7.3 gave exit 1 to any report with an `open` or `absent` phase. Under the
`generic` pack every engine phase is absent by design, so every run on an engine without a pack
would exit 1, and a CI job could not tell "still booting" from "no pack for this engine". From
now on, a phase counts toward exit 1 only if the selected pack can produce it: the pack declares
the markers its start and end need, and the phase is not optional (`node-provision` only happens
when a node is created for the pod). The report still lists every other phase as `absent`. The
rule lives in one place, `phase.Expected` in `internal/phase/assign.go`, next to the phase
table it reads; section 7.3 says so in one line. Revisit if a pack declares a marker its engine
does not always log (for example a first-token marker that needs a request), because that phase
would then hold a booted pod at exit 1.
