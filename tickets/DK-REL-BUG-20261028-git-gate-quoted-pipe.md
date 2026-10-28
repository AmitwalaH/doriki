# DK-REL-BUG-20261028-git-gate-quoted-pipe: A golden re-record with a "|" in a quoted argument skips the approval gate

**Date Discovered:** 2026-10-28
**Severity:** HIGH
**Status:** CLOSED (2026-10-28)
**Label:** [ticket]
**Found by:** the M3 agent, measured: the owner's `.doriki-approve-update` marker was still present
after an approved `-update` run; the hook was then replayed on scratch directories
**Filed by:** the M3 agent

## Problem
The agent runs `go test ./internal/cli/ -run 'TestCLIGoldens/^(a|b)$/' -update` (a subtest
filter with an alternation is the natural way to re-record some goldens and not others). The
hook allows it whether or not the owner's marker exists, and does not consume the marker.

Measured on 2026-10-28 by piping hook input to `.claude/hooks/git-gate.py` with `cwd` set to a
scratch directory:
- `go test ./internal/cli/ -count=1 -run 'TestCLIGoldens/^(kind-nginx)$/' -update -v`, no marker:
  exit 2 (blocked), as intended.
- `go test ./internal/cli/ -run 'a|b' -update`, no marker: exit 0 (allowed).
- The approved run of 2026-10-28 (`-run 'TestCLIGoldens/^(kind-nginx|vllm-0.32|llamacpp-11889)$/'`)
  with the marker present: allowed, marker left in place.

Cause: `split_commands` (`.claude/hooks/git-gate.py` line 106) splits on `|` with a regex that
does not know about quotes, so the command becomes `go test ... -run 'TestCLIGoldens/^(kind-nginx`,
`vllm-0.32`, `llamacpp-11889)$/' -update -v`. `judge_update` (line 242) looks for `go test` and
`-update` in the same segment and finds neither together. The same split affects any quoted
`|`, `;` or `&&`, for example in a `git commit -m` message.

## Why it matters
The marker is the owner's one-time approval for re-recording frozen goldens (METHOD.md Step 11).
A command form that bypasses it makes the gate opt-out, which is what the hook exists to
prevent. Today's run was approved, so nothing was re-recorded without consent; but the marker
left behind would approve the next `-update` too.

## Options
1. Split with quote awareness (`shlex` in punctuation mode, failing closed when it cannot
   parse), and add the three commands above as hook test cases. Recommended.
2. Additionally, judge `-update` on the whole command as well as per segment: any `go test`
   and any `-update` anywhere in one command needs the marker. Cheap belt and braces.

## Related
`DK-REL-BUG-20261026-git-gate-stash-list`; `tasks/2026-10-28-hooks-git-gate.md` (hook v4).

## Resolution
Fixed by hook v5 (`4ee5fd6`, the owner): command splitting is quote-aware, and the `-update`
check applies only where `go` or `make` is the command word, so `-update` quoted as text no
longer consumes the marker. `bash .claude/hooks/git-gate_test.sh` on 2026-10-28:
`git-gate: pass=86 fail=0`; its v5 cases cover a quoted `|` in single and double quotes and
the text-only case. The three commands under Problem, replayed against v5 on a scratch
directory by the M3 agent: each blocked (exit 2) without the marker, allowed with it, and the
marker consumed.
