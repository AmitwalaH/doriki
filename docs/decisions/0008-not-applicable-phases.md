# 0008: A pack may declare phases its engine never has; they are labelled, not just absent

**Date:** 2026-10-26 · **Decided by:** the owner · **Status:** decided, implemented 2026-10-26 (M2)

Some engines have no step for a phase in the fixed order. llama.cpp never runs torch.compile
and never captures CUDA graphs, and it does not say so. Under decision 0003 those phases are
already harmless: the pack declares no markers for them, so they are not expected, never make
a report incomplete, and decision 0005 never warns about them. What was wrong is the label. The
report listed them as `absent` ("not seen in the events or the log"), which reads as "it may
have happened and doriki missed it", the same words used for a phase that was expected and
not found.

A pack may now declare `not_applicable: [phase, ...]`. A listed phase the log does not show is
reported as `skipped` with the attribute `not_applicable: "true"`, and the terminal shows it
on its own line, "skipped: compile, graph-capture (not applicable to this engine)", apart from
"skipped: ... (the engine said it skips them)" and from "absent: ... (not seen in the events
or the log)". This is a label distinction only: `phase.Expected`, the exit code and the
warnings are unchanged. If the log does show a listed phase, it is reported as observed;
`packs/notes_test.go` rejects a pack that lists a phase its own markers can produce. The field
is in the pack schema (`schema/pack.v1.json`); the first pack to use it is `llamacpp/11889`,
for compile and graph-capture.

Revisit if an engine gains one of its not-applicable steps in a later version (the pack for
that version drops the entry), or if users read "skipped" as something the engine chose rather
than something it does not have.
