# DK-COLLECT-ENH-20261115-env-from-refs-unseen: a mirror set through envFrom or valueFrom is invisible, so fetch-remote-hub fires on a mirrored pod

**Date Discovered:** 2026-11-15
**Severity:** MEDIUM
**Status:** OPEN
**Label:** [ticket]
**Found by:** the model-source task agent, read in code
**Filed by:** the model-source task agent

## Problem
A user puts `HF_ENDPOINT=http://hf-mirror.internal` in a ConfigMap and loads it into the vLLM
container with `envFrom: [{configMapRef: {name: hf-env}}]`, or with
`valueFrom: {configMapKeyRef: ...}` on one variable. The download is still a large part of the
boot, and doriki reports the model's source as huggingface.co and advises "serve it from a
cluster-local Hub mirror (HF_ENDPOINT)", the thing they already do. The same happens when the
variable is set by the image's own `ENV`.

Cause: the collector captures only literal values, `for _, env := range c.Env { add(...,
env.Value, ...) }` (`internal/collect/files/manifest.go:67-69`). `envFrom` is not read at all;
a `valueFrom` variable is captured with an empty value, which `manifest.ModelSource` treats as
unset. The image's environment is not in the pod object. `ModelSource` can only read what the
allowlist captured (`internal/manifest/manifest.go`).

## Why it matters
`PROJECT.md` section 8: advice naming a fix the user already applied is the same kind of wrong
as advice naming a fix the engine does not support. A ConfigMap shared by many model pods is
one way to set a mirror for all of them; how often it is used is not measured.

## Options
1. Resolve `configMapRef` and `configMapKeyRef` with a read-only `get configmaps` in the
   collector, through the same allowlist and redaction; never read Secrets (a `secretRef` or
   `secretKeyRef` stays unresolved). Adds one RBAC verb. Recommended for v0.2.
2. When the container has any `envFrom`, or an allowlisted `valueFrom` variable, add a warning
   ("environment from a ConfigMap or Secret was not read; model.source may be wrong") and keep
   the advice. No new verb; honest, but the advice still fires.
3. Document it as a limit in the README only.

## Related
`DK-RULES-BUG-20261028-model-source-from-ref-shape` (closed, which records this limit);
`internal/rules/fetch_remote_hub.go`.
