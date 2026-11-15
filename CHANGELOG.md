# Changelog

All notable changes to doriki are written here. The format follows
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and doriki uses
[Semantic Versioning](https://semver.org/spec/v2.0.0.html).

This file is written by hand until release-please runs
(`tickets/DK-REL-ENH-20261028-release-please-workflow.md`).

<!-- Planned: v0.1.0 is tagged on 2026-11-18. Remove this comment at the tag. -->
## [0.1.0] - 2026-11-18

The first stable release. Added lists everything v0.1.0 does, for a reader new to doriki;
Changed and Fixed list only what moved since v0.1.0-rc.3.

### Added

- A startup waterfall for an LLM inference pod on Kubernetes: `doriki pod/<name>` (also
  `kubectl doriki`) reads the pod's events, container statuses and engine log and shows which
  phase took the time, from scheduling and image pull to weight download, weight load, KV cache
  and server ready. (#2, #3, #5)
- Live pods, `deploy/<name>` and `sts/<name>`, once or with `--follow` while the pod boots, or
  the same report offline from saved files with `--from-files`. doriki only reads; it never
  writes to the cluster. (#2, #5)
- Up to three lines of advice naming the phase that dominated and a fix that applies, from five
  rules: `pull-dominant`, `fetch-remote-hub`, `compile-miss`, `load-slow-storage`,
  `restart-loop`. (#6, #7)
- Parser packs for vLLM 0.32 and llama.cpp build 11889 (default level and `-lv 4`), each written
  from a real CPU capture; any other engine gets the Kubernetes phases and a warning naming the
  image. (#3, #5, #7)
- `-o json`: a versioned, append-only report with a JSON Schema; `--include-events` adds every
  event behind each number. (#2, #3)
- Restarted pods: the latest boot attempt is reported, `--attempt N` picks another and
  `--all-attempts` lists them; time the earlier attempts used is an `earlier attempts` row. (#5)
- A deployment manifest in every report: image digest, owner, allowlisted environment
  variables, node and kubelet version when readable, and a fingerprint. (#5, #6)
- `doriki capture`: saves a pod's inputs as a redacted bundle (`--redact-level strict` hashes
  names too) that `--from-files <dir>` turns back into the same report; its `SOURCE.md` ends
  with the checksums a test fixture needs. (#6, #7, #8, #9)
- `doriki pack test`: checks a parser pack against a log alone, with no cluster. (#6)
- Exit codes: `0` complete, `1` a phase still open or not seen, `2` the pod cannot be read,
  `64` a usage error. (#5)
- Signed releases: binaries for Linux, macOS and Windows on amd64 and arm64, a container image,
  a Homebrew cask and a krew manifest; `checksums.txt` is signed keyless with cosign and each
  archive has an SPDX SBOM. (#6)
- `deploy/rbac.yaml`, a read-only ClusterRole for whoever runs doriki. (#5)
- `docs/fixtures.md`, which walks a shared capture to a test fixture; the issue form asks for a
  strict capture as a zip. (#9)

### Changed

Nothing since v0.1.0-rc.3.

### Fixed

Nothing since v0.1.0-rc.3.

## [0.1.0-rc.3] - 2026-11-15

### Added

- `capture` ends `SOURCE.md` with the Checksums section a test fixture needs, so a bundle passes
  the fixture check as it stands. (#9)
- `docs/fixtures.md`, which walks a shared capture to a test fixture; the issue form asks for a
  strict capture as a zip. (#9)

### Changed

- A phase under a second with a whole-second Kubernetes time at one end prints `<1s` instead of
  a figure such as `0.0s`, which read as free; JSON marks it with `attrs.resolution: "1s"`. (#11)
- Kubernetes client libraries 0.37.2 and go-yaml 3.0.6. (#10)

### Fixed

- The bars add up to the total once the pod is Ready: time from the last phase to Ready is a
  `gap` row (JSON `before: "ready"`) when no phase ends there, for example with no engine pack.
  (#11)
- `fetch-remote-hub` no longer advises a mirror to a pod already using one (`HF_ENDPOINT`,
  `MODEL_ENDPOINT`) or running offline; `model.source` shows the mirror's host; the env
  allowlist gains `MODEL_ENDPOINT` and `TRANSFORMERS_OFFLINE`. (#14)

## [0.1.0-rc.2] - 2026-10-30

### Added

- llama.cpp at `-lv 4`: the report names the build with no warning and shows kv-alloc apart
  from weight-load. A pack may gate markers by the engine's log level (`min_log_level`,
  decision 0010). (#7)
- Kubelet-aware advice: on a node whose kubelet is 1.35 or newer, `pull-dominant` suggests an
  image volume for the model weights. (#7)
- The KV cache size on the terminal, as GiB with one decimal (`<0.1 GiB` below that). (#8)

### Fixed

- `capture` bundles no longer carry the node's machine ID, system UUID or boot ID. (#7)
- A phase nested inside another no longer shows a false gap; gaps are measured from the latest
  end so far. (#8)
- The `capture` command in a default-level `SOURCE.md` can be pasted as written. (#8)

## [0.1.0-rc.1] - 2026-10-28

The first release candidate.

### Added

- The startup waterfall from `--from-files`: phases from pod creation to Ready, `gap` rows for
  time no phase covers, `~` for estimates, and a JSON report with its schema. (#2, #3)
- The vLLM 0.32 parser pack, from a real CPU capture. (#3)
- Live collection: `pod/`, `deploy/` and `sts/` targets, `--follow`, `--attempt`,
  `--all-attempts`, `--since`, exit code `2` when the pod cannot be read, and a read-only
  ClusterRole. (#5)
- The llama.cpp build 11889 parser pack at its default log level, and a `generic` fallback that
  shows the Kubernetes phases for any other engine. (#5)
- The deployment manifest: image digest, owner, allowlisted environment, secret redaction and a
  fingerprint. (#5)
- Five advice rules, `doriki capture`, `doriki pack test`, and engine log lines redacted in
  every report. (#6)
- The signed release pipeline: binaries, container image, Homebrew cask, krew manifest, SBOMs.
  (#6)
- `doriki version`. (#1)

[0.1.0]: https://github.com/AmitwalaH/doriki/compare/v0.1.0-rc.3...v0.1.0
[0.1.0-rc.3]: https://github.com/AmitwalaH/doriki/compare/v0.1.0-rc.2...v0.1.0-rc.3
[0.1.0-rc.2]: https://github.com/AmitwalaH/doriki/compare/v0.1.0-rc.1...v0.1.0-rc.2
[0.1.0-rc.1]: https://github.com/AmitwalaH/doriki/releases/tag/v0.1.0-rc.1
