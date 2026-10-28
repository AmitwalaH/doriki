# DK-COLLECT-ENH-20261028-secret-keys-in-log-lines: An API key vLLM echoes in its log stays in a capture bundle

**Date Discovered:** 2026-10-28
**Severity:** MEDIUM
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M3 agent, read in code and in the vLLM 0.32.0 fixture while writing `capture`
**Filed by:** the M3 agent

## Problem
A user starts vLLM with `--api-key my-internal-key` and runs `doriki capture pod/x`. The pod's
args are redacted (a secret-named flag), but vLLM prints its non-default arguments at startup
(the line the vLLM pack's model.ref fact reads: `non-default args: {..., 'model': 'facebook/opt-125m', ...}`),
so the log holds `'api_key': 'my-internal-key'`. The value has no known token shape, so
`manifest.RedactLine` leaves it, and it is in `container.log` and in `--include-events` output.

Cause: log-line redaction (`internal/manifest/redact.go`, `RedactLine`) matches token shapes
only; the secret-name rule (`secretName`) is applied to env names and flags, not to `key: value`
or `key=value` pairs inside a log line. Each bundle's SOURCE.md states this limit.

## Why it matters
`PROJECT.md` section 8: a leaked secret in a capture bundle is the most embarrassing failure.
Many deployments use plain API keys, not token-shaped ones.

## Options
1. In `RedactLine`, also redact the value after a secret-named key in the common shapes
   (`'key': 'v'`, `"key": "v"`, `key=v`, `--key v`), with the same `secretName` pattern; test
   on the vLLM fixture's real `non-default args` line with an `api_key` planted. Recommended.
2. Leave it documented in SOURCE.md and the README.

## Related
`DK-COLLECT-ENH-20261026-redact-log-lines` (closed: token shapes); M3 slice 5 (`capture`).
