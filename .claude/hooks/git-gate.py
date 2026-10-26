#!/usr/bin/env python3
"""doriki git gate: a Claude Code PreToolUse hook for the Bash tool.

Turns METHOD.md rule 1 and the PROJECT.md section 5 commit policy from prose into a
mechanism. It reads the proposed shell command from stdin (JSON), and exits 2 (block,
message shown to the agent) when the command would:

  - push to main, force-push, or delete a remote branch;
  - merge, rebase, open or merge a pull request (human-only);
  - move or discard work: checkout, switch, reset, restore, stash, clean, branch -D/-d,
    commit --amend, filter-repo (METHOD.md rule 1 read-only list);
  - commit outside a task/* branch;
  - commit more than 2 non-test files (policy condition 2);
  - commit when `make gate` has not passed since the last change (policy condition 3,
    proven by a .gate-ok marker that the Makefile writes on success);
  - regenerate goldens with -update without the owner's one-time approval marker
    (.doriki-approve-update in the repo root; consumed on use).
    The marker is consumed when the hook approves the command, before the command runs, so
    run the -update command alone: a check for the marker in the same command finds it gone.

Everything else passes. Allowed git verbs: status, log, diff, show, grep, add, commit
(under the conditions above), push to task/* without force, fetch, remote -v, rev-parse,
branch (listing only), tag (listing only), ls-files, blame, describe, check-ignore.

Fails closed: any error while judging a git/gh command blocks it with the error text.
"""

import json
import os
import re
import subprocess
import sys

MAX_NON_TEST_FILES = 2
GATE_MARKER = ".gate-ok"
UPDATE_MARKER = ".doriki-approve-update"
TASK_BRANCH = re.compile(r"^task/[0-9]{4}-[0-9]{2}-[0-9]{2}-[a-z0-9][a-z0-9-]*$")

# git or gh used as a command word (preceded by start/space/quote/paren/separator and followed by
# whitespace or end), so "git-gate_test.sh" and "docs/git-workflow.md" do not match.
CMD_WORD = re.compile(r'''(?:^|[\s;&|('"`])(?:git|gh)(?:\s|$)''')

WRAPPERS = {
    "bash", "sh", "zsh", "dash", "fish", "env", "eval", "exec", "xargs", "sudo", "nohup",
    "timeout", "command", "nice", "time", "script", "watch", "python3", "python", "perl",
}
READ_ONLY_GIT = {
    "status", "log", "diff", "show", "grep", "rev-parse", "ls-files", "blame",
    "describe", "check-ignore", "remote", "fetch", "cat-file", "name-rev",
    "shortlog", "reflog", "config", "var", "help", "version", "worktree", "archive",
}
FORBIDDEN_GIT = {
    "checkout", "switch", "reset", "restore", "stash", "clean", "rebase", "merge",
    "pull", "cherry-pick", "revert", "filter-repo", "filter-branch", "gc", "prune",
    "rm", "mv",
}


def block(msg):
    sys.stderr.write("git-gate: BLOCKED. " + msg + "\n")
    sys.exit(2)


def run(args, cwd):
    return subprocess.run(args, cwd=cwd, capture_output=True, text=True, check=False)


def current_branch(cwd):
    r = run(["git", "rev-parse", "--abbrev-ref", "HEAD"], cwd)
    if r.returncode != 0:
        block("cannot determine current branch: " + r.stderr.strip())
    return r.stdout.strip()


def strip_heredocs(cmd):
    """Remove heredoc bodies (data) but keep every command line before and after them.

    For each line containing <<WORD or <<'WORD' or <<-WORD, the lines after it up to and
    including the terminator line (a line equal to WORD, ignoring leading tabs) are dropped.
    The opening line itself stays, so the command that reads the heredoc is still judged.
    Lines after the terminator are commands and stay."""
    lines = cmd.split("\n")
    out = []
    i = 0
    while i < len(lines):
        line = lines[i]
        out.append(line)
        m = re.search(r"<<-?\s*['\"]?(\w+)['\"]?", line)
        if m:
            word = m.group(1)
            i += 1
            while i < len(lines) and lines[i].lstrip("\t") != word:
                i += 1
            i += 1  # skip the terminator line
            continue
        i += 1
    return "\n".join(out)


