# DK-CLI-ENH-20261028-probe: first-token is absent for every engine that does not log a request

**Date Discovered:** 2026-10-28
**Severity:** LOW
**Status:** OPEN, v0.2
**Label:** [ticket]
**Found by:** the M3 task (acceptance 10), decision 0009
**Filed by:** the M3 agent, on the owner's instruction

## Problem
A user profiles a vLLM or llama.cpp pod that has booted. The waterfall lists `first-token` under
"absent", because neither engine logs a first request on its own, and doriki v0.1 has no
`--probe` flag to send one, although DESIGN.md section 7.2 lists it.

## Why it matters
Pending-to-first-token is the number an operator cares about most after Ready; without it the
report stops at server-ready.

## Options
1. Implement `--probe` as decision 0009 states: one `POST /v1/completions` to the pod's service
   with `{"model": manifest.model.ref, "prompt": "Hello", "max_tokens": 1}`, only with the flag,
   once, refused without a model ref; first-token from the response time. Tests against a fake
   server; an e2e case on the kind llama.cpp pod. Recommended.
2. Leave first-token to engines that log it.

## Related
Decision 0009; DESIGN.md sections 4.2, 7.2, 10.3 and 12.2.
