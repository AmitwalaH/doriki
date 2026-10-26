# DK-PACK-ENH-20261026-llamacpp-verbose: llama.cpp at -lv 4 shows its build, tensor load and KV cache, but the pack cannot use them yet

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M2 agent, measured: a throwaway llama-server pod with `-lv 4` on the kind
cluster, same image as the `llamacpp/11889` fixture
**Filed by:** the M2 agent, on the owner's instruction

## Problem
A user runs llama-server with `-lv 4` (or higher) to get more detail. Its log then says the
build (`common_params_print_info: build 11889 (d854d98ba)`), the tensor load
(`load_tensors: loading model tensors ...`, the model buffer sizes) and the KV cache
(`llama_kv_cache: CPU KV buffer size = 1.69 MiB`). doriki's report still shows kv-alloc as
absent, weight-load as one block from "loading model" to "model loaded", and the manifest has
no engine version, because the pack (`packs/llamacpp/11889.yaml`) was written from the
default-level capture, which prints none of those lines. Only `log_regex` reads the build
line, for pack selection.

## Why it matters
Users debugging a slow load are the ones who raise the log level; for them the waterfall could
split weight-load into tensor load and KV allocation, and the manifest could carry the build.

## Options
1. Capture a second real fixture at `-lv 4` (`packs/llamacpp/testdata/11889-lv4/`), and add
   markers for `load_tensors`, the KV buffer line and an `engine.version` fact, each tested
   against it, with the default-level fixture still passing. Acceptance: the second fixture
   is a real capture, every new marker matches a line in it and none in the default-level
   one changes its phases. Recommended.
2. Leave the pack at the default level and document that `-lv 4` adds nothing to the report.

## Related
`packs/llamacpp/11889.yaml`; its `SOURCE.md` (the `-lv 4` comparison); decision 0003.
