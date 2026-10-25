# DK-DOCS-ENH-20261025-doc-drift-check: The design docs name packages, flags and paths that do not exist

**Date Discovered:** 2026-10-25
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the owner, during M1a review (five one-line DESIGN.md corrections); one more case
measured by the M1b agent with `ls` and `grep`
**Filed by:** the M1b agent

## Problem
A contributor follows the run command in `PROJECT.md` section 3 or `CLAUDE.md`:
`go run ./cmd/doriki --from-files packs/vllm/testdata/0.28/{pod.json,events.json,container.log}`.
The directory does not exist (`ls packs/vllm`: no such file or directory, measured 2026-10-25 at
`21b8157`), and the vLLM fixture M1b captures will be 0.32, not 0.28. Nothing in CI notices.

Cause: `DESIGN.md` and `PROJECT.md` name Go packages, CLI flags, make targets and file paths in
backticks, and nothing checks those names against the repo. During M1a `DESIGN.md` needed five
one-line corrections as the code diverged from it; each was found by a reader, not a check.

## Why it matters
The docs are what an agent or a new contributor reads first. A wrong path or flag sends them
down a dead end, and an agent may build on a name that never existed.

## Options
1. A CI check (a small Go test or script) that extracts backticked tokens shaped like a repo path,
   a `--flag`, a `make` target or an `internal/` package from `DESIGN.md` and `PROJECT.md`, and
   fails when one does not exist, with an explicit allowlist for illustrative names (such as the
   example output in `DESIGN.md` section 2). Recommended, after v0.1: mechanical, and the
   allowlist makes every exception a reviewed decision.
2. A review checklist item. No tooling, but relies on the reader who already missed five.

## Related
The M1a task's DESIGN.md corrections; M1b question Q1 (vLLM 0.32 paths); `PROJECT.md`
section 6 (one encoding per fact).
