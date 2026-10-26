# DK-CLI-BUG-20261026-select-on-full-line: A pack's log_regex past byte 512 of a line does not select the pack

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M2 agent, read in code (METHOD.md Step 9 after slice 1)
**Filed by:** the M2 agent

## Problem
A user profiles an engine whose image does not name the engine, so the pack is chosen by its
`log_regex`. If the line that identifies the engine is long and the matching text sits past its
first 512 bytes, no pack matches, the `generic` pack is used, and every engine phase is absent.

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
