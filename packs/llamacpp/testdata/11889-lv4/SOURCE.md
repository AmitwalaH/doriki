# llama.cpp build 11889 at -lv 4: CPU fixture from kind, captured with doriki capture

A real capture of the llama.cpp server serving a tiny GGUF on CPU in a local, throwaway `kind`
cluster, at log level 4 (`-lv 4`), taken with `doriki capture` itself: the first fixture the
tool captured. The image was not on the node, so the capture includes a real pull (`Pulling`
12:19:48, `Pulled` 12:24:23, 95% of the 290 s to Ready), and the model was downloaded from
huggingface.co inside the pod by the server itself (`--hf-repo`). Same image, model and flags
as `../11889/`, plus `-lv 4`.

The last section of this file, from "# doriki capture" on, is the SOURCE.md `doriki capture`
wrote, unchanged. Everything above it is hand-written in the format of the other fixtures.

- **Engine:** llama.cpp server, build 11889 (commit d854d98ba), as line 5 of the log says
- **Image:** `ghcr.io/ggml-org/llama.cpp:server`, pulled by the index digest
  `sha256:5741ab30aae42af23ca57d9b98a6d4544cb09c21f372f92c5ee55312fa8b8b0c` (the
  `containerStatuses[0].imageID` is the same digest); linux/amd64
- **Model:** `ggml-org/models`, file `tinyllamas/stories15M-q4_0.gguf`, public, downloaded
  without a token
- **Hardware:** CPU only (x86_64, 12 logical cores); WSL2 with Docker
- **Cluster:** kind v0.33.0, Kubernetes v1.37.0 node image (kubelet v1.37.0 in `node.json`),
  containerd 2.3.4; kubectl v1.37.1
- **doriki:** `bin/doriki` from `make build` on commit `baaebe8` with this task's changes not
  yet committed, which land as `fix(cli): capture drops the node's machine, system and boot
  IDs`, `feat(packs): min_log_level gates markers and facts by the engine's log level` and
  `feat(packs): llamacpp/11889 at -lv 4: fixture captured with doriki capture,
  build/context/KV markers, expected.json` (which also holds this fixture). So `report.json`
  is what the committed pack reports, and `node.json` has no host IDs. The capture section's
  "commit baaebe8" is the commit the binary was built on, not all of its source.
- **Captured:** 2026-10-29 12:24 UTC by the lv4-fixture-canary agent. A first capture at 11:56
  was discarded before it was committed: its `node.json` held the node's machine ID, system
  UUID and boot ID (DK-CLI-BUG-20261029-capture-node-host-ids).
- **Edited:** never. `pod.json`, `events.json`, `container.log`, `node.json` and
  `manifest.json` are exactly as `doriki capture` wrote them. `llamacpp.yaml` is the manifest
  that was applied, unchanged.
- **Derived:** `report.json`, like `expected.json`. `doriki capture` wrote it from the files
  above (sha256 as written:
  `53812915a3793554449aaae95686ef0392824b6260a31b77657467f8bbf6d09b`).
  `packs/capture_test.go` checks that `doriki --from-files` on this directory reproduces its
  phases, gaps and totals. A later pack change that moves them needs it re-recorded with
  `-update`, only with the owner's approval marker (METHOD.md Step 11).

## Why the pod is configured this way

As `../11889/`: `--hf-repo` and `--hf-file` so the fetch happens in the profiled container,
`LLAMA_CACHE=/cache` on an `emptyDir`, and a readiness probe on `/health` every 2 s. In addition
`-lv 4`, which prints the build (line 5), the level (line 6, `verbosity = 4`), the tensor load
(lines 115 to 129), the context (line 131) and the KV cache (line 147, `CPU KV buffer size =
1.69 MiB`), none of which the default level prints.

## Commands

The kind cluster `doriki` already existed (created 2026-10-26) and ran the `-lv 4` deployment
of the discarded first capture, with the image cached. Instead of deleting the cluster, the
deployment was scaled to zero and the image removed from the node, so the pull is real.

