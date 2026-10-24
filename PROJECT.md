# Project card: doriki

This file holds everything `METHOD.md` deliberately leaves out. The method is portable; this is
where we are. Every section is filled. "None" is an answer; blank is not.

---

## 1. The repo and the branches

- **Repo:** `github.com/AmitwalaH/doriki` (personal account). Fill in `AmitwalaH` at M0.
- **Working branch:** one task branch per task file.
- **Branch naming for a task:** `task/<YYYY-MM-DD>-<slug>`, matching the task file name.
- **Who else works in this branch**, and anything automated that runs against it: only the agent
  working the task and the owner. CI (`.github/workflows/ci.yml`) runs on every push.
- **Default branch**, and whether it matters: `main`. Branch-protected: pull request required,
  CI green required. The agent never pushes to it.

## 2. The stack, in one paragraph

doriki is a Go 1.26 single static binary (no CGO) that also runs as a kubectl plugin
(`kubectl doriki`) through `k8s.io/cli-runtime`. It reads Kubernetes events, container statuses,
and inference-engine logs, turns them into a per-pod startup waterfall, and prints advice.
Engine log knowledge lives in YAML parser packs embedded in the binary, not in Go. Minimum
Kubernetes 1.29. The one thing a newcomer gets wrong: the core never talks to Kubernetes.
Collectors do, and everything after them (`normalize`, `phase`, `rules`, `render`) is pure and
tested from fixtures.

## 3. The commands

| What | Command |
|---|---|
| Install | `go install ./cmd/doriki` (local); users: krew / Homebrew / GitHub Releases |
| Run locally | `go run ./cmd/doriki --from-files packs/vllm/testdata/0.28/{pod.json,events.json,container.log}` |
| Types | `go build ./... && go vet ./...` |
| Lint | `make lint` (golangci-lint with gofmt, govet, staticcheck, errcheck, gosec, depguard) |
| Tests | `make test` (unit + golden + schema) |
| The full gate | `make gate` (= build, lint, test; `make e2e` is added to the gate at M2) |
| Regenerate anything generated | goldens: `go test ./... -update` (approval required first); JSON schemas: `make schema` |

**Anything with a flag that matters**, and what goes wrong without it:
`-update` on `go test` rewrites golden files silently. Never run it without approval under
`METHOD.md` Step 11. `make e2e` needs a running `kind` cluster; without one it must skip with a
clear message, never fail.

## 4. What is failing on purpose

| Check | Why it is red | Who owns it | When it goes green |
|---|---|---|---|
| `make e2e` | Not implemented until M2 | owner | M2 (3–9 Nov 2026) |

Anything red and not on this list is ours.

## 5. Off-limits

**Always:**
- Tests and checks owned elsewhere: none yet. Fixtures donated by community users become owned
  by them once merged (see `SOURCE.md` in each fixture directory).
- Generated output, and the source it comes from: `schema/*.json` is generated from Go types by
  `make schema`; edit the types, not the JSON.
- Frozen or pinned artifacts, and what guards them: `packs/*/testdata/**` (fixture captures and
  `expected.json`), golden files under `internal/render/testdata/**`. Guarded by `METHOD.md`
  Step 11: stop and report before re-recording.
- Branches that are not ours: `main`.
- Anything else: any mutating Kubernetes verb; any network call other than the Kubernetes API;
  any telemetry.

**Commit policy (the single recorded exception to `METHOD.md` rule 1):**
The agent may `git commit` and `git push` to its own `task/*` branch, never to `main`, never
force-push, never merge, rebase or open a PR, only when all of these hold:
1. The task file says `commit: allowed`.
2. The change touches at most 2 files, not counting test files the agent wrote itself.
3. `make gate` is green and its output is pasted in the report.
4. No frozen artifact (golden, fixture, schema) was re-recorded.
5. No `[decision]` was raised in the task.
6. The branch is `task/*`.
Any condition unmet or unclear: report at Step 12, do not commit or push.

**Open decisions, so don't decide them:**
- `--probe` request shape and prompt (decide before M3).

**Decided, so follow them:**
- Go, not Rust. Apache-2.0. Packs as data, embedded. No GPU in CI, ever. Report JSON is
  append-only. No telemetry. Name `doriki`. Kubernetes >= 1.29.
- v0.1 advice rules: `pull-dominant`, `fetch-remote-hub`, `compile-miss`, `load-slow-storage`,
  `restart-loop`. The other six are v0.2 (`docs/v0.2-backlog.md`).
- v0.1 scope ends 18 Nov 2026. Cut list and slip order are in `DESIGN.md` section 11.

**Dated and temporary:** live in the task file, not here.

## 6. One encoding per fact

| The fact | Where it lives | Who reads it |
|---|---|---|
| Phase order and names | `internal/phase/order.go` | phase engine, renderers, schema |
| Env capture allowlist | `internal/manifest/allowlist.go` | manifest, capture |
| Secret redaction patterns | `internal/manifest/redact.go` | manifest, capture |
| Advice thresholds | the `const` block at the top of each file in `internal/rules/` | that rule only |
| Pack schema | `schema/pack.v1.json` (generated from `internal/model/pack.go`) | pack loader, `pack test` |
| Report schema | `schema/report.v1.json` (generated from `internal/model/report.go`) | renderers, users |
| Commit policy | this file, section 5 | agent, owner |
| Minimum Kubernetes version | `internal/cli/version.go` (`MinKubeVersion`) and README | cli, docs |

## 7. The ticket convention

- **Prefix:** `BP` is retired; use `DK`.
- **Categories:** `CLI`, `COLLECT`, `PACK`, `PHASE`, `RULES`, `RENDER`, `REL`, `DOCS`.
- **Where tickets live:** `tickets/` as `DK-<CAT>-<BUG|ENH>-<YYYYMMDD>-<slug>.md`.
- **Where plans and handoffs live:** `tasks/` as `YYYY-MM-DD-<task>.md`. Decisions in
  `docs/decisions/NNNN-<slug>.md`.

## 8. What a user actually sees

- **Who the user is:** a platform or SRE engineer at a terminal, running `kubectl doriki pod/x`
  against a cluster serving LLMs; also CI jobs consuming the JSON report.
- **The surfaces they touch:** the terminal waterfall, the JSON report, the `capture` bundle, the
  README, the krew/Homebrew install.
- **The thing that must stay true across all of them:** every number traces to a specific event
  the user can see with `--include-events`; phase totals never exceed wall-clock span.
- **What would embarrass us in front of them:** a leaked token in a capture bundle; a phase total
  longer than the pod has existed; advice naming a fix the engine does not support; a crash on a
  pod whose engine we do not recognise (the `generic` pack must always work).

## 9. Checking it by hand

- **How to get to it:** `kind create cluster --name doriki && make e2e-up` deploys a CPU-only
  llama.cpp server with a tiny GGUF model; then `go run ./cmd/doriki deploy/llamacpp -n doriki`.
- **How to reset to a clean state:** `kind delete cluster --name doriki`.
- **Anything that needs credentials, a tunnel or a VPN:** none.
- **What cannot be checked by hand here, and who can check it:** GPU timing accuracy for vLLM
  and SGLang. Checked by community users who donate `doriki capture` bundles; those become
  fixtures.
