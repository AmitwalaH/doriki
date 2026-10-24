# doriki: agent instructions

This repo uses the owner's three-file method. Read these, in order, at the start of every
session, before any code:

1. `METHOD.md`: how we work. It wins over anything below.
2. `PROJECT.md`: where we are: stack, commands, off-limits, decided and open, commit policy.
3. `tasks/<YYYY-MM-DD>-<task>.md`: what we are doing now. The newest file is the current task.

`METHOD.md` and the current task file are provided by the owner outside the repo; they are not
committed. If either is missing from the working directory, stop and ask the owner for it before
doing anything else.

`DESIGN.md` is the product and architecture source of truth. If a task conflicts with it, stop
and ask (METHOD.md rule 6, numbered questions).

## Project-specific non-negotiables (these add to METHOD.md; they never replace it)

- Go 1.26, no CGO. No new dependency without a one-line justification in the report.
- `internal/phase`, `internal/rules`, `internal/diff`, `internal/render` are pure: no I/O, no
  `time.Now()`, no cluster access. Take `now` as a parameter where needed.
- Never write to the Kubernetes cluster. Mutating client-go calls are forbidden.
- Packs ship with a real fixture under `packs/<engine>/testdata/`. Never fabricate logs.
- Env capture is allowlist-only; secrets are redacted before anything is written to disk.
- Report JSON is append-only. Do not rename or remove fields.
- Golden files and fixtures are frozen artifacts under METHOD.md Step 11: stop and report before
  re-recording anything.

## Commit policy

The single exception to METHOD.md rule 1 is written in `PROJECT.md` section 5. You may
`git add`, `git commit` and `git push origin task/<branch>` only when every condition there
holds. If any condition fails, or you are unsure, do not commit or push: report at Step 12 and
stop. You never push to `main`, never force-push, never merge, rebase, or open a pull request.

## Commands (also in PROJECT.md section 3)

- `make build` · `make test` · `make lint` · `make gate` (build + lint + test)
- `make e2e` (kind, CPU engines; added to the gate at M2)
- `go run ./cmd/doriki --from-files packs/vllm/testdata/0.28/{pod.json,events.json,container.log}`
  is the fastest manual check once M1 lands.

## Checkpoints (from METHOD.md section 3)

1. After Step 4, the plan: report Steps 0 to 4 and wait. No code, no test files, no edits.
2. Whenever rule 6 applies.
3. At Step 12, the report.

## Do not

- Do not push to `main`, force-push, merge, rebase, or open PRs. Do not commit or push outside
  the commit policy.
- Do not add a TUI, a web server, a database, or an LLM call.
- Do not edit a golden file or fixture by hand; regenerate with `-update` only after approval.
- Do not claim a check passed without pasting its output.
- Do not put a real person's name in code or comments. No em dashes in prose.
