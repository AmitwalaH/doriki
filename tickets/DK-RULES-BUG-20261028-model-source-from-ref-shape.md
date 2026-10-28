# DK-RULES-BUG-20261028-model-source-from-ref-shape: fetch-remote-hub can tell a pod that already uses a mirror to use a mirror

**Date Discovered:** 2026-10-28
**Severity:** MEDIUM
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M3 agent, read in code and in the vLLM 0.32.0 wheel while writing slice 2
**Filed by:** the M3 agent

## Problem
A user runs vLLM with `HF_ENDPOINT=http://hf-mirror.internal` (a cluster-local mirror) and a
model ref `org/name`. The download is slow anyway, weight-fetch is 30% of the boot, and doriki
advises "serve it from a cluster-local Hub mirror (HF_ENDPOINT)", the thing they already do,
and says the weights came from huggingface.co, which is false.

Cause: `manifest.ModelSource` (`internal/manifest/manifest.go`) decides the source from the
shape of `model.ref` alone: `org/name` is huggingface.co. It does not read the env allowlist,
which already captures `HF_ENDPOINT` (and `VLLM_USE_MODELSCOPE`, which switches vLLM to
ModelScope: `vllm/model_executor/model_loader/weight_utils.py:167`). Two smaller cases share
the cause: a relative local directory such as `models/opt-125m` has the same shape as a Hub id,
and `HF_HUB_OFFLINE=1` means nothing is downloaded at all.

## Why it matters
`PROJECT.md` section 8 names "advice naming a fix the engine does not support" as an
embarrassment; advice naming a fix the user already applied is the same kind of wrong. No
current fixture is affected: neither sets these variables, and neither crosses 25%.

## Options
1. In `ModelSource`, read `container.env_allowlist`: `HF_ENDPOINT` set means the source is its
   host; `VLLM_USE_MODELSCOPE` true or `HF_HUB_OFFLINE=1` means no Hub source. Recommended:
   about ten lines, one place, tested with minimal manifests. The relative-path case stays a
   documented limit (vLLM checks the filesystem, which doriki cannot see).
2. Leave it to v0.2 with a README note.

## Related
`internal/rules/fetch_remote_hub.go`; M3 task acceptance 2 (`model.source`).
