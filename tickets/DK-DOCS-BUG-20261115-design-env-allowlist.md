# DK-DOCS-BUG-20261115-design-env-allowlist: DESIGN.md lists a smaller env allowlist than doriki captures

**Date Discovered:** 2026-11-15
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the model-source task agent, read in code and docs
**Filed by:** the model-source task agent

## Problem
`DESIGN.md` section 4.3 (line 151) says env capture is allowlist-only and lists `VLLM_*`,
`HF_*`, `SGLANG_*`, `OLLAMA_*`, `CUDA_*`, `NCCL_*`. The allowlist in
`internal/manifest/allowlist.go` also holds `LLAMA_*` (before this task) and, since the
model-source fix, the exact names `MODEL_ENDPOINT` and `TRANSFORMERS_OFFLINE`. The README's
capture section lists all of them; DESIGN does not.

## Why it matters
DESIGN is the product source of truth. A reader checking what a capture bundle may contain
(a privacy question) gets an incomplete list from it. `allowlist.go` is the one encoding
(`PROJECT.md` section 6), so the list in DESIGN is a copy that has drifted.

## Options
1. Replace the list in DESIGN 4.3 with a pointer to `internal/manifest/allowlist.go` and the
   README's capture section, keeping the rule ("allowlist-only, secrets redacted inside it").
   Recommended: one encoding, no further drift.
2. Bring the list in DESIGN up to date by hand, and update it with each allowlist change.

## Related
`DK-RULES-BUG-20261028-model-source-from-ref-shape` (closed); `README.md` "capture" section.
