# doriki: Design Document v0.1

As of 24 Oct 2026 · maintained by the repository owner

## 1. Problem, thesis, and positioning

**doriki answers one question operators ask weekly and currently answer with stopwatches: "why did this inference pod take six minutes to become ready, which phase ate the time, and what changed since it took ninety seconds?"**

**Problem.** LLM serving pods (vLLM, SGLang, llama.cpp, Ollama, TGI-compatible) take 3–11 minutes from Pending to first token. Published measurements from Tensorfuse, H Company, Microsoft, AWS and Google in 2026 each blame a different phase: image pull, weight download, weight load, torch.compile, or CUDA-graph capture. The dominant phase depends on engine version, model size, storage class, node type and cache state. No tool shows an operator a per-pod breakdown, so every team hand-instruments its own stack or guesses which vendor fix to apply.

**Thesis.** The fixes already exist (OCI image volumes, Dragonfly P2P, Run:ai streamer, compile caches, prepull DaemonSets). What is missing is the *measurement* that says which fix applies to *this* pod. doriki is a kubectl plugin that correlates Kubernetes events, container-runtime timings and engine log/trace markers into one waterfall per pod, tags it with a deployment manifest (image digest, model revision, flags, GPU/driver), and emits deterministic advice. It stores nothing, calls no SaaS, and needs no GPU to develop or test.

**Non-goals (v0.x).** No load generation or throughput benchmarking (guidellm, inference-perf, AIPerf, inference-doctor own that). No automatic remediation. No agent or DaemonSet in v0.1 (exporter mode comes later). No support for training jobs. No GUI beyond a static HTML report.

**Prior art and positioning.**

