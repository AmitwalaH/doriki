# DK-REL-BUG-20261026-git-gate-stash-list: The git-gate hook blocks the read-only `git stash list`

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** CLOSED (2026-10-26)
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

## Update (2026-10-26, M2 agent, walked): a second false positive
With four files staged by `git add` and nothing else in the index, the agent committed with the
message on stdin: `git commit -q -F -` followed by a quoted heredoc holding the message. The
hook blocked it: "`git commit <path>` commits files that were not staged with `git add`; stage
them first and commit without paths: <<EOF". The command names no path: `-F -` reads stdin,
and the heredoc is that stdin. The hook appears to take the heredoc marker as a path. Under
CLAUDE.md the agent stopped and did not retry in another form, so a commit the policy allows
(2 non-test files, gate green) was left staged for the owner.

A related, deliberate block seen the same day: any shell command whose text mentions git inside
backticks or `$(...)` is refused, including a heredoc that only writes prose about git to a
file. The agent used the editor tool instead. Probably intended, listed so the owner can judge.

Same fix as above for the false positive: parse `-F -` and heredocs, with cases added to
`git-gate_test.sh`.

## Resolution (2026-10-26, the owner, hook v3)
Option 1, by the owner (commit `9091873`): heredoc commit messages and redirections are no
longer read as paths, and `git stash list`, `git stash show` and `git archive` are allowed.
Checked by the M2 agent: `bash .claude/hooks/git-gate_test.sh` gives `pass=74 fail=0`, and
`git stash list` now runs (no stashes on the task branch). The block on git text inside
backticks in a shell command stays, as intended.
