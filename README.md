# doriki

doriki answers the question operators of LLM serving on Kubernetes ask every week: why did this
inference pod take six minutes to become ready, and which phase ate the time? It reads a pod's
Kubernetes events, its container statuses and the inference engine's own log, turns them into a
startup waterfall (scheduling, image pull, engine start-up, weight download, weight load,
compile, graph capture, KV cache, server ready) and, from v0.1.0, prints advice on which fix
applies. It is a single static binary that also runs as a kubectl plugin, stores nothing, calls
no SaaS and sends no telemetry.

**doriki measures and advises; it never changes the cluster.**

**Status:** pre-release. Today doriki builds the report from saved files (`--from-files`);
profiling a live pod (`kubectl doriki pod/<name>`) and the advice rules arrive before v0.1.0,
planned for 18 Nov 2026.

## What it shows

This is real output, not a mock-up: vLLM 0.32 starting `facebook/opt-125m` on CPU in a local
`kind` cluster, from the files in [`packs/vllm/testdata/0.32/`](packs/vllm/testdata/0.32/)
(copied from [`internal/cli/testdata/vllm-0.32.txt`](internal/cli/testdata/vllm-0.32.txt)).

```text
Pod default/vllm-785fb8997d-bvsz4   engine vllm 0.32.0   node doriki-control-plane   total 7:36

scheduling       │                                               0.0s
image-pull       ██████████████████████████                      4:28
container-init                             █                     0.4s
engine-init                                ███████               1:11  facebook/opt-125m
gap                                               ░              0:01
weight-fetch                                      ██████████    ~1:48  facebook/opt-125m
weight-load                                                 █   ~0.5s
kv-alloc                                                    █    0.0s  1.0 GiB, 29,056 tokens
gap                                                         ░    0:05
server-ready                                                │    0.0s

skipped: compile, graph-capture (the engine said it skips them)
absent: node-provision, first-token (not seen in the events or the log; pack vllm/0.32)
~ estimated: confidence below 1.0 (a start derived from a logged duration, or a heuristic marker)
```

Bars are placed by when each phase ran and sized by its share of the time from pod creation to
Ready; `gap` rows are time no phase accounts for. `~` marks an estimate: vLLM logs only how long
the weight download and load took, so their starts are computed from those durations. `-o json`
writes the same report as JSON, with every event behind it under `--include-events`.

To reproduce it from a clone:

```sh
go run ./cmd/doriki --from-files packs/vllm/testdata/0.32/{pod.json,events.json,container.log}
```

## What is validated

doriki is only as good as the captures its parsers are tested against, so this is the whole list
today:

- **vLLM 0.32.0 on CPU only.** The vLLM parser pack is written from one real capture: a
  community CPU build of vLLM 0.32.0 in `kind`, in eager mode (image, digest and settings in
  [`SOURCE.md`](packs/vllm/testdata/0.32/SOURCE.md)). Every phase it reports is checked against
  a line of that log.
- **Observed:** image pull, container and engine start-up, weight download, weight load,
  **kv-alloc (CPU vLLM logs its KV cache)**, server ready.
- **Not yet observed: compile and graph-capture.** Eager mode turns both off and CPU vLLM does
  not capture CUDA graphs. The pack has markers for them taken from the vLLM 0.32.0 source, at
  lower confidence, but no capture has exercised them.
- **GPU vLLM images (`vllm/vllm-openai`) are matched by the pack's image pattern but unverified.**
  On a GPU pod doriki will try, and the waterfall may miss or misplace phases.
- Engines without a pack (anything other than vLLM 0.32.0 today) get the Kubernetes phases only,
  with a warning naming the image.

### Please send a capture from a GPU vLLM pod

One real startup from a GPU pod turns those unverified phases into tested ones. Right after the
pod becomes Ready (Kubernetes keeps events for about an hour), save these three files:

```sh
POD=<your vLLM pod>
kubectl logs "$POD" --timestamps > container.log
kubectl get pod "$POD" -o json > pod.json
kubectl get events --field-selector involvedObject.name="$POD" -o json > events.json
```

Before sharing them, redact, and say in the issue what you changed:

- **Secrets:** in `pod.json`, the values of any secret-looking `env` entries (`HF_TOKEN`, API
  keys, passwords) and any `--api-key` in `args`; search all three files with
  `grep -n -i -E 'token|key|secret|password' pod.json events.json container.log` and replace
  each secret value with `REDACTED`.
- **Names, if you need to:** pod, namespace, node, registry or model names may be replaced, but
  replace each one the same way everywhere it appears.
- **Never** change a timestamp, reorder lines or delete log lines: the timing is what doriki
  measures.

Then open an issue with the files attached and the vLLM version, GPU model and image.

## Requirements

- Kubernetes **1.29** or newer.
- Read-only access to pods, pod logs and events in the namespaces you profile. doriki asks for
  no write permission and makes no write call.

## Install

None of these are published yet. They are the channels v0.1.0 will ship on.

| Channel | Command | Status |
|---|---|---|
| krew | `kubectl krew install doriki` | not yet published |
| Homebrew | `brew install AmitwalaH/tap/doriki` | not yet published |
| Go | `go install github.com/AmitwalaH/doriki/cmd/doriki@latest` | not yet published |
| GitHub Releases | download a signed binary, then `cosign verify-blob` | not yet published |

Until then, build from source with Go 1.26:

```sh
git clone https://github.com/AmitwalaH/doriki.git
cd doriki
make build
./bin/doriki version
```

## Roadmap

v0.1.0 is planned for **18 Nov 2026**: live profiling of a pod or Deployment, `--follow`, the
`capture` command for shareable redacted bundles, the first five advice rules, and the
llama.cpp and Ollama packs. Everything cut from v0.1, and why, is in
[`docs/v0.2-backlog.md`](docs/v0.2-backlog.md): `diff` with a CI exit code, the Prometheus
exporter, the SGLang pack and the HTML report lead its default order, which v0.1 feedback will
settle.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md). doriki is licensed under
the [Apache License 2.0](LICENSE).
