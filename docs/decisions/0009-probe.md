# 0009: `--probe` sends one POST /v1/completions with max_tokens 1, and only when asked

**Date:** 2026-10-28 · **Decided by:** the owner · **Status:** decided; implementation in v0.2
(`DK-CLI-ENH-20261028-probe`)

`first-token` ends at the first token a pod serves (DESIGN.md section 4.2). An engine that logs
no request of its own leaves it absent, so DESIGN.md section 7.2 gives doriki a `--probe` flag
that sends one tiny completion request and measures it; section 12.2 left open which request.

The probe is a single `POST /v1/completions` to the pod's own service, with the body
`{"model": <manifest.model.ref>, "prompt": "Hello", "max_tokens": 1}`. `/v1/completions` and
not `/v1/chat/completions`, because it needs no chat template, which a base model may not have;
the model is the one the manifest already read from the engine log, so the request names what
is loaded; one token is the least work that still yields a first token.

It is sent only when `--probe` is given, never by default, never more than once per run, and
never by `capture` or `pack test`: it touches the workload, the one exception to "doriki only
reads" (DESIGN.md section 10.3). Without a model ref in the manifest, `--probe` is refused with
a message rather than guessing a model.

Implementation is v0.2; v0.1 ships without the flag. Revisit if an engine serves only the chat
endpoint, or if a pack's log gives first-token without a request.
