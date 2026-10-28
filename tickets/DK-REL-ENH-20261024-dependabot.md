# DK-REL-ENH-20261024-dependabot: Pinned CI actions and Go modules never get update PRs

**Date Discovered:** 2026-10-24
**Severity:** LOW
**Status:** CLOSED (2026-10-28)
**Label:** [ticket]
**Found by:** the M0 agent, read in `DESIGN.md` section 10.3 while pinning actions in `ci.yml`
**Filed by:** the M0 agent

## Problem
`DESIGN.md` section 10.3 says "Dependabot for Go modules and Actions, with Actions pinned to
commit SHAs". M0 pins the three actions in `.github/workflows/ci.yml` to SHAs, but there is no
`.github/dependabot.yml`, so nothing proposes updates. The M0 task file does not list it.

## Why it matters
SHA pins are only safe if someone updates them. Without Dependabot, a security fix in
`actions/checkout`, `actions/setup-go`, `golangci-lint-action` or `spf13/cobra` is missed
silently until someone looks by hand.

## Options
1. Add `.github/dependabot.yml` with `gomod` and `github-actions` ecosystems, weekly. Recommended:
   one small file, matches the design.
2. Fold it into the M3 release-pipeline task.

## Related
`DESIGN.md` section 10.3; `.github/workflows/ci.yml`.

## Resolution
Option 2, in M3 slice 8: `.github/dependabot.yml` asks for weekly update pull requests for
`gomod` and `github-actions`, one grouped pull request per ecosystem. It covers the SHA-pinned
actions in `ci.yml` and the new `release.yml`. Not covered: the distroless base image digest in
`deploy/Dockerfile`, which is bumped by hand (its comment says so); adding the `docker`
ecosystem would cover it.
