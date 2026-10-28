#!/usr/bin/env bash
# Tests the git-gate hook with the exact JSON Claude Code sends. Run from the repo root:
#   bash .claude/hooks/git-gate_test.sh
# Exit 0 only when every case passes. CI runs this so a config edit that narrows the gate fails.
set -u
HOOK="$(cd "$(dirname "$0")" && pwd)/git-gate.py"
T=$(mktemp -d); trap 'rm -rf "$T"' EXIT
cd "$T"
git init -q -b main . && git config user.email t@t && git config user.name t
echo hi > a.go && git add . && git commit -qm init
git checkout -q -b task/2026-01-01-test
pass=0; fail=0
run() {
  out=$(printf '{"tool_name":"Bash","tool_input":{"command":%s},"cwd":"%s"}' \
        "$(python3 -c 'import json,sys;print(json.dumps(sys.argv[1]))' "$1")" "$T" \
        | python3 "$HOOK" 2>&1); code=$?
  if [ "$code" = "$2" ]; then pass=$((pass+1)); else fail=$((fail+1)); printf 'FAIL [got %s want %s] %s\n  %s\n' "$code" "$2" "$1" "$out"; fi
}
for c in "git status --short" "git log --oneline -3 && git diff --stat" "git add x.go" "git fetch --prune" "gh pr checks --watch" "go test ./... && make gate" "gh pr view 3 --json mergeStateStatus"; do run "$c" 0; done
for c in "git checkout main" "git switch -c foo" "git reset --hard HEAD~1" "git restore --staged a.go" "git stash" "git clean -fd" "git rebase main" "git merge main" "git pull" "git branch -D task/x" "git branch new-branch" "git filter-repo --invert-paths --path x" "git status && git checkout -- a.go"; do run "$c" 2; done
run "git push origin task/2026-01-01-test" 0
run "git push -u origin task/2026-01-01-test" 0
for c in "git push origin main" "git push --force origin task/2026-01-01-test" "git push origin +main" "git push origin :task/old" "git push origin HEAD:main" "git push -f"; do run "$c" 2; done
for c in "gh pr create --title x" "gh pr merge 3 --merge" "gh repo edit --visibility private" "gh api -X PUT repos/x/y/branches/main/protection"; do run "$c" 2; done
echo x > b.go; echo y > c.go; echo z > d.go; git add b.go c.go d.go
run "git commit -m 'too many files'" 2
git reset -q b.go c.go d.go; git add b.go; echo t > b_test.go; git add b_test.go
run "git commit -m 'no gate marker'" 2
touch .gate-ok
run "git commit -m 'gate ok'" 0
sleep 1.1; echo more >> b.go; git add b.go
run "git commit -m 'stale gate'" 2
run "git commit --amend -m x" 2
git checkout -q main; run "git commit -m 'on main'" 2; git checkout -q task/2026-01-01-test
run "go test ./packs/ -run TestFixtures -update" 2
touch .doriki-approve-update
run "go test ./packs/ -run TestFixtures -update" 0
if [ -e .doriki-approve-update ]; then fail=$((fail+1)); echo "FAIL marker not consumed"; else pass=$((pass+1)); fi
run "go test ./packs/ -run TestFixtures -update" 2
# v2 cases: bypasses found in review on 26 Oct
echo q > q.go; git add q.go; sleep 0.2; touch .gate-ok
for c in "git commit -a -m x" "git commit --all -m x" "git commit -am x" "git commit --include q.go -m x" "git commit --only q.go -m x" "git commit -p -m x" "git commit -m x q.go" "git commit -m x -- q.go"; do run "$c" 2; done
run "git commit -m 'plain staged commit still allowed'" 0
for c in "bash -c 'git push origin main'" "sh -c 'git checkout main'" "env git push origin main" "eval git stash" "xargs git rm" "sudo git reset --hard" "cd /tmp && git commit -m x" "cd .. ; git push origin main" "echo \$(git stash)" "echo \`git checkout main\`"; do run "$c" 2; done
run "echo 'no git here' && ls" 0
# v3 cases: heredoc commits, redirections, read-only stash/archive (26 Oct)
git reset -q; echo h > h.go; git add h.go; sleep 0.2; touch .gate-ok
run "git commit -q -F - <<'EOF'
fix(x): heredoc message

body line mentioning git push origin main as prose
EOF" 0
run "git commit -F - <<EOF
feat: unquoted heredoc marker
EOF" 0
run "git log --oneline -3 > /tmp/log.txt 2>&1" 0
run "git stash list" 0
run "git stash show -p" 0
run "git stash" 2
# v5 cases: quoted pipes are not separators; -update as text does not consume the marker
run "go test ./internal/cli/ -run 'TestCLIGoldens/^(kind-nginx|vllm)$/' -update" 2
run "go test ./pkg -run \"a|b\" -update" 2
touch .doriki-approve-update
run "echo 'go test ./x -update is text'; for c in 'go test -update'; do echo \$c; done" 0
[ -e .doriki-approve-update ] && pass=$((pass+1)) || { fail=$((fail+1)); echo "FAIL marker consumed by text"; }
run "go test ./internal/cli/ -run 'TestCLIGoldens/^(kind-nginx|vllm)$/' -update" 0
[ ! -e .doriki-approve-update ] && pass=$((pass+1)) || { fail=$((fail+1)); echo "FAIL marker not consumed by real run"; }
run "git log --oneline | head -3" 0
run "git push origin 'main'" 2
run "git stash pop" 2
run "git archive --format=tar HEAD > /tmp/x.tar" 0
# v4 cases: commands after a heredoc body must still be judged (bypass found 26 Oct)
run "python3 - x.go <<'EOF'
print('hello')
EOF
git rm -q --cached a.go 2>/dev/null; rm -f a.go; echo done" 2
run "cat <<EOF > /tmp/note.txt
git push origin main is just text here
EOF
git push origin main" 2
run "cat <<EOF > /tmp/note.txt
some text
EOF
git status" 0
run "cat <<-EOF
	tab-indented terminator
	EOF
git stash" 2
run "bash .claude/hooks/git-gate_test.sh" 0
run "python3 .claude/hooks/git-gate.py --version" 0
run "cat docs/git-workflow.md" 0
printf 'not json' | python3 "$HOOK" >/dev/null 2>&1; [ $? -eq 2 ] && pass=$((pass+1)) || { fail=$((fail+1)); echo "FAIL unreadable input did not fail closed"; }
echo "git-gate: pass=$pass fail=$fail"
[ "$fail" = 0 ]
