# DK-CLI-BUG-20261026-select-on-full-line: A pack's log_regex past byte 512 of a line does not select the pack

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** CLOSED (2026-10-26)
**Label:** [ticket]
**Found by:** the M2 agent, read in code (METHOD.md Step 9 after slice 1)
**Filed by:** the M2 agent

## Problem
A pack is chosen by image first; among the packs whose `image_regex` matches, `log_regex` reads
the engine version to pick the one that covers it (`packs/doc.go`, `selectByVersion`). If the
line that says the version is long and the version sits past its first 512 bytes, it is not
read: with one matching pack the report carries a `pack-version-unknown` warning (measured,
`TestSelectionReadsFullLine` before the fix); with several version packs for one engine,
selection fails or cannot tell them apart.

(Corrected 2026-10-26: the first version of this ticket said the generic pack would be used.
That was read wrongly from the code; selection by `log_regex` happens only after the image
matched.)

Cause: `internal/cli/profile.go`, `logText` (line 122), hands `packs.Select` each line's `Raw`,
which the collector cuts to 512 bytes. Slice 1 of M2 made markers and facts read the whole line
(`model.Event.Line`); selection was outside that slice's approved files. Measured on the vLLM
0.32 fixture: its `log_regex` matches in the first 200 bytes, so it is not affected today.

## Why it matters
Same class as `DK-COLLECT-BUG-20261025-match-full-log-line`: a pack author tests a regex on the
real line, sees it match, and the pipeline does not.

## Options
1. Move enginelog's `fullLine` to a method on `model.Event` (one encoding of "the text a pack
   reads") and use it in both `enginelog` and `cli.logText`. Recommended: three small edits,
   one rule.
2. Copy the `Line`-else-`Raw` fallback into `cli`. Two encodings of one rule.

## Related
`DK-COLLECT-BUG-20261025-match-full-log-line` (closed); `DESIGN.md` section 5.3.

## Resolution (2026-10-26, M2)
Option 1. `model.Event.Text()` is the one rule for the text a pack reads (whole line, else
`Raw`); `cli.logText` uses it, so `log_regex` sees the whole line. Proved by
`TestSelectionReadsFullLine` (a vLLM version at byte 600 selects `vllm/0.32` with no
`pack-version-unknown` warning, through the real command). `enginelog` switches from its own
`fullLine` to `Text()` in slice 2, which touches that file anyway.
