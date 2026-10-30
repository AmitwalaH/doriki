# DK-COLLECT-ENH-20261029-redact-token-ids: Redaction removes llama.cpp's tokenizer token ids, which are not secrets

**Date Discovered:** 2026-10-29
**Severity:** LOW
**Status:** CLOSED (2026-10-30), by `docs: README capture notes, close four tickets, file the DRA ticket`
**Label:** [ticket]
**Found by:** the lv4-fixture-canary agent, read in the first `doriki capture` bundle
**Filed by:** the lv4-fixture-canary agent

## Problem
A user captures a llama.cpp pod at `-lv 4`. In `container.log`, `print_info: BOS token = 1
'<s>'` becomes `BOS token = [redacted] '<s>'`, and `bad special token:
'tokenizer.ggml.seperator_token_id' = 4294967295` becomes `bad special token: '[redacted]' =
...`. The same happens in every report and `--include-events`, since `Sanitize` applies the same
rule. The `11889-lv4` fixture holds the redacted lines; no marker or fact reads them.

Cause: `manifest.RedactLine` (`internal/manifest/redact.go`) redacts the value after any
secret-named key, and `token` is on the `secretName` list
(DK-COLLECT-ENH-20261028-secret-keys-in-log-lines).

## Why it matters
Redacting too much is the safe side, and nothing is leaked. But a pack author reading the
bundle sees text the engine never printed, and a future marker on a tokenizer line would be
written against the redacted text.

## Options
1. Leave it, and say in the README's capture section that tokenizer lines lose their ids.
   Recommended until a pack needs those lines.
2. Do not redact a value that is only digits after a key ending in `token`; would need a check
   that no token-shaped secret is ever all digits.

## Related
`DK-COLLECT-ENH-20261028-secret-keys-in-log-lines`.

## Resolution
Option 1. The README's capture section says the value after a log key ending in `token` is
redacted on purpose (`BOS token = [redacted] '<s>'`), in the bundle and in a report's
`--include-events` lines, and that no parser pack reads those lines (checked in
`packs/llamacpp/*.yaml`, 2026-10-30). Revisit with option 2 when a pack needs a tokenizer line.