def split_commands(cmd):
    """Split on shell separators so chained commands are judged one by one.

    Heredoc bodies are data, not commands, and are removed first; everything else,
    including commands on lines after a heredoc, is judged."""
    cmd = strip_heredocs(cmd)
    parts = re.split(r"\s*(?:&&|\|\||;|\|)\s*|\n", cmd)
    return [p.strip() for p in parts if p and p.strip()]


def tokens(segment):
    # Good enough for git/gh invocations; quoted commit messages do not matter here.
    try:
        import shlex
        return shlex.split(segment)
    except ValueError:
        return segment.split()


def judge_git(toks, cwd):
    args = [t for t in toks[1:] if not t.startswith("-c")]  # drop `-c key=val` global opts
    if not args:
        return
    verb = args[0]
    rest = args[1:]

    if verb == "stash" and rest and rest[0] in ("list", "show"):
        return  # read-only stash queries are allowed
    if verb in FORBIDDEN_GIT:
        block(f"`git {verb}` moves or discards work; METHOD.md rule 1 allows read-only "
              "version control only. Ask the owner to run it.")

    if verb == "branch":
        if any(a in ("-D", "-d", "--delete", "-m", "-M", "--move") for a in rest):
            block("deleting or renaming branches is the owner's job (METHOD.md rule 1).")
        if any(not a.startswith("-") for a in rest):
            block("creating branches is the owner's job; the task branch already exists.")
        return

    if verb == "tag" and any(not a.startswith("-") for a in rest) and "-l" not in rest:
        block("tags are created by the release pipeline, never by the agent.")

    if verb == "push":
        if any(a in ("-f", "--force", "--force-with-lease", "--force-if-includes", "-d",
                     "--delete", "--mirror", "--all") or a.startswith("--force") for a in rest):
            block("force pushes and remote deletes are never delegated (PROJECT.md section 5).")
        targets = [a for a in rest if not a.startswith("-")]
        refspecs = targets[1:] if targets else []
        branch = current_branch(cwd)
        if not refspecs and not TASK_BRANCH.match(branch):
            block(f"push from branch `{branch}` is not allowed; only task/* branches.")
        for spec in refspecs:
            dst = spec.split(":")[-1]
            if spec.startswith(":"):
                block("deleting a remote branch is never delegated.")
            if dst in ("main", "master", "HEAD") and not TASK_BRANCH.match(branch):
                block("pushing to main is never delegated (PROJECT.md section 5).")
            if dst in ("main", "master"):
                block("pushing to main is never delegated (PROJECT.md section 5).")
            if dst.startswith("refs/heads/") and not TASK_BRANCH.match(dst[len("refs/heads/"):]):
                block(f"push target `{dst}` is not a task/* branch.")
            if not dst.startswith("refs/") and not TASK_BRANCH.match(dst):
                block(f"push target `{dst}` is not a task/* branch.")
        return

    if verb == "commit":
        if any(a in ("--amend", "--no-verify", "-n") for a in rest):
            block("`--amend` rewrites history and `--no-verify` skips checks; neither is allowed.")
        if any(a in ("-a", "--all", "--include", "-i", "--only", "-o", "--interactive", "-p",
                     "--patch") or (a.startswith("-") and len(a) > 1 and not a.startswith("--")
                                    and "a" in a[1:] and a not in ("-m", "-F", "-q", "-v", "-s", "-S"))
               for a in rest):
            block("`git commit -a/--all/--include/--only/--patch` adds files at commit time, "
                  "bypassing the staged-file limit; stage with `git add` and commit without them.")
        # path arguments after the options also add files at commit time
        pathspec = []
        skip_next = False
        for a in rest:
            if skip_next:
                skip_next = False
                continue
            if a in ("-m", "--message", "-F", "--file", "--author", "--date", "-C", "-c",
                     "--reuse-message", "--reedit-message", "--trailer", "--cleanup"):
                skip_next = True
                continue
            if a.startswith("-"):
                continue
            if a.startswith(("<", ">", "&", "|")) or re.match(r"^\d*[<>]", a):
                continue  # shell redirection (<<EOF, >file, 2>&1), not a path
            pathspec.append(a)
        if pathspec:
            block("`git commit <path>` commits files that were not staged with `git add`; "
                  "stage them first and commit without paths: " + ", ".join(pathspec))
        branch = current_branch(cwd)
        if not TASK_BRANCH.match(branch):
            block(f"commits are allowed only on task/* branches; current branch is `{branch}` "
                  "(PROJECT.md section 5, condition 6).")
        r = run(["git", "diff", "--cached", "--name-only"], cwd)
        staged = [f for f in r.stdout.split("\n") if f.strip()]
        if not staged:
            block("nothing is staged.")
        non_test = [f for f in staged if not f.endswith("_test.go")
                    and "/testdata/" not in f and not f.startswith("testdata/")]
        if len(non_test) > MAX_NON_TEST_FILES:
            block(f"{len(non_test)} non-test files are staged; the policy allows at most "
                  f"{MAX_NON_TEST_FILES} (PROJECT.md section 5, condition 2). Report at Step 12 "
                  "and let the owner commit.\n  staged: " + ", ".join(non_test))
        marker = os.path.join(cwd, GATE_MARKER)
        if not os.path.exists(marker):
            block("no `.gate-ok` marker: run `make gate` (it writes the marker on success) "
                  "before committing (PROJECT.md section 5, condition 3).")
        gate_time = os.path.getmtime(marker)
        stale = [f for f in staged
                 if os.path.exists(os.path.join(cwd, f))
                 and os.path.getmtime(os.path.join(cwd, f)) > gate_time]
        if stale:
            block("`make gate` ran before these staged files last changed; run it again:\n  "
                  + ", ".join(stale))
        return

    if verb in READ_ONLY_GIT or verb == "add":
        return

    block(f"`git {verb}` is not on the allowed list; ask the owner.")


