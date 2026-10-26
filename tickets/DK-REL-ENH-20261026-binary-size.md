# DK-REL-ENH-20261026-binary-size: The release binary grew from 10.8 MB to 30.3 MB with the live collector

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** OPEN
**Label:** [ticket]
**Found by:** the M2 agent, measured: `CGO_ENABLED=0 go build -trimpath -ldflags '-s -w'` of
`a0d47b4` (10,825,890 bytes) and of the slice 3b working tree (30,298,274 bytes), linux/amd64
**Filed by:** the M2 agent

## Problem
A user installing doriki from krew, Homebrew or GitHub Releases downloads a binary about three
times the size it was before live collection. The growth comes with `k8s.io/client-go` and
`k8s.io/cli-runtime`, which `DESIGN.md` section 8.1 chose; the share of each was not measured.
doriki uses `cli-runtime` only for `genericclioptions.ConfigFlags` (the `--kubeconfig`,
`--context`, `-n` and related flags) in `internal/cli/profile.go` and `internal/cli/live.go`.

## Why it matters
Size is not a correctness problem, and many kubectl plugins are larger. It slows installs and
CI image builds a little, and it is a number users notice in a release listing. Accepted for
v0.1 by the owner on 2026-10-26.

## Options
1. Measure first: build with the flags taken from `k8s.io/client-go/tools/clientcmd` directly
   (`clientcmd.BindOverrideFlags`) instead of `cli-runtime`, and compare. Switch only if the
   saving is worth a departure from `DESIGN.md` section 8.1, and record that as a decision.
   Recommended.
2. Keep `cli-runtime`, which also brings kubectl's exact flag set and resource parsing for later.

## Related
`DESIGN.md` section 8.1; M2 slice 3b.
