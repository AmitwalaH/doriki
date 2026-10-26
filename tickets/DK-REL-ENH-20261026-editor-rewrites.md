# DK-REL-ENH-20261026-editor-rewrites: An editor can silently reformat packs and fixture captures

**Date Discovered:** 2026-10-26
**Severity:** LOW
**Status:** CLOSED (2026-10-26)
**Label:** [ticket]
**Found by:** the owner, walked: their editor rewrote the quotes in `packs/llamacpp/11889.yaml`
on save, after the agent wrote it (2026-10-26)
**Filed by:** the M2 agent, on the owner's instruction

## Problem
Someone opens a pack or a fixture in an editor with format-on-save and saves it. The file
changes without anyone deciding to change it. For a pack that was harmless this time (single
quotes became double quotes; the regexes mean the same). For a fixture capture it would break
the rule that fixtures are never edited by hand (`DESIGN.md` section 9.2): the capture would no
longer be what kubectl wrote, and nothing would notice unless a golden happened to move.

## Why it matters
Fixtures are the evidence every pack and golden rests on. A silent rewrite of a capture file
is indistinguishable from a hand edit.

## Options (both, in slice 8)
1. A committed `.vscode/settings.json` that turns format-on-save off for this repository.
2. A test that recomputes the sha256 of every fixture capture file (`pod.json`,
   `events.json`, `container.log`, `container-previous.log`, and an applied manifest) and
   compares it with the hash recorded in that fixture's `SOURCE.md`, so a rewrite fails the
   gate. Existing `SOURCE.md` files gain the hashes once, from the committed files.

## Related
`DESIGN.md` section 9.2; `packs/*/testdata/*/SOURCE.md`; M2 slice 8.

## Resolution (2026-10-26, M2 slice 8)
Both options. `.vscode/settings.json` turns off format-on-save, trailing-whitespace trimming and
final-newline insertion for this repository. `packs/fixtures_test.go` (`TestFixtureChecksums`)
recomputes the sha256 of every captured file in every `packs/*/testdata/*/` and compares it with
the `## Checksums` section appended to that fixture's `SOURCE.md` (computed from the committed
files; no capture file was touched); it also fails on a captured file with no entry, or an entry
with no file. `.gitattributes` marks the fixtures `-text`, so no checkout converts their line
endings and the hashes hold on every platform. Pack YAML files are not hashed: they are code,
and their own tests check what they match.
