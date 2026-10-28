# DK-REL-ENH-20261028-release-please-workflow: Releases are tagged by hand; nothing opens the release pull request

**Date Discovered:** 2026-10-28
**Severity:** LOW
**Status:** OPEN, after v0.1.0
**Label:** [ticket]
**Found by:** the M3 plan (choice e, owner decision)
**Filed by:** the M3 agent

## Problem
`DESIGN.md` section 10.1 says release-please opens a release pull request with the changelog
and merging it tags `vX.Y.Z`. M3 committed only `release-please-config.json` and
`.release-please-manifest.json` (manifest at `0.0.0`); there is no workflow that runs
release-please, so v0.1.0-rc.1 and v0.1.0 are tagged by hand, and the manifest will not move
until a workflow runs.

## Why it matters
Tagging by hand is fine for a release candidate and a first release; for later ones the
changelog and version bump are manual work and can drift from the Conventional Commits.

## Options
1. After v0.1.0, add `.github/workflows/release-please.yml` with
   `googleapis/release-please-action` (v5.0.0 on 2026-10-28) pinned by SHA, set the manifest to
   the released version, and let its tag trigger `release.yml`. Note that a tag created with the
   workflow's own token does not trigger other workflows, so it needs a token or GitHub App that
   can. Recommended.
2. Keep tagging by hand and drop the release-please files.

## Related
M3 task acceptance 7; `.github/workflows/release.yml`.
