# DK-REL-BUG-20261026-git-gate-stash-list: The git-gate hook blocks the read-only `git stash list`

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** OPEN, assigned to the owner
**Label:** [ticket]
**Found by:** the M2 agent, walked: the hook blocked the command during the METHOD.md section 2
branch check
**Filed by:** the M2 agent

## Problem
At the start of a session an agent checks the branch state with read-only commands, as
METHOD.md section 2 step 3 asks. It ran `git status`, `git branch`, `git log` and `git stash list`.
The hook stopped the whole command with:

```
git-gate: BLOCKED. `git stash` moves or discards work; METHOD.md rule 1 allows read-only
version control only. Ask the owner to run it.
```

`git stash list` (and `git stash show`) only read. The hook treats every `git stash` as moving
work. The agent did not look for stashes another way, so whether uncommitted work sits in a
stash stays unknown to it.

## Why it matters
METHOD.md rule 1 warns that there may be uncommitted work the agent cannot see. A stash is one
place that work hides, and the read-only way to see it is blocked. Each session either skips the
check or asks the owner.

## Options
1. Allow `git stash list` and `git stash show` in `.claude/hooks/git-gate.py`, keep every other
   `stash` subcommand blocked, and add both to `git-gate_test.sh`. Recommended. The hook is a
   judge, so the owner makes this change.
2. Leave it, and have the owner report stashes in the task file's handoff.

## Related
`.claude/hooks/git-gate.py`; `.claude/hooks/git-gate_test.sh`; PR #4.
