# DK-CLI-BUG-20261029-capture-source-command: A default-level bundle's SOURCE.md shows a command that cannot be run

**Date Discovered:** 2026-10-29
**Severity:** LOW
**Status:** CLOSED (2026-10-30), by `fix(cli): capture SOURCE.md names the target at the default level`
**Label:** [ticket]
**Found by:** the lv4-fixture-canary agent, read in the first `doriki capture` bundle
**Filed by:** the lv4-fixture-canary agent

## Problem
A user captures a pod at the default level and reads the bundle's `SOURCE.md`. Its Commands
block says `doriki capture the pod named on the command line --out <dir> --redact-level
default`, which is not a command anyone can paste.

Cause: `sourceMD` (`internal/cli/capture.go`) fills in the target only at `--redact-level
strict` (`c.target`, hashed); at the default level it falls back to the phrase.

## Why it matters
The block is there so the recipient can reproduce the bundle. The pod's name is already in
`pod.json` at the default level, so leaving it out hides nothing.

## Options
1. At the default level write the target as given (`pod/<name>`), and the namespace flag;
   strict keeps the hashed form. Recommended.
2. Write `pod/<name>` as a placeholder in the code block, and the phrase outside it.

## Related
M3 slice 5 (`capture`).

## Resolution
Option 1. At the default level the Commands block reads `doriki capture pod/<name> -n <ns>
--out <dir> --redact-level default`. The namespace is always written and comes from the pod
object, not the kubeconfig context, which belongs to the capturing machine (owner's answer at
the plan checkpoint, 2026-10-30). `--context` and `--kubeconfig` are not written. Strict keeps
the hashed target. Test: `TestCaptureSourceCommand`; `TestCaptureStrict` already checks that no
plain pod name is in a strict SOURCE.md.
