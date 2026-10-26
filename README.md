# doriki

doriki answers the question operators of LLM serving on Kubernetes ask every week: why did this
inference pod take six minutes to become ready, and which phase ate the time? It reads a pod's
Kubernetes events, its container statuses and the inference engine's own log, turns them into a
startup waterfall (scheduling, image pull, engine start-up, weight download, weight load,
compile, graph capture, KV cache, server ready) and, from v0.1.0, prints advice on which fix
applies. It is a single static binary that also runs as a kubectl plugin, stores nothing, calls
no SaaS and sends no telemetry.

**doriki measures and advises; it never changes the cluster.**

**Status:** pre-release. doriki profiles a live pod, Deployment or StatefulSet, once or with
`--follow` while it boots, and builds the same report from saved files (`--from-files`). The
advice rules and the `capture` command arrive before v0.1.0, planned for 18 Nov 2026.

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

## Usage

```sh
doriki pod/vllm-7c9f -n serving          # one pod; a bare name is a pod too
doriki deploy/vllm -n serving            # the newest pod of a Deployment (sts/<name>: StatefulSet)
doriki deploy/vllm -n serving --follow   # watch it boot; redraws as phases complete
doriki pod/vllm-7c9f -o json             # the report as JSON (--include-events adds every event)
```

- **Cluster access** uses kubectl's own flags: `--kubeconfig`, `--context`, `-n`, `--as` and
  the rest. doriki only reads (see [Requirements](#requirements)).
- **`--follow`** streams the container log and reads the pod and its events every 2 s. It stops
  when the pod is Ready and every phase the engine's pack can see is complete, or at `--timeout`
  (default 15m) with the report as it stands. On a terminal it redraws in place; otherwise it
  prints the final report once. Started right after `kubectl apply`, it waits for the first pod.
- **Restarts:** the latest boot attempt is reported. `--attempt N` picks another (N is the
  container's restart count) and `--all-attempts` shows every attempt the pod object still has;
  time the earlier attempts used appears as an `earlier attempts` row.
- **`--since 24h`** uses only events last seen in that window. Kubernetes keeps events for about
  an hour by default, so profile a pod soon after it starts.
- **Exit codes:** `0` the report is complete; `1` a phase doriki expected is still open or not
  seen (the pod is still booting, or its events expired); `2` the pod cannot be read (not found,
  no permission, cluster unreachable), with the API's message; `64` a usage error.

### As a kubectl plugin

kubectl runs any `kubectl-<name>` binary on your `PATH` as `kubectl <name>`. Put doriki there
under that name, or link it:

```sh
ln -s "$PWD/bin/doriki" ~/.local/bin/kubectl-doriki   # any directory on your PATH
kubectl doriki pod/vllm-7c9f -n serving
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
- **llama.cpp server build 11889 on CPU, at its default log level.** One real capture in `kind`
  ([`SOURCE.md`](packs/llamacpp/testdata/11889/SOURCE.md)): engine start-up, weight download,
  weight load and server ready are observed. llama-server prints nothing while it downloads a
  model, so the download is the silent interval before it starts loading, marked `~` (measured:
  10.86 s with the model not cached, 0 s cached). Compile and graph capture are shown as not
  applicable to this engine. The server prints its build only at `-lv 4`, so the report warns
  that the version was not read.
- Engines without a pack (anything other than vLLM 0.32.0 and llama.cpp today) get the
  Kubernetes phases only, with a warning naming the image.

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
- Read-only access: `get` and `list` on pods, pod logs and events in the namespaces you
  profile, and on Deployments, ReplicaSets and StatefulSets to resolve `deploy/` and `sts/` and
  name a pod's owner. Nodes are optional: without them the report omits instance type, zone and
  GPU product, and says so. doriki asks for no write permission and makes no write call.

[`deploy/rbac.yaml`](deploy/rbac.yaml) is that ClusterRole, `doriki-reader`; bind it to whoever
runs doriki:

```sh
kubectl apply -f deploy/rbac.yaml
kubectl create clusterrolebinding doriki-reader --clusterrole=doriki-reader --user=<you>
```

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

v0.1.0 is planned for **18 Nov 2026**: what is above, plus the `capture` command for shareable
redacted bundles and the first five advice rules. The Ollama pack is cut from v0.1: Ollama loads
its model on the first request, after the pod is Ready, which the fixed phase order cannot show
yet ([`DK-PHASE-ENH-20261026-lazy-load-phase-order`](tickets/DK-PHASE-ENH-20261026-lazy-load-phase-order.md)). Everything cut from v0.1, and why, is in
[`docs/v0.2-backlog.md`](docs/v0.2-backlog.md): `diff` with a CI exit code, the Prometheus
exporter, the SGLang pack and the HTML report lead its default order, which v0.1 feedback will
settle.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md). doriki is licensed under
the [Apache License 2.0](LICENSE).
