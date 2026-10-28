# DK-COLLECT-ENH-20261028-secret-keys-in-log-lines: An API key vLLM echoes in its log stays in a capture bundle

**Date Discovered:** 2026-10-28
**Severity:** HIGH (raised by the owner on 2026-10-28)
**Status:** CLOSED (2026-10-28)
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

## Resolution
Option 1. `manifest.RedactLine` (`internal/manifest/redact.go`), which `Sanitize` applies to
every engine log line and `capture` to its log files, also redacts the value after any key the
`secretName` list matches, the list env names use, in four shapes: a quoted value (`'api_key':
'x'`, `"password": "x"`), `key=value` and `key: value` (with a Bearer or Basic scheme taken
along), and `--flag value`. `TestRedactLineUsesSecretName` swaps the list for a sentinel and
proves no second list is in use. `TestEchoedAPIKeyRedacted` plants `'api_key': 'plainword-7f3a'`
in the fixture's real `non-default args` line: the value is absent from `-o json
--include-events` and `model.ref` (read from the same line) is still `facebook/opt-125m`; with
the key rule switched off it fails ("the echoed API key is in the report").

Measured cost, 2026-10-28, all five fixture logs: the rule also redacts the values of
tokenizer settings (vLLM `tokenizer`, `Tokenizer`, `tokenizer_mode`, `tokenizer_revision`,
`skip_tokenizer_init`, `tokens`; one llama.cpp `token` key) in `--include-events` output and
bundles. No marker or fact line is affected; phases, manifests and goldens are unchanged.

**Changed 2026-10-28, on the owner's instruction:** secret names now match whole words only.
`secretName` is a function over one list, `secretWords` (token, secret, password, passwd, key,
apikey, credential, credentials, auth, authorization), and a name says secret when one of its
words is in it; words split at anything but letters and digits and at camelCase. It applies to
env names, flags and log-line keys alike. `tokenizer`, `tokenizer_mode` and `max_tokens` keep
their values (negative cases in `TestRedactLineSecretKeys` and `TestSecretName`); `accessToken`,
`apikey` and `Authorization` are still redacted. Re-measured on all five fixture logs: the vLLM
tokenizer settings are no longer redacted; two llama.cpp warnings still are (`bad special token:
'tokenizer.ggml.padding_token_id'`, a real `token: '<value>'` shape). Env names that held a
secret word only inside a longer word (`VLLM_KEYFILE`) are no longer redacted by name; a
token-shaped value in them still is.
