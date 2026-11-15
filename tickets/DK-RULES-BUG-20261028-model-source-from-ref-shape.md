# DK-RULES-BUG-20261028-model-source-from-ref-shape: fetch-remote-hub can tell a pod that already uses a mirror to use a mirror

**Date Discovered:** 2026-10-28
**Severity:** MEDIUM
**Status:** CLOSED (2026-11-15), by `fix(manifest): model.source reads HF_ENDPOINT, MODEL_ENDPOINT, HF_HUB_OFFLINE and VLLM_USE_MODELSCOPE` and `fix(rules): fetch-remote-hub names the mirror variable per engine`
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

## Resolution
Option 1, engine-aware. `ModelSource` still returns "" for a ref that is not Hub-shaped. For a
Hub id it reads `container.env_allowlist`, each variable only under the engine that reads it
(an unknown engine is checked for all), in this order, each read checked in the source at the
pinned version on 2026-11-15:
- offline, source "": `HF_HUB_OFFLINE`, or else `TRANSFORMERS_OFFLINE`, in huggingface_hub's
  truthy set (`1`, `on`, `yes`, `true`, any case; v1.31.0 `constants.py:12`, `:19`, `:192`),
  not under llama.cpp; `LLAMA_ARG_OFFLINE` in llama.cpp's set (`on`, `enabled`, `true`, `1`,
  case sensitive; b11889 `common/arg.cpp:1345-1346`, `:800`, `:3981-3986`) under llama.cpp.
- `VLLM_USE_MODELSCOPE` `1` or `true` (vLLM 0.32.0 `envs.py:734-735`; not `yes` or `on`),
  not under llama.cpp: source `modelscope.cn`.
- an endpoint: `MODEL_ENDPOINT` (llama.cpp and an unknown engine, b11889 `common/common.cpp:1557`) or else
  `HF_ENDPOINT` (huggingface_hub `constants.py:69`; llama.cpp's fallback at `:1560`): source
  is the URL's host, `huggingface.co` for the Hub or a subdomain, the value as given when it
  has no readable host.
- otherwise `huggingface.co`, as before.

The allowlist gained `MODEL_ENDPOINT` and `TRANSFORMERS_OFFLINE` as exact names. The rule's
trigger is unchanged, so a mirrored or offline pod no longer gets the advice, and its fix
names `HF_ENDPOINT` under vLLM, `MODEL_ENDPOINT` under llama.cpp, both under an unknown engine.
Known limits: a relative directory such as `models/opt-125m` still looks like a Hub id
(function comment and README); `HF_ENDPOINT=https://hf.co` reads as a mirror (README); only
literal `env` values in the pod spec are seen, not `envFrom`, `valueFrom` or the image's own
environment (collector, `internal/collect/files/manifest.go:67-69`; filed as
`DK-COLLECT-ENH-20261115-env-from-refs-unseen`).
