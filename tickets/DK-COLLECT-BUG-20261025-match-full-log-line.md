# DK-COLLECT-BUG-20261025-match-full-log-line: A marker or fact past byte 512 of a log line is never seen

**Date Discovered:** 2026-10-25
**Severity:** MEDIUM
**Status:** OPEN, scheduled for M2 (live collector)
**Label:** [ticket]
**Found by:** the M1b agent, read in code while writing the vLLM pack test, then measured on the
vLLM 0.32 fixture
**Filed by:** the M1b agent

## Problem
A user profiles an engine that prints a long startup line, such as vLLM's full config dump, and
the fact or marker the pack looks for sits after the first 512 bytes. The report shows the phase
absent (or, after Ready, a `phase-unobserved` warning) and the manifest field missing, although
the line is in the log.

Cause: the collector cuts every event's text to 512 bytes when it creates the event
(`internal/collect/files/files.go`, `event`, `model.TruncateRaw`), and `enginelog` then matches
markers, facts and the in-line timestamp against that stored text
(`internal/collect/enginelog/enginelog.go`, `Apply` and `compiled.match`, all on `e.Raw`).
Measured on `packs/vllm/testdata/0.32/container.log`: one line is over 512 bytes (line 126, 3,376
bytes, the engine config); its matches (version, `log_regex`) fall in the first 200 bytes, so
this fixture is not affected. `packs/vllm_test.go` matches against truncated text on purpose so a
pack cannot pass a test that the pipeline would fail.

## Why it matters
`DESIGN.md` section 4.1 caps `Event.Raw` at 512 bytes as a storage rule for the report; it was
never meant to limit what packs can read. A pack author who tests a regex on the real line sees
it match and gets an absent phase in the report, with nothing pointing at the cause.

## Options
1. Keep the full line in the collector's in-memory event (or alongside it) until `enginelog` has
   matched it, and truncate only the `Raw` that is stored in the report. Recommended for M2,
   where the live collector reads logs anyway: matching sees the whole line, the report stays
   bounded.
2. Raise the cap. Moves the edge, does not remove it, and grows every report.

## Related
`DESIGN.md` sections 4.1 and 5.2; decision 0005 (`phase-unobserved`); the M1b vLLM pack.
