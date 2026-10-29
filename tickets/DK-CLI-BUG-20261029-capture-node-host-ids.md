# DK-CLI-BUG-20261029-capture-node-host-ids: A capture bundle keeps the node's machine ID, system UUID and boot ID

**Date Discovered:** 2026-10-29
**Severity:** MEDIUM
**Status:** CLOSED (2026-10-29), by `fix(cli): capture drops the node's machine, system and boot IDs`
**Label:** [decision], answered by the owner on 2026-10-29: option 1
**Found by:** the lv4-fixture-canary agent, read in the first `doriki capture` bundle
(`packs/llamacpp/testdata/11889-lv4/node.json`)
**Filed by:** the lv4-fixture-canary agent

## Problem
A user captures a pod at the default redaction level and shares the bundle. Its `node.json`
holds `status.nodeInfo.machineID`, `systemUUID` and `bootID`. On a cloud node the system UUID
identifies the instance (on EC2 it is the instance's UUID); on the kind fixture it is the WSL2
VM's. Nothing doriki reads needs them: the manifest takes the kubelet version, and the labels
and capacity, from the node.

Cause: `redactObjects` (`internal/cli/capture.go`) copies `o.Node.Status.NodeInfo` whole. Its
SOURCE.md text says what it keeps ("name, labels, kubelet and system info, and capacity"), so it
is documented, but "system info" reads as OS and versions, not identifiers.

## Why it matters
`PROJECT.md` section 8: a bundle is made to be shared. A stable host identifier in it is the
kind of thing a security reviewer flags, and a fixture is never edited once committed.

## Options
1. Clear `machineID`, `systemUUID` and `bootID` from the node at every redaction level, with a
   test on a node carrying all three; re-capture `11889-lv4` before it is committed (about 5
   minutes on kind). Recommended.
2. Clear them only at `--redact-level strict`; commit the fixture as captured.
3. Keep them, and say so in the README's list of what `capture` keeps.

## Related
`packs/llamacpp/testdata/11889-lv4/`; `DK-COLLECT-ENH-20261026-redact-log-lines`.

## Resolution
Option 1. `redactObjects` (`internal/cli/capture.go`) clears `machineID`, `systemUUID` and
`bootID` from the node's system info at both redaction levels; the kubelet version and the rest
of the system info stay. `TestCaptureDropsNodeHostIDs` plants all three and checks none is in
`node.json` at either level. The bundle's SOURCE.md and the README say so. The `11889-lv4`
fixture was recaptured with the fixed binary before it was committed.