```sh
make build
export KUBECONFIG="$PWD/bin/kind-doriki.kubeconfig"

kubectl -n doriki scale deployment/llamacpp --replicas=0
kubectl -n doriki wait --for=delete pod -l app=llamacpp --timeout=120s
docker exec doriki-control-plane crictl rmi 3671daf4816b3   # the llama.cpp image
docker exec doriki-control-plane crictl images | grep -c llama   # 0

kubectl apply -f llamacpp.yaml
kubectl -n doriki rollout status deployment/llamacpp --timeout=10m
POD=$(kubectl -n doriki get pods -l app=llamacpp -o jsonpath='{.items[0].metadata.name}')
bin/doriki capture pod/"$POD" -n doriki --kubeconfig "$KUBECONFIG" --out packs/llamacpp/testdata/11889-lv4/
```

`capture` refuses a directory that already holds files, so `llamacpp.yaml` was applied from a
scratch copy and copied in afterwards, byte for byte.

## What is in the files that a reader should know

- `container.log` has 188 lines, as the 26 Oct comparison run in `../11889/SOURCE.md` did.
  The in-line times (`0.11.920.769`) count from the process start; the pack uses only the
  runtime timestamps.
- Default redaction replaced the token ids in llama.cpp's tokenizer lines (`BOS token =
  [redacted] '<s>'`) and the key in `bad special token: '[redacted]'`, because `token` is a
  secret-named key (DK-COLLECT-ENH-20261028-secret-keys-in-log-lines). No marker or fact reads
  those lines.
- `events.json` is an `EventList`. The readiness probe failed 8 times (two `Unhealthy` events,
  `count: 7` and `count: 1`) while the server downloaded and loaded the model; `Ready` at
  12:24:37.
- `node.json` is what `capture` keeps of the node: name, labels, capacity and `nodeInfo`, which
  holds the kubelet version. Its `machineID`, `systemUUID` and `bootID` are empty strings:
  `capture` clears them, and the Kubernetes type always writes the keys.
- Pod, node and IP names are kind defaults on a local cluster (`doriki-control-plane`). No
  tokens: the only `token` in `pod.json` is the projected service-account volume's file name;
  the container's only environment variable is `LLAMA_CACHE`.

## Checksums

The sha256 of each captured file as committed. `packs/fixtures_test.go` recomputes them, so a
file rewritten by an editor or a tool fails the gate. `report.json` is derived and not listed.

- `container.log`: `sha256:59d108cdbcbc580efe3e07b12457bf35fa5d23bf3fe33a3d96b492dbaff1cb3e`
- `events.json`: `sha256:c501a572ea23adf91863c53c7bee88ba6faa719689b118384a707e2e0013f008`
- `llamacpp.yaml`: `sha256:fb4070e45028b005fbc6e1154514aca76b82e045ea67f41c3372de19024c1c4e`
- `manifest.json`: `sha256:a86e4cb3f471a09c2f41992b49c16d0765e0089a4a8c04dd81b2534e08d2d764`
- `node.json`: `sha256:4ff96e82bd58dde8364e007d8e057a86e631a76abc29049b3fa22f3f919b2ca6`
- `pod.json`: `sha256:7dfe1562f5de36881e37dec01ef7ad4cd606defff92c36a7abc0954c709385ac`

# doriki capture

- doriki: dev (commit baaebe8)
- Captured: 2026-10-29T12:24:38Z
- Parser pack: llamacpp/11889
- Redaction: default

## Commands

```sh
doriki capture the pod named on the command line --out <dir> --redact-level default
doriki --from-files <dir>   # replays this report
```

## What was removed

Every level:
- Environment variables outside the allowlist (DESIGN.md section 4.3) are removed from every
  container; an allowlisted variable whose name says secret has its value replaced by
  `[redacted]`.
- The value of a secret-named flag (`--api-key x`, `--hf-token=x`) in args and command.
- The `kubectl.kubernetes.io/last-applied-configuration` annotation and managedFields.
- From node.json, everything but its name, labels, kubelet and system info, and capacity;
  the system info without the machine ID, system UUID and boot ID.
- In every string of every file and every log line: values shaped like a known token (Hugging
  Face, OpenAI-style, GitHub, Slack, AWS access key ids, JWTs).

Not removed: a secret with no known token shape that the engine prints in its own log (for
example a password it echoes), and secret values inside a longer word.
