# doriki

doriki answers the question operators of LLM serving on Kubernetes ask every week: why did this
inference pod take six minutes to become ready, which phase ate the time, and what changed
since it took ninety seconds? It is a kubectl plugin (`kubectl doriki pod/x`) and a single
static binary that reads Kubernetes events, container statuses and inference-engine logs
(vLLM, llama.cpp, Ollama and more), turns them into a per-pod startup waterfall (for example
image pull, weight download, weight load, compile, graph capture), and prints deterministic
advice on which fix applies to that pod. It only reads from the cluster, stores nothing, calls
no SaaS and sends no telemetry.

**Status:** pre-release. Only `doriki version` works today; the first release, v0.1.0, is
planned for 18 Nov 2026.

## Requirements

- Kubernetes **1.29** or newer.
- Read-only access to pods, pod logs and events in the namespaces you profile.

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

## Contributing and security

See [CONTRIBUTING.md](CONTRIBUTING.md) and [SECURITY.md](SECURITY.md). doriki is licensed under
the [Apache License 2.0](LICENSE).
