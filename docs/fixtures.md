# From a capture to a fixture

A fixture is one real startup, saved by `doriki capture`, that doriki's tests replay on every
push. This page takes a bundle to a merged fixture. The fixture
[`packs/llamacpp/testdata/11889-lv4/`](../packs/llamacpp/testdata/11889-lv4/SOURCE.md) was
made this way; open it next to these steps. Commands run from the repository root.

## 1. Capture

Right after the pod becomes Ready (Kubernetes keeps events for about an hour), with doriki
v0.1.0-rc.2 or newer:

```sh
doriki capture pod/<pod> -n <namespace> --out <dir> --redact-level strict
```

`strict` also hashes pod, namespace, owner, node and registry names. The bundle's `SOURCE.md`
says what was removed. Read `container.log` yourself before sharing it: a secret with no known
token shape that the engine prints is not removed. To send the bundle in an issue, zip the
directory (`zip -r <dir>.zip <dir>`); GitHub does not take folders.

## 2. Put the directory in place

Move the whole directory to `packs/<engine>/testdata/<version>/`, for example
`packs/vllm/testdata/0.33/`. `<engine>` is the pack's directory name; `<version>` is the
version the engine prints, with a suffix for a variant (`11889-lv4` is build 11889 at `-lv 4`).
Change nothing inside it.

## 3. Write the hand-written part of SOURCE.md

Add your notes **above** the line `# doriki capture`. Leave everything from that line on byte for
byte as `capture` wrote it. Your part says:

- **Engine:** name and version, and the log line that prints it.
- **Image:** the reference and the digest (`containerStatuses[].imageID` in `pod.json`).
- **Model**, and whether it needs a token.
- **Hardware:** GPU model and count, or CPU.
- **Cluster:** Kubernetes and kubelet versions, container runtime, kind or cloud.
- **The manifest** you applied. If you add it to the directory (as `llamacpp.yaml` is in
  `11889-lv4`), say so; step 4 covers its checksum.
- **doriki:** the version that captured it (`doriki version`).
- **Edited: never.** Say that every captured file is exactly as `capture` wrote it.
- **Derived:** `report.json`, written by `capture` from the other files.

## 4. Checksums for files `capture` did not write

`capture` ends `SOURCE.md` with a `## Checksums` section for the files it wrote. For each file
you add (the manifest, for example), write a `## Checksums` section in your own part, one line
per file, in the same format:

```sh
sha256sum packs/llamacpp/testdata/11889-lv4/llamacpp.yaml
```
```
fb4070e45028b005fbc6e1154514aca76b82e045ea67f41c3372de19024c1c4e  packs/llamacpp/testdata/11889-lv4/llamacpp.yaml
```

becomes the line

```
- `llamacpp.yaml`: `sha256:fb4070e45028b005fbc6e1154514aca76b82e045ea67f41c3372de19024c1c4e`
```

A bundle from doriki v0.1.0-rc.2 has no `## Checksums` section at all: write one line per file
this way for every file except `SOURCE.md`, `report.json` and `expected.json`. `make gate`
says which file has no line or the wrong one.

## 5. See which markers the log matches

```sh
doriki pack test packs/llamacpp/11889.yaml --log packs/llamacpp/testdata/11889-lv4/container.log \
  --pod packs/llamacpp/testdata/11889-lv4/pod.json --events packs/llamacpp/testdata/11889-lv4/events.json
```

Every marker and fact is listed with the first line it matched, or `no match`. It exits 1 when
a tested marker matched nothing. An untested marker that matched is flagged
`promote to tested`: say so in the pull request. `doriki --from-files <dir>` prints the report
the fixture gives.

## 6. Record expected.json

`expected.json` holds the phases, gaps, totals and manifest the fixture must keep producing.
Once the report looks right, record it:

```sh
go test ./packs/ -run 'TestFixtures/llamacpp/testdata/11889-lv4$' -update
```

The pattern is the full path under `packs/`: `<engine>/testdata/<version>`. A pattern that
matches no fixture prints `no tests to run` and records nothing. Never hand-edit
`expected.json` or `report.json`; a later pack change that moves them is re-recorded the same
way, and the pull request explains the diff.

## 7. Run the gate

```sh
make gate
```

For a fixture it checks that:
- every file has the sha256 its `SOURCE.md` records, and no listed file is missing
  (`packs/fixtures_test.go`);
- the full pipeline on the directory still gives `expected.json` (`TestFixtures`), and
  `expected.json` fits the report schema;
- `doriki --from-files` on the directory reproduces the phases in `report.json`
  (`packs/capture_test.go`).

It does not check each marker against the log by itself: per-engine tests such as
`packs/llamacpp_lv4_test.go` do, and a maintainer may add one for your fixture.

## 8. What a reviewer looks for

- **Real lines only.** Every marker a pack change adds cites a line of `container.log` by number
  in `SOURCE.md` or the pull request. No log line is written or edited by hand.
- **Timing untouched.** No timestamp changed, no line reordered or deleted, every captured file
  as `capture` wrote it.
- **Nothing private.** No token, no real cluster's names or hostnames (a throwaway `kind`
  cluster's defaults are fine), no host IDs in `node.json`.
- **The capture-written part** of `SOURCE.md` unchanged, and `Edited: never` true.
