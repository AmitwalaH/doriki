# doriki

doriki answers the question operators of LLM serving on Kubernetes ask every week: why did this
inference pod take six minutes to become ready, and which phase ate the time? It reads a pod's
Kubernetes events, its container statuses and the inference engine's own log, turns them into a
startup waterfall (scheduling, image pull, engine start-up, weight download, weight load,
compile, graph capture, KV cache, server ready) and prints up to three lines of advice on
which fix applies. It is a single static binary that also runs as a kubectl plugin, stores nothing, calls
no SaaS and sends no telemetry.

**doriki measures and advises; it never changes the cluster.**

**Status:** pre-release; v0.1.0 is planned for 18 Nov 2026, with a release candidate first.
doriki profiles a live pod, Deployment or StatefulSet, once or with `--follow` while it boots,
builds the same report from saved files (`--from-files`), ends it with advice, saves a redacted
bundle to share (`capture`) and checks a parser pack against a log (`pack test`).

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

Advice
  1. pull-dominant: image-pull took 59% of startup (268 s of 456 s) and the image was not on the
     node; prepull it with a DaemonSet or pull lazily with SOCI or estargz (image volumes are not
     suggested: the node's kubelet version could not be read).
```

Bars are placed by when each phase ran and sized by its share of the time from pod creation to
Ready; `gap` rows are time no phase accounts for. `~` marks an estimate: vLLM logs only how long
the weight download and load took, so their starts are computed from those durations. The
Advice block names the phase that dominated and a fix that applies to this cluster: here the
4:28 image pull. Image volumes are suggested only on a node whose kubelet is 1.35 or newer,
where they are on by default; these saved files hold no node object, so the advice says the
version could not be read. `-o json` writes the same report as JSON, advice included, with every
event behind it under `--include-events`.

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

### Advice

At most three lines are printed, highest share first; `-o json` carries every one, each with
the numbers it used (`evidence`) and the setting to change (`fix`). Severity is `high` when the
rule's share of the time to Ready is 40% or more, otherwise `medium`. `--no-advice` runs no rules.
Advice says what to try and why; it never claims how much time a fix will save.

| Rule | Fires when | Suggests |
|---|---|---|
| `pull-dominant` | image-pull is 35% or more of the time to Ready and the image was not already on the node | a prepull DaemonSet; lazy pulling (SOCI, estargz); an image volume for the weights, only from kubelet 1.35 |
| `fetch-remote-hub` | weight-fetch is 25% or more and the model comes from huggingface.co | a cluster-local mirror (`HF_ENDPOINT` under vLLM), Dragonfly's `hf://` proxy or a ModelCar image; under vLLM, pin `--revision` |
| `compile-miss` | compile is 20% or more and the engine logged a torch.compile cache miss | `VLLM_CACHE_ROOT` on a PersistentVolumeClaim; the same GPU type, tensor parallel size, dtype and engine version between boots |
| `load-slow-storage` | weights load slower than 0.5 GB/s, the size estimated from the engine's loaded-weight memory | weights on local NVMe; under vLLM, `--load-format runai_streamer` or `tensorizer` |
| `restart-loop` | the container restarted two or more times | find the crash first: `kubectl logs --previous` |

An estimate (`~`) in a phase the rule used is called estimated in its advice.

### Sharing a boot: `capture`

```sh
doriki capture pod/vllm-7c9f -n serving --out vllm-boot/                        # redacted bundle
doriki capture pod/vllm-7c9f -n serving --out vllm-boot/ --redact-level strict  # names hashed too
doriki --from-files vllm-boot/                                                  # the same report again
```

`capture` writes `pod.json`, `events.json`, `container.log` (and `container-previous.log` after a
restart), `node.json`, `manifest.json`, `report.json` and a `SOURCE.md` saying what was removed.
`--out` must be a new or empty directory. Everything is redacted before it is written:

- **Removed at every level:** environment variables outside the allowlist (`VLLM_*`, `HF_*`,
  `SGLANG_*`, `OLLAMA_*`, `LLAMA_*`, `CUDA_*`, `NCCL_*`); the value of a variable, flag or log
  key whose name holds a secret word (such as token, key, secret, password, credential, auth); values shaped
  like a known token (Hugging Face, OpenAI-style, GitHub, Slack, AWS key ids, JWTs) in every file
  and log line; kubectl's last-applied annotation; everything in `node.json` but the name,
  labels, system info (kubelet version; not the machine ID, system UUID or boot ID) and capacity.
- **Also with `--redact-level strict`:** pod, namespace, owner and node names and each image's
  registry and org are replaced by salted hashes, the same in every file; the image name is kept
  only when a parser pack recognises it. IP addresses are dropped. The fingerprint in the bundle
  then differs from a live report's.
- **Removed on purpose, though not secret:** the value after a log key ending in `token`, such
  as llama.cpp's tokenizer ids: `BOS token = 1 '<s>'` is written as `BOS token = [redacted]
  '<s>'`, in the bundle and in the log lines a report shows (`--include-events`). Redaction
  errs on the safe side here; no parser pack reads those lines.
- **Not removed:** a secret with no known shape that the engine prints without a secret-named
  key in front of it. Read `container.log` before you share it.

### Checking a parser pack: `pack test`

```sh
doriki pack test packs/vllm/0.32.yaml --log container.log
doriki pack test my-pack.yaml --log container.log --pod pod.json --events events.json
```

It runs the pack against a log alone, with no cluster: every marker and fact is listed with the
time and text of the first line it matched, or `no match`. It exits `1` when a tested marker
(confidence 1.0) or tested fact matched nothing, and flags an untested one that matched, which is how a
log from a GPU machine turns an untested marker into a tested one. Times come from the log's
`--timestamps`, or else from the engine's own in-line times. A marker or fact the pack gates
by log level (`min_log_level`, such as llama.cpp's `-lv 4` lines) is listed as `skipped` when
the log's level is lower, and the output says the level it read.

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
- **llama.cpp server build 11889 on CPU, at its default log level and at `-lv 4`.** Two real
  captures in `kind` ([default](packs/llamacpp/testdata/11889/SOURCE.md),
  [`-lv 4`](packs/llamacpp/testdata/11889-lv4/SOURCE.md), the second taken with `doriki capture`
  itself): engine start-up, weight download, weight load and server ready are observed.
  llama-server prints nothing while it downloads a model, so the download is the silent interval
  before it starts loading, marked `~` (measured: 10.86 s with the model not cached, 0 s cached).
  Compile and graph capture are shown as not applicable to this engine. At the default level the
  server does not print its build, so the report warns that the version was not read. At
  `-lv 4` it does: the report names build 11889 with no warning, weight-load ends where the
  context is built, and kv-alloc is observed, with the KV buffer size on the terminal (as GiB
  with one decimal, so the test model's 1.69 MiB reads `<0.1 GiB`) and, as logged, in the JSON
  report.
  With llama.cpp's default mmap load on Linux the model file is read in full before the context
  is built, so weight-load includes the disk read; with `--numa`, or a tensor read lazily
  (`--lazy-mode`), pages are read on demand later and that time is not in weight-load.
- Engines without a pack (anything other than vLLM 0.32.0 and llama.cpp today) get the
  Kubernetes phases only, with a warning naming the image.
- **Advice: only `pull-dominant` has fired on a real capture** (vLLM 59%, llama.cpp 91% and 95%
  at `-lv 4`, the nginx capture 95%). The `-lv 4` llama.cpp capture is the only one that holds a
  node object; there it read kubelet v1.37.0 and printed: "image-pull took 95% of startup (275 s
  of 290 s) and the image was not on the node; move the model weights into an image volume,
  prepull the image with a DaemonSet, or pull lazily with SOCI or estargz." On the others the
  kubelet version is unread, so image volumes are not suggested. The other four rules are tested
  on hand-made reports only:
  `fetch-remote-hub` (the vLLM capture's download is 23.7% of its boot, just under 25%),
  `restart-loop` (the restart capture restarted once, one short of the trigger), and
  `compile-miss` and `load-slow-storage`, which need the torch.compile cache and loaded-memory
  lines vLLM prints only on GPU; the pack reads them as written in the vLLM 0.32.0 source, but
  no capture has exercised them.

### Please send a capture from a GPU vLLM pod

One real startup from a GPU pod turns those unverified phases and rules into tested ones. Right
after the pod becomes Ready (Kubernetes keeps events for about an hour), run `capture`, read
`container.log` and attach the directory to an issue, with the vLLM version, GPU model and
image:

```sh
doriki capture pod/<your vLLM pod> -n <namespace> --out vllm-gpu-capture/
```

With a doriki older than v0.1.0-rc.1, save the three files with kubectl instead:

```sh
POD=<your vLLM pod>
kubectl logs "$POD" --timestamps > container.log
kubectl get pod "$POD" -o json > pod.json
kubectl get events --field-selector involvedObject.name="$POD" -o json > events.json
```

and redact them by hand before sharing, saying in the issue what you changed:

- **Secrets:** in `pod.json`, the values of any secret-looking `env` entries (`HF_TOKEN`, API
  keys, passwords) and any `--api-key` in `args`; search all three files with
  `grep -n -i -E 'token|key|secret|password' pod.json events.json container.log` and replace
  each secret value with `REDACTED`.
- **Names, if you need to:** pod, namespace, node, registry or model names may be replaced, but
  replace each one the same way everywhere it appears.
- **Never** change a timestamp, reorder lines or delete log lines: the timing is what doriki
  measures.

A log alone, from a machine with no Kubernetes (a notebook, `docker logs`), still helps:
`doriki pack test packs/vllm/0.32.yaml --log container.log` shows which untested markers it
matches.

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

| Channel | Command | Status |
|---|---|---|
| GitHub Releases | download an archive and `checksums.txt`, then verify (below) | available after v0.1.0-rc.1 |
| Homebrew | `brew install AmitwalaH/tap/doriki` | available after v0.1.0-rc.1 |
| Container | `docker run --rm ghcr.io/amitwalah/doriki:0.1.0-rc.1 version` | available after v0.1.0-rc.1 |
| Go | `go install github.com/AmitwalaH/doriki/cmd/doriki@v0.1.0-rc.1` | available after v0.1.0-rc.1 |
| krew | `kubectl krew install doriki` | after v0.1.0, once the krew-index pull request is merged |

Every release's `checksums.txt` is signed with cosign, keyless, by the release workflow; each
archive also has an SPDX SBOM. To verify a download:

```sh
cosign verify-blob \
  --certificate-identity 'https://github.com/AmitwalaH/doriki/.github/workflows/release.yml@refs/tags/v0.1.0-rc.1' \
  --certificate-oidc-issuer 'https://token.actions.githubusercontent.com' \
  --bundle checksums.txt.sigstore.json checksums.txt
sha256sum --check --ignore-missing checksums.txt
```

A binary built with `go install` carries no release stamp, so `doriki version` says `dev`.

On macOS the Homebrew cask removes the download quarantine flag from the doriki binary at
install, because the binary is signed with cosign but not notarized by Apple; without that,
macOS would refuse to run it.

Or build from source with Go 1.26:

```sh
git clone https://github.com/AmitwalaH/doriki.git
cd doriki
make build
./bin/doriki version
```

## Roadmap

v0.1.0 is planned for **18 Nov 2026**: what is above, after a release candidate and a period
of validation on real clusters. The Ollama pack is cut from v0.1: Ollama loads
its model on the first request, after the pod is Ready, which the fixed phase order cannot show
yet ([`DK-PHASE-ENH-20261026-lazy-load-phase-order`](tickets/DK-PHASE-ENH-20261026-lazy-load-phase-order.md)). Everything cut from v0.1, and why, is in
[`docs/v0.2-backlog.md`](docs/v0.2-backlog.md): `diff` with a CI exit code, the Prometheus
exporter, the SGLang pack and the HTML report lead its default order, which v0.1 feedback will
settle.

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md). doriki is licensed under
the [Apache License 2.0](LICENSE).