def judge_gh(toks):
    args = toks[1:]
    if len(args) >= 2 and args[0] == "pr" and args[1] in ("merge", "create", "close", "reopen",
                                                           "edit", "ready", "review"):
        block(f"`gh pr {args[1]}` is the owner's job; the agent never opens, edits or merges "
              "pull requests.")
    if len(args) >= 1 and args[0] in ("release", "api") and any(
            a in ("-X", "--method") for a in args) and any(
            a.upper() in ("PUT", "POST", "PATCH", "DELETE") for a in args):
        block("mutating `gh api` and `gh release` calls are the owner's job.")
    if len(args) >= 2 and args[0] == "repo" and args[1] in ("edit", "delete", "rename",
                                                             "archive", "create"):
        block(f"`gh repo {args[1]}` is the owner's job.")


def judge_update(segment, cwd):
    if re.search(r"\bgo\s+test\b.*\s-update\b", segment) or re.search(r"\bmake\s+.*update", segment):
        marker = os.path.join(cwd, UPDATE_MARKER)
        if not os.path.exists(marker):
            block("regenerating goldens with -update needs the owner's approval under METHOD.md "
                  f"Step 11. The owner creates `{UPDATE_MARKER}` in the repo root once; it is "
                  "consumed by this run.")
        os.remove(marker)
        sys.stderr.write("git-gate: -update approved once; marker consumed.\n")


def main():
    try:
        data = json.load(sys.stdin)
    except Exception as e:  # noqa: BLE001
        block(f"cannot read hook input ({e}); failing closed.")
    if data.get("tool_name") != "Bash":
        sys.exit(0)
    cmd = (data.get("tool_input") or {}).get("command", "") or ""
    cwd = data.get("cwd") or os.getcwd()

    lowered = cmd
    if CMD_WORD.search(lowered):
        if re.search(r"\$\(|`", lowered):
            block("git/gh inside command substitution ($(...) or backticks) cannot be judged; "
                  "run it as a plain command.")
        if re.search(r"(^|[;&|]\s*)cd\s", lowered):
            block("`cd` combined with git/gh in one command cannot be judged against the right "
                  "repository; run git from the repository root in its own command.")

    for segment in split_commands(cmd):
        toks = tokens(segment)
        if not toks:
            continue
        head = os.path.basename(toks[0])
        if head in WRAPPERS and CMD_WORD.search(segment):
            block(f"`{head}` wrapping a git/gh command cannot be judged; run git/gh directly.")
        try:
            if head == "git":
                judge_git(toks, cwd)
            elif head == "gh":
                judge_gh(toks)
            judge_update(segment, cwd)
        except SystemExit:
            raise
        except Exception as e:  # noqa: BLE001
            block(f"error while judging `{segment}`: {e}")
    sys.exit(0)


if __name__ == "__main__":
    main()