| Project | What it does | Where it stops | Our relation |
| --- | --- | --- | --- |
| [upb-cn/vllm-startup-profiler](https://github.com/upb-cn/vllm-startup-profiler) (MLSys 2026) | Research scripts decomposing vLLM startup into six steps; prediction model | vLLM only, not K8s-aware, not a tool operators run | Validates our phase model; we cite it and reuse its step names |
| H Company vLLM OTel spans ([Booting Fast and Slow](https://hcompany.ai/booting-fast-and-slow)) | Added tracing spans for engine init, weight staging, warmup, graph capture | Internal; may land upstream | If upstream, doriki consumes spans natively; log parsing becomes fallback |
| [inference-doctor](https://pypi.org/project/inference-doctor/) | Baseline-vs-candidate statistical gate on throughput/latency | Does not measure startup; no manifest | Complementary: our startup metric and manifest can feed its compare-runs |
| Vendor write-ups (AWS EKS Auto Mode, GKE, Tensorfuse, Microsoft) | Fixes and one-off measurements | Each measures one stack once | Our rule set encodes their fixes as advice |
| kubectl describe / k9s / Lens | Pod events and status | No engine phases, no timeline, no advice | We build on their data sources |

**Name: `doriki`** (decided 24 Oct 2026). In One Piece, doriki is the unit CP9 uses to measure strength: a number that says what something can actually do instead of a guess. The tool measures a boot instead of guessing at it. Checked 24 Oct: krew index, Homebrew core, npm and PyPI are free; GitHub has one unrelated zero-star repo of the same name, no conflict for `github.com/<owner>/doriki`. The binary and kubectl plugin are `doriki` (`kubectl doriki pod/x`).

## 2. Users and the one-command experience

**The whole product is one command that returns a waterfall in under ten seconds against a live or recently-finished pod.**

**Primary user.** A platform or SRE engineer running LLM inference on Kubernetes (self-managed, EKS, GKE, AKS, on-prem) who owns scaling, cost and on-call for serving. Secondary: an ML engineer debugging a slow deployment; a homelab operator running Ollama/llama.cpp on k3s.

**Core scenarios.**

1. **Diagnose one slow pod.** `kubectl doriki pod/vllm-7c9f` prints a phase waterfall, totals, and up to three advice lines.
2. **Watch a pod boot live.** `kubectl doriki pod/vllm-7c9f --follow` streams phases as they complete, so an operator does not stare at `kubectl logs -f`.
3. **Compare two boots.** `kubectl doriki diff a.json b.json` shows phase deltas side by side and the manifest fields that differ (image digest, flags, node, driver, cache state).
4. **Record for later.** `--output json` writes a self-contained report; teams attach it to tickets or commit it as a fixture.
5. **Fleet view (v0.3+).** `doriki export` runs as a Deployment and publishes `doriki_phase_seconds` histograms to Prometheus for every inference pod matching a label selector.

**Terminal output** (real: the vLLM 0.32 CPU fixture, recorded as `internal/cli/testdata/vllm-0.32.txt`; advice rules arrive in M3, so it has no Advice block).

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

**What "done" feels like.** An operator with no prior knowledge of the tool installs it with `kubectl krew install doriki`, runs it against a pod, and knows within one screen what to fix first.

## 3. Architecture

**A one-way pipeline: collectors produce events, a pure core turns them into phases and advice, renderers print the report; nothing in the core knows about Kubernetes.**

```text
collectors (K8s events/status, CRI, engine logs via packs, OTel spans [v0.2], --from-files)
        |
        v
   normalizer (clock skew, sort)  --> phase engine (fixed order, gaps) --> rules + manifest
        [pure core: no I/O, no clock, tested from fixtures]                       |
                                                                                  v
                                                  renderers: terminal waterfall, JSON, HTML/Prometheus
```

Collectors run concurrently and each returns a plain list of events plus any warnings about its inputs (such as log lines with no timestamp); the normalizer merges and clock-aligns them; the phase engine and rules are deterministic functions over that list, which is why every fixture test exercises the real code path. The `--from-files` collector replaces the live ones with saved objects, so offline mode and live mode share every line of code after collection.

**Runtime shape.** v0.1 is a process that starts, collects, prints and exits. `--follow` keeps the engine-log collector open and re-runs the pure core on every new marker. The v0.3 exporter is the same pipeline in a loop over a label selector, with the Prometheus renderer attached.

## 4. Data model

**Everything flows through one normalized `Event` type; phases, manifest and report are derived from it, and every schema is versioned JSON so reports stay diffable and committable.**

### 4.1 Event (internal)

```go
type Source string // "k8s-event" | "container-status" | "cri" | "engine-log" | "otel-span" | "manifest"

type Event struct {
    TS       time.Time         // monotonic-corrected wall clock, UTC
    Source   Source
    Kind     string            // e.g. "Scheduled", "Pulling", "Pulled", "log.marker"
    Marker   string            // parser-pack marker id, e.g. "vllm.weights.load.end"
    Attrs    map[string]string // raw key/values: image, size_bytes, model, cache="miss"
    Raw      string            // original line or event message, truncated to 512 bytes
    Confidence float32         // 1.0 for K8s/CRI; parser packs may set lower
}
```

Timestamps from different sources are not on the same clock. The normalizer anchors engine-log timestamps to the container `started` time from the container status: if the first engine log line is stamped before `started`, which is impossible, every engine-log event is shifted later by that difference, and a first line after `started` is left alone because that time is real `container-init`, so a log clock running ahead is not detected from this anchor. Skew above 2 s is reported as a warning in the report.

### 4.2 Phase model

Phases are a fixed ordered set. Each is defined by a start marker and an end marker; a phase with no observed markers is `absent`, a phase with a start but no end at report time is `open`.

| Phase | Starts at | Ends at | Primary source |
| --- | --- | --- | --- |
| `scheduling` | Pod `creationTimestamp` | Event `Scheduled`, or the pod's `PodScheduled` condition when the event has expired | k8s-event, container-status |
| `node-provision` | `Scheduled` on a node whose `Ready` condition is newer than the pod | Node `Ready` | k8s-event (optional) |
| `image-pull` | Event `Pulling` | Event `Pulled` | k8s-event, cri |
| `container-init` | Container `started` | first engine log line | container-status, engine-log |
| `engine-init` | first engine log line | marker `*.engine.init.end` (decision 0006) | engine-log |
| `weight-fetch` | marker `*.weights.fetch.start` | marker `*.weights.fetch.end` | engine-log |
| `weight-load` | marker `*.weights.load.start` | marker `*.weights.load.end` | engine-log |
| `compile` | marker `*.compile.start` | marker `*.compile.end` | engine-log |
| `graph-capture` | marker `*.graph.start` | marker `*.graph.end` | engine-log |
| `kv-alloc` | marker `*.kv.start` | marker `*.kv.end` | engine-log |
| `server-ready` | marker `*.server.listening` | readiness probe `Ready=True` | engine-log, container-status |
| `first-token` | `Ready=True` | first `*.request.first_token` marker, or doriki's own probe request when `--probe` is set | engine-log, probe |

Unattributed time between phases is reported as `gap` with the two bounding markers, never silently folded into a neighbor. Overlapping phases (some engines fetch and load concurrently) are rendered as overlapping bars; totals use wall-clock span, not the sum of phases.

### 4.3 Deployment manifest

Captured once per report from the pod spec, the node, and engine startup output. Every field is optional; absent fields are omitted, never null.

```yaml
manifest_version: 1
pod: {namespace: default, name: vllm-qwen3-7c9f, uid: …, owner: Deployment/vllm-qwen3}
node: {name: gpu-a100-3, instance_type: p4d.24xlarge, zone: us-east-1a, kubelet: v1.33.2}
container:
  image: ghcr.io/org/vllm-openai:0.28.1
  image_digest: sha256:3f9a…            # resolved from containerStatuses.imageID
  image_size_bytes: 11203487201          # from CRI when available
  args: ["--model", "Qwen/Qwen3-35B-A3B", "--tensor-parallel-size", "2", …]
  env_allowlist: {VLLM_CACHE_ROOT: /tmp/cache, HF_HUB_OFFLINE: "0"}   # allowlisted keys only, never secrets
engine: {name: vllm, version: 0.28.1, parser_pack: vllm/0.28}
model: {ref: "Qwen/Qwen3-35B-A3B", revision: 8d1c…, source: huggingface.co, dtype: fp8, params_b: 35}
accelerator: {vendor: nvidia, product: A100-SXM4-80GB, count: 2, driver: 570.86, cuda: 12.8, mig: none}
caches: {torch_compile: miss, cuda_graph: miss, hf_hub: partial}
fingerprint: sha256:…   # hash of the fields above excluding pod.name/uid and timestamps
```

`fingerprint` is what `diff` uses to say "these two boots are the same deployment" or to list exactly which fields changed. Env capture is allowlist-only (`VLLM_*`, `HF_*`, `SGLANG_*`, `OLLAMA_*`, `CUDA_*`, `NCCL_*`), and values matching secret patterns are redacted even inside the allowlist.

### 4.4 Report

```json
{
  "report_version": 1,
  "generated_at": "2026-10-24T18:02:11Z",
  "doriki_version": "0.1.0",
  "manifest": { … },
  "phases": [ {"name": "image-pull", "start": "…", "end": "…", "seconds": 161.4, "state": "complete", "attrs": {…}, "sources": ["k8s-event"]} ],
  "gaps": [ {"after": "image-pull", "before": "weight-fetch", "seconds": 4.1} ],
  "totals": {"pending_to_ready_seconds": 372.0, "pending_to_first_token_seconds": 385.2},
  "advice": [ {"rule": "pull-dominant", "severity": "high", "text": "…", "evidence": {"share": 0.43}} ],
  "warnings": [ {"code": "clock-skew", "text": "engine log clock is 3.2s ahead of kubelet"} ],
  "events": [ … ]            // included only with --include-events
}
```

The report is the contract. Renderers (terminal, HTML, Prometheus) consume only the report, never events directly, so a report produced by v0.1 renders identically in v0.5.

## 5. Parser packs

**Engine log knowledge lives in YAML files, not Go, so a contributor adds support for a new engine version with one YAML file and one fixture log, and no recompile.**

### 5.1 Why packs are data

Log formats drift every engine release; vLLM renamed metrics and reworded startup lines several times in 2026. If patterns were Go code, every drift would need a Go contributor and a release. As YAML, a user who hits an unsupported version writes a pack, runs `doriki pack test`, and opens a PR; the maintainer reviews a diff of regexes and a fixture.

### 5.2 Pack schema

```yaml
pack_version: 1
engine: vllm
versions: ">=0.27.0 <0.30.0"        # semver range this pack is tested against
detect:                                # how doriki picks this pack for a pod
  image_regex: "vllm/vllm-openai|vllm-openai"
  log_regex: "^INFO .* vLLM API server version (?P<version>[0-9.]+)"
timestamp:
  regex: "^(?P<level>[A-Z]+) (?P<ts>\\d{2}-\\d{2} \\d{2}:\\d{2}:\\d{2})"
  layout: "01-02 15:04:05"              # Go layout; year inferred from container start
markers:
  - id: vllm.weights.load.start
    regex: "Loading weights took|Starting to load model (?P<model>\\S+)"
    confidence: 1.0
  - id: vllm.weights.load.end
    regex: "Model loading took (?P<gib>[0-9.]+) GiB and (?P<seconds>[0-9.]+) seconds"
    attrs: {seconds: seconds, gib: gib}
  - id: vllm.compile.start
    regex: "Start compiling function|torch.compile\\(\\) starting"
  - id: vllm.compile.end
    regex: "Compiling a graph for .* took (?P<seconds>[0-9.]+) s|Using cache directory: (?P<dir>\\S+) for vLLM's torch.compile"
    attrs: {cache: "hit-if-matched:Using cache directory"}
  - id: vllm.graph.start
    regex: "Capturing CUDA graphs"
  - id: vllm.graph.end
    regex: "Graph capturing finished in (?P<seconds>[0-9.]+) secs"
  - id: vllm.kv.end
    regex: "GPU KV cache size: (?P<tokens>[0-9,]+) tokens"
  - id: vllm.server.listening
    regex: "Uvicorn running on|Application startup complete"
facts:                                  # one-shot manifest facts pulled from logs
  - key: engine.version
    regex: "vLLM API server version (?P<v>[0-9.]+)"
  - key: accelerator.cuda
    regex: "CUDA (?P<v>[0-9.]+)"
```

Rules: every regex uses Go RE2 syntax; named groups become `Event.Attrs`; a marker may list several alternatives; `confidence` below 1.0 marks heuristic matches, which the renderer shows with a `~`. Packs are embedded in the binary at build time (`//go:embed packs/**`) and can be overridden or extended from `~/.config/doriki/packs/` or `--pack-dir`.

### 5.3 Selection

`detect.image_regex` is tried against the container image; if several packs match, `detect.log_regex` on the first 200 log lines disambiguates and extracts the version; the pack whose `versions` range contains it wins. No match falls back to the `generic` pack (K8s and CRI phases only, no engine phases) with a warning naming the image so users know which pack to write.

### 5.4 Testing a pack

Every pack ships with at least one fixture under `packs/<engine>/testdata/<version>/`: a `pod.json` (pod object), `events.json`, `container.log`, and `expected.json` (the report's `phases` and `manifest` sections). `go test ./packs/...` runs every fixture through the full pipeline and diffs the result against `expected.json`. `doriki pack test ./my-pack.yaml --log container.log` runs the same check outside the repo. A pack PR is accepted only with a fixture captured from a real run; synthetic logs are rejected.

### 5.5 Initial packs (v0.1)

| Pack | Versions | Fixture source |
| --- | --- | --- |
| `vllm` | 0.32.0 (CPU fixture; widened as GPU fixtures arrive) | CPU run in `kind` (M1b); Kaggle/Colab GPU runs and community donations |
| `sglang` | 0.5.x | same |
| `llamacpp` | server b4xxx+ | CPU run in `kind`, used for e2e |
| `ollama` | 0.6+ | CPU run in `kind` |
| `generic` | any | none (K8s phases only) |

TGI is intentionally absent: the project was archived in March 2026.

## 6. Phase engine and rules

**Phase assignment is a pure function of the sorted event list, and advice is a pure function of the report, so both are fully testable from fixtures and never depend on a live cluster.**

### 6.1 Phase assignment algorithm

1. Collect events from all collectors, normalize timestamps to UTC and correct engine-log clock skew (section 4.1).
2. Sort by timestamp; ties broken by source priority `k8s-event > container-status > cri > otel-span > engine-log`.
3. For each phase in the fixed order, find its first start marker at or after the previous phase's start, and its first end marker after that start. Missing start → `absent`; missing end → `open`.
4. A phase whose end precedes its start by more than 1 s (clock bug or restart) is marked `invalid` and listed in warnings; it never contributes to totals.
5. Compute `gaps` between consecutive complete phases when the gap exceeds 1 s.
6. Totals: `pending_to_ready = Ready(True) − creationTimestamp`; `pending_to_first_token = first_token − creationTimestamp` when observed.
7. Container restarts: each `restartCount` increment opens a new boot attempt; the report covers the latest attempt by default, `--attempt N` selects another, and `--all-attempts` lists them.

### 6.2 Advice rules

Rules are Go functions over the report, each returning zero or one `Advice`. v0.1 ships these; each cites the public write-up its fix comes from.

| Rule id | Trigger | Advice (abridged) |
| --- | --- | --- |
| `pull-dominant` | `image-pull` ≥ 35% of total and image not previously on node | Prepull DaemonSet; OCI image volume (K8s ≥1.31); SOCI/estargz lazy pull; slim the image (vLLM images are 8–18 GB) |
| `pull-small-node` | `image-pull` ≥ 35% and node vCPU < 16 | Decompression is CPU-bound; larger node or pre-decompressed image |
| `fetch-remote-hub` | `weight-fetch` ≥ 25% and source host is `huggingface.co` | Cluster-local mirror, Dragonfly `hf://`, or ModelCar/OCI artifact; pin `revision` |
| `fetch-rate-limited` | log contains HTTP 429 from the Hub | Shared egress IP hit the 5-minute window; stagger ranks or use a mirror |
| `compile-miss` | `compile` ≥ 20% and `caches.torch_compile == miss` | Persist `VLLM_CACHE_ROOT` on a PVC; keep cache key stable (GPU SKU, TP, dtype, engine version) |
| `graph-heavy` | `graph-capture` ≥ 20% | Reduce `cuda_graph_sizes`/batch sizes captured, or enable graph cache where the engine supports it |
| `load-slow-storage` | `weight-load` throughput < 0.5 GB/s | Weights on network disk; move to local NVMe or use a streaming loader (Run:ai streamer, tensorizer) |
| `provision-dominant` | `node-provision` ≥ 30% | Warm node pool / overprovisioning; pre-compiled driver images (EKS Auto Mode-style) |
| `probe-late` | `server-ready` > 20 s after listening | Readiness probe period/initialDelay too long; tune probe |
| `restart-loop` | restartCount ≥ 2 within the window | Each restart repays compile/graph; find the crash before tuning startup |
| `scale-thrash` | owner's HPA/KEDA cooldown < observed boot time | Set cooldown ≥ p95 boot time or the autoscaler will scale down and immediately back up |

Severity: `high` when the triggering share ≥ 40%, otherwise `medium`; at most three advice lines are printed in the terminal (all are in JSON). Rules never claim an effect size; they say what to try and why.

### 6.3 What the engine deliberately does not do

No machine learning, no prediction of boot time for unseen configs (the MLSys paper's predictor is out of scope), no automatic changes to the cluster. Every number in the report traces to a specific event the user can see with `--include-events`.

## 7. CLI surface

**One binary, `doriki`, that also works as `kubectl doriki`; every command reads the cluster or files, never writes to the cluster.**

### 7.1 Commands

| Command | Purpose | v |
| --- | --- | --- |
| `doriki pod/<name> [-n ns]` | Profile one pod (default command; also `doriki profile`) | 0.1 |
| `doriki deploy/<name>` | Profile the newest pod of a Deployment/StatefulSet/InferenceService | 0.1 |
| `doriki --follow pod/<name>` | Stream phases live until first token or timeout | 0.1 |
| `doriki --from-files pod.json events.json container.log` | Offline mode: build a report from saved objects (used by tests and by users sharing data) | 0.1 |
| `doriki diff a.json b.json` | Phase deltas and manifest diff between two reports | 0.2 |
| `doriki pack list / test / detect` | Inspect embedded packs; test a pack against a log; show which pack a pod would use | 0.1 |
| `doriki capture pod/<name> -o dir/` | Save the raw inputs (pod, events, logs, node) as a shareable, redacted bundle | 0.1 |
| `doriki export --selector app=vllm` | Long-running Prometheus exporter for a fleet | 0.3 |
| `doriki version` | Version, commit, embedded pack versions | 0.1 |

### 7.2 Global flags

Standard kubectl flags via `cli-runtime` (`--kubeconfig`, `--context`, `-n`), plus `-o text|json|html` (default `text`), `--include-events`, `--attempt N`, `--all-attempts`, `--probe` (send one tiny completion request to measure first-token; off by default because it touches the workload), `--pack-dir`, `--no-advice`, `--since 24h` (how far back to search events; K8s event retention is typically 1 h, which is a documented limitation), `--timeout` for `--follow`, `--redact-level none|default|strict`.

### 7.3 Exit codes

| Code | Meaning |
| --- | --- |
| 0 | Report produced, all requested phases complete |
| 1 | Report produced with `open` or `absent` phases (pod still booting, or events expired); phases the selected pack cannot produce are listed as `absent` but do not count (decision 0003) |
| 2 | Pod not found / no permission / unreachable cluster |
| 3 | No parser pack matched and `--require-pack` was set |
| 4 | `diff`: a phase regressed beyond `--fail-on` threshold (e.g. `--fail-on total>+20%`) |
| 64 | Usage error |

Exit 4 is what lets `diff` sit in a CI job as a startup-time gate without any other tooling.

### 7.4 RBAC

Read-only: `get`/`list` on `pods`, `pods/log`, `events`, `nodes`, and the owner kinds (`deployments`, `replicasets` to walk a pod to its Deployment, `statefulsets`, `inferenceservices` if present). The README ships a minimal ClusterRole. `nodes` is optional; without it the manifest omits node fields and a warning says so.

### 7.5 Output rules

Terminal output fits 100 columns, no color when not a TTY or `NO_COLOR` is set, durations as `m:ss`, sizes as GiB with one decimal. JSON is the report schema from section 4.4 and nothing else. HTML is a single self-contained file with inline CSS and no JavaScript, so it can be attached to a ticket.

## 8. Tech stack and repository layout

**Go 1.26, a single static binary, no CGO, no daemon, no database; the Kubernetes ecosystem's defaults everywhere so contributors never learn anything doriki-specific.**

### 8.1 Dependencies (pinned; nothing else without a reason in the PR)

| Need | Library | Why this one |
| --- | --- | --- |
| CLI | `spf13/cobra`, `spf13/pflag` | kubectl plugin convention |
| K8s client | `k8s.io/client-go`, `k8s.io/cli-runtime` | official client; `cli-runtime` gives kubeconfig/context/namespace flags and resource name parsing (`pod/x`, `deploy/x`) for free |
| Image digests | `google/go-containerregistry` | resolve tag → digest, read manifest size without pulling |
| YAML | `go.yaml.in/yaml/v3` | packs |
| Semver | `Masterminds/semver/v3` | pack version ranges |
| Terminal | `charm.land/lipgloss/v2` | bars and tables; no TUI framework |
| HTML | stdlib `html/template` | single-file report, no JS |
| Metrics | `prometheus/client_golang` | exporter (v0.3) |
| Traces | `go.opentelemetry.io/otel` | consume engine spans when present (v0.2) |
| Tests | stdlib `testing`, `stretchr/testify`, `sebdah/goldie` (golden files) |  |
| Lint | `golangci-lint` with `gofmt`, `govet`, `staticcheck`, `errcheck`, `gosec` |  |

No NVML/CUDA bindings in v0.x; accelerator facts come from node labels (`nvidia.com/gpu.product`, GPU Feature Discovery) and engine logs.

### 8.2 Repository layout

```text
doriki/
├── cmd/doriki/main.go          # wires cobra commands; nothing else
├── internal/
│   ├── cli/                      # command definitions, flag parsing, output selection
│   ├── collect/                  # one package per source
│   │   ├── k8s/                  # pods, events, nodes, owners (client-go)
│   │   ├── cri/                  # image size/pull facts via CRI when reachable (optional)
│   │   ├── enginelog/            # log streaming + pack application
│   │   ├── otel/                 # span ingestion (v0.2)
│   │   └── files/                # --from-files loader (same interfaces as live collectors)
│   ├── model/                    # Event, Phase, Manifest, Report types + JSON schema
│   ├── normalize/                # clock alignment, sorting, dedupe
│   ├── phase/                    # phase assignment (pure)
│   ├── manifest/                 # manifest assembly, fingerprint, redaction
│   ├── rules/                    # advice rules (pure), one file per rule
│   ├── render/                   # text, json, html
│   └── diff/                     # report comparison (v0.2)
├── packs/                        # embedded parser packs + fixtures
│   ├── vllm/0.32.yaml
│   ├── vllm/testdata/0.32/{pod.json,events.json,container.log,expected.json}
│   └── ...
├── schema/                       # report.v1.json, manifest.v1.json, pack.v1.json (JSON Schema)
├── test/e2e/                     # kind-based end-to-end (CPU engines)
├── docs/                         # mkdocs or plain markdown; GitHub Pages
├── deploy/                       # ClusterRole, exporter Deployment (v0.3), krew plugin.yaml
├── .github/workflows/            # ci.yml, release.yml, pack-pr.yml
├── .goreleaser.yaml
├── CLAUDE.md                     # agent instructions (section 12)
├── CONTRIBUTING.md  SECURITY.md  CODE_OF_CONDUCT.md  LICENSE (Apache-2.0)
└── README.md
```

### 8.3 Module boundaries (enforced by `depguard` in lint)

- The pure packages (`phase`, `rules`, `diff`, `render`) import only the standard library, `model`, and their declared external libraries; `render`'s is `lipgloss`, and the others declare none. They do no I/O and read no clock; they take a `now` parameter where needed.
- `collect/*` may import any external library but no internal package other than `model`; collectors return `[]model.Event` and know nothing about phases.
- `cli` imports everything.

This is what keeps the core testable from fixtures and lets the exporter (v0.3) reuse `phase` and `rules` unchanged.

## 9. Testing strategy

**No test in the repository requires a GPU, a cloud account, or money; CI runs entirely on free GitHub-hosted runners with recorded fixtures and a CPU-only `kind` cluster.**

### 9.1 Test layers

| Layer | What it covers | Tooling | Runs on |
| --- | --- | --- | --- |
| Unit | normalize, phase, manifest fingerprint, each rule, each renderer | `go test`, table-driven, `testify` | every push, < 30 s |
| Pack golden | every `packs/*/testdata/*` fixture through the full offline pipeline; output diffed against `expected.json` | `goldie`; `-update` flag regenerates after a reviewed change | every push |
| CLI golden | `doriki --from-files … -o text/json/html` output snapshots | `goldie` | every push |
| Schema | every report/manifest/pack in testdata validates against `schema/*.json` | `santhosh-tekuri/jsonschema` | every push |
| E2E | `kind` cluster → deploy llama.cpp server (CPU, tiny GGUF) and Ollama → `doriki deploy/llamacpp` → assert phases `image-pull`, `weight-load`, `server-ready` complete and totals > 0 | `kind`, `kubectl`, Go test harness | PRs to `main`, \~6 min |
| Pack-PR check | a PR touching `packs/` must add or update a fixture; the workflow fails otherwise | `.github/workflows/pack-pr.yml` | pack PRs |

### 9.2 Fixture policy

- Fixtures are real captures, made with `doriki capture` (which redacts) and committed under `packs/<engine>/testdata/<version>/`. Synthetic logs are not accepted for packs.
- Each fixture directory carries a `SOURCE.md`: engine version, image digest, GPU/CPU, who captured it, date. No tokens, no names or hostnames from a real cluster; the defaults of a throwaway local `kind` cluster (node name, private pod and node IPs) are acceptable.
- GPU fixtures for vLLM and SGLang are captured once on free Kaggle/Colab GPU time (or donated by community users) and reused forever; they are the only GPU-derived artifacts in the repo.
- A fixture is never edited by hand. To change expectations, re-run `-update` and explain the diff in the PR.

### 9.3 Clock-skew and ordering tests

Dedicated unit cases: engine log ahead/behind kubelet by 0 s, 1.5 s, 3 s, 60 s; events delivered out of order; duplicate events; container restart mid-boot; events expired (only container statuses present). Each has a defined expected outcome in section 6.1.

### 9.4 Property tests

`phase.Assign` is run with `testing/quick` over random event permutations: output must be deterministic for the same multiset of events, phases must never overlap in a way that violates the fixed order, and totals must never exceed the wall-clock span of the input.

### 9.5 What is not tested automatically

Real GPU timing accuracy. The README states this and asks users to share `doriki capture` bundles; those become fixtures.

## 10. Distribution, releases, security, licensing

**Install paths are the ones kubectl users already use: `krew`, Homebrew, GitHub Releases and `go install`; every artifact is signed and carries an SBOM, which is a differentiator after this year's pip-based supply-chain incidents in AI infra.**

### 10.1 Release pipeline

1. Conventional commits on `main`; `release-please` opens a release PR with the changelog.
2. Merging it tags `vX.Y.Z`; `release.yml` runs `goreleaser` for `linux|darwin|windows × amd64|arm64`, builds a multi-arch image to `ghcr.io/<org>/doriki`, and signs binaries, checksums and image with `cosign` (keyless, GitHub OIDC). SBOM via `syft`, attached to the release.
3. `goreleaser` updates the Homebrew tap and opens the krew index PR (`krew-release-bot`).
4. Pre-1.0: minor versions may break the pack schema; the report schema is append-only from 0.1 and bumps `report_version` only with a migration note.

### 10.2 Install matrix

| Channel | Command | From |
| --- | --- | --- |
| krew | `kubectl krew install doriki` | v0.1 (after index PR merges) |
| Homebrew | `brew install <org>/tap/doriki` | v0.1 |
| GitHub Releases | download + `cosign verify-blob` | v0.1 |
| Go | `go install github.com/<owner>/doriki/cmd/doriki@latest` | v0.1 |
| Container | `ghcr.io/<org>/doriki:<ver>` for CI jobs and the exporter | v0.1 |
| Helm | exporter chart | v0.3 |

### 10.3 Security posture

- Read-only against the cluster; no mutating verbs in the ClusterRole.
- Env capture allowlist + secret-pattern redaction (section 4.3); `--redact-level strict` also hashes hostnames and image registries.
- `doriki capture` bundles are redacted by default and the README says what is and is not removed.
- No network calls except the Kubernetes API and, only with `--probe`, one request to the pod's own service; no telemetry, no update checks.
- `SECURITY.md` with private vulnerability reporting enabled on GitHub; `gosec` in lint; Dependabot for Go modules and Actions, with Actions pinned to commit SHAs.
- Release provenance: SLSA level 3 via the `slsa-github-generator` once the pipeline is stable (v0.2 target).

### 10.4 License and governance

Apache-2.0 (explicit patent grant; matches most CNCF tooling). Contributor DCO sign-off instead of a CLA. `CONTRIBUTING.md` states: PRs may be written with AI tools, but every PR needs a human-written summary and the author must have run the tests locally; PRs that only reformat or that add packs without real fixtures are closed. One maintainer initially; add a `MAINTAINERS.md` and a second maintainer before v1.0.

## 11. Milestones

**Hard end date 18 Nov 2026: a signed v0.1.0 on GitHub Releases and Homebrew with the krew index PR opened; one week to a postable offline pipeline, a gate that decides scope but never the date, then two weeks of live collectors and release engineering.**

```text
Oct 27 ───── Nov 3 ───── Nov 10 ───── Nov 17 ─┐
[M0+M1 skeleton, offline pipeline ]           │
                  ◆ validation gate (3 Nov)   │
                  [M2 live collectors, e2e ]  │
                             [M3 manifest, rules, release pipe ]
                                                    ◆ v0.1.0 released (18 Nov)
```

The gate on 3 Nov can cut scope further or change a pack priority; it cannot move 18 Nov. Work that does not fit is written to `docs/v0.2-backlog.md` the same day it is cut, so nothing is lost silently. Roles: the owner approves plans at checkpoints, answers `[decision]`s within one day, reviews diffs, pushes and merges; the architect (this conversation) keeps `DESIGN.md` and the task files current and reviews every Claude Code plan and report before the owner sees it; Claude Code implements one task file at a time and stops at the method's checkpoints.

| Milestone | Dates | Scope | Acceptance (all must pass) |
| --- | --- | --- | --- |
| M0+M1 skeleton and offline pipeline | 27 Oct – 2 Nov | Repo layout (8.2), `go.mod`, cobra root, `make build/test/lint/gate`, CI on push; `PROJECT.md`, `DESIGN.md`, `CLAUDE.md` committed; METHOD.md and the task file kept outside version control; name decided; `model`, `normalize`, `phase`, `files` collector; `vllm/0.28` pack with one real fixture; text + JSON renderers; golden tests; schema validation | `make gate` green on a clean clone; `doriki --from-files <fixture> -o text` matches the golden; JSON validates against `schema/report.v1.json`; phase property test passes 1,000 iterations; README has a real screenshot |
| Validation gate | 3 Nov | 60-second asciinema; RFC post in vLLM and SGLang discussions and r/LocalLLaMA; request for `capture` bundles | Decision recorded in `docs/decisions/0002-gate.md`: continue as planned, cut further, or reorder packs. Responses are read daily until 18 Nov; the gate never stops the build |
| M2 live collectors and e2e | 3 Nov – 9 Nov | `k8s` collector (pods, events, owners, nodes), `enginelog` streaming, `--follow`, `llamacpp` and `ollama` packs, `generic` fallback, `kind` e2e, clock-skew tests, RBAC manifest | `make e2e` passes on a GitHub-hosted runner in < 8 min; profiling a CPU llama.cpp pod shows `image-pull`, `weight-load`, `server-ready` complete; `--follow` ends on `server-ready` |
| M3 manifest, rules, release pipeline | 10 Nov – 16 Nov | `manifest` package with fingerprint and redaction; `capture` command; the five advice rules `pull-dominant`, `fetch-remote-hub`, `compile-miss`, `load-slow-storage`, `restart-loop` with unit tests; goreleaser + cosign + SBOM; Homebrew tap; krew manifest | `cosign verify-blob` succeeds on a release candidate; `kubectl krew install --manifest=deploy/krew.yaml` works; redaction test proves no secret value reaches disk; `brew install` from the tap works on macOS and Linux |
| v0.1.0 released | 18 Nov | Tag, release notes, krew index PR opened, announcement post | Install from Homebrew on a fresh machine and profile a pod; GitHub release shows signed assets and SBOM; krew PR link in the release notes |

**Cut to v0.2 (recorded in `docs/v0.2-backlog.md` at M0):** `diff` with `--fail-on`, OTel span collector, `sglang` pack, HTML renderer, the other six advice rules (`pull-small-node`, `fetch-rate-limited`, `graph-heavy`, `provision-dominant`, `probe-late`, `scale-thrash`), exporter mode and Helm chart, SLSA provenance, docs site. 17 Nov is a buffer day; if it is not needed, the first v0.2 item (`diff`) starts.

**What slips first if a week runs long, in order:** Homebrew tap (GitHub Releases + `go install` still ship) → `ollama` pack → `capture` command. The vLLM pack, the live collectors, the signed release and the five rules do not slip; if they cannot fit, the owner is told by 12 Nov, not on 17 Nov.

After 18 Nov: read gate responses and v0.1.0 feedback for one week before choosing the v0.2 order; `diff` and the exporter are the default first two unless users say otherwise.

## 12. Risks, open questions, and instructions for Claude Code

**The two risks that can sink the project are log-format drift and building for six weeks without talking to a user; both have a mitigation written into the plan.**

### 12.1 Risks

| Risk | Likelihood | Mitigation |
| --- | --- | --- |
| Engine log formats drift every release | High | Packs as data; fixture-gated PRs; `generic` fallback never breaks; consume OTel spans when engines emit them (v0.2) |
| vLLM/SGLang ship native startup spans + Grafana dashboards | Medium | doriki becomes the correlator across K8s + CRI + spans + manifest; spans reduce our parsing burden rather than replace the tool |
| Kubernetes event retention (\~1 h) loses early phases | High for old pods | Document it; `--follow` for live boots; exporter mode (v0.3) records continuously |
| CRI access unavailable on managed clusters | Medium | CRI collector is optional; image size falls back to registry manifest via go-containerregistry |
| Nobody cares | Unknown | Validation gate at week 2 (section 11); if under \~10 real responses after posting, re-evaluate before building v0.2 |
| Solo-maintainer bus factor | Certain at start | Small surface, boring stack, exhaustive fixtures so a second maintainer can take over; recruit one from the first pack contributors |

### 12.2 Open questions (decide before the matching milestone)

- [x] Final project name and GitHub org: `doriki`, under the owner's personal GitHub account (24 Oct 2026)
- [x] Minimum Kubernetes version: 1.29 (24 Oct 2026)
- [x] v0.1 advice rules confirmed: `pull-dominant`, `fetch-remote-hub`, `compile-miss`, `load-slow-storage`, `restart-loop`; the other six are v0.2 because they need data v0.1 does not collect (24 Oct 2026, architect's call)
- [ ] Whether `--probe` sends a `/v1/completions` or `/v1/chat/completions` request, and with which 1-token prompt (before M3)
- [ ] HTML report: cut to v0.2, so no decision needed for v0.1
