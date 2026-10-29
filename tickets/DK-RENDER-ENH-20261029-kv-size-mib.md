# DK-RENDER-ENH-20261029-kv-size-mib: llama.cpp's KV cache size is in the JSON report but not on the terminal

**Date Discovered:** 2026-10-29
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the lv4-fixture-canary agent, read in code; the owner agreed to defer it
**Filed by:** the lv4-fixture-canary agent

## Problem
A user runs `doriki` on a llama.cpp pod at `-lv 4`. The kv-alloc row shows its duration and no
size, while vLLM's shows "1.0 GiB, 29,056 tokens". The size is in the JSON report, as the
phase's `mib` attribute (`1.69` in the `11889-lv4` fixture).

Cause: the terminal note (`attrs`, `internal/render/text/text.go:276`) shows only `gib` and
`tokens`. llama.cpp logs MiB, and DESIGN.md section 7.5 says sizes are GiB with one decimal,
which would print 1.69 MiB as "0.0 GiB".

## Why it matters
The size is what a user raises the log level to see; a real model's KV buffer is GiB-sized,
where GiB with one decimal reads fine.

## Options
1. Render `mib` as GiB with one decimal, per section 7.5, and below 0.1 GiB as "<0.1 GiB".
   Recommended: one rule for every size.
2. Render MiB under 1 GiB; a one-line DESIGN 7.5 change.

## Related
Decision 0010; `packs/llamacpp/11889.yaml`.
