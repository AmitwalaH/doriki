# DK-COLLECT-ENH-20261026-redact-log-lines: A token printed in an engine log is shown by --include-events

**Date Discovered:** 2026-10-26
**Severity:** MEDIUM
**Status:** OPEN, scheduled for M3 (with `capture`)
**Label:** [ticket]
**Found by:** the M2 agent, read in code while adding env redaction (slice 3a)
**Filed by:** the M2 agent

## Problem
A user profiles a pod whose engine prints a secret in its startup log, for example a server
that echoes its command line with `--api-key sk-…`, or a library that logs a Hugging Face URL
with a token. They run `doriki pod/x -o json --include-events` and attach the report to a
ticket. The token is in the report, in the `raw` of that log line.

Cause: `manifest.Sanitize` (`internal/manifest/redact.go`) redacts only the collector's env and
args facts. Engine log lines pass through unchanged, so `Event.Raw` (and the in-memory
`Event.Line`) keep whatever the engine printed, and `cli` writes them with `--include-events`.
The waterfall and the manifest are not affected: they never print log text.

## Why it matters
`PROJECT.md` section 8 names a leaked token in a capture bundle as the thing that would
embarrass us most, and `--include-events` output is shared the same way. `DESIGN.md` section
10.3 promises redacted `capture` bundles; the same rule should hold for any output that carries
log text.

## Options
1. In `Sanitize`, replace `secretValue` matches in every engine log line's `Raw` and `Line`
   before packs read them, the same patterns as env (one encoding), with a test that a marker
   regex still matches a line once a token in it is redacted. Recommended, together with
   `capture`, which needs the same rule for the files it writes.
2. Redact only at output time, in the JSON renderer. The pure renderer would then hold the
   secret patterns too: two encodings.

## Related
`DESIGN.md` sections 4.3 and 10.3; M2 slice 3a (env and args redaction); M3 `capture`.
