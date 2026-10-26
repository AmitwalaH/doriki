# llama.cpp build 11889: CPU fixture from kind

A real capture of the llama.cpp server serving a tiny GGUF on CPU in a local, throwaway `kind`
cluster, at the server's default log level. The image was not on the node, so the capture
includes a real pull (`Pulling` 13:18:21, `Pulled` 13:21:09), and the model was downloaded
from huggingface.co inside the pod by the server itself (`--hf-repo`).

- **Engine:** llama.cpp server, `version: 0.7.0-dev (build 11889, commit d854d98ba)`, as
  `llama-server --version` printed it in the same image (the server's log does not say it at
  the default level)
- **Image:** `ghcr.io/ggml-org/llama.cpp:server`, pulled by the index digest
  `sha256:5741ab30aae42af23ca57d9b98a6d4544cb09c21f372f92c5ee55312fa8b8b0c` (the
  `containerStatuses[0].imageID` is the same digest); linux/amd64
- **Model:** `ggml-org/models`, file `tinyllamas/stories15M-q4_0.gguf` (19,077,344 bytes),
  public, downloaded without a token
- **Hardware:** CPU only (x86_64, AVX2, 12 logical cores); WSL2 with Docker
- **Cluster:** kind v0.33.0, Kubernetes v1.37.0 node image, containerd 2.3.4; kubectl v1.37.1
- **Captured:** 2026-10-26 by the M2 agent
- **Edited:** never. These are the files exactly as kubectl wrote them. `llamacpp.yaml` is the
  manifest that was applied; `make e2e` applies the same file.

## Why the pod is configured this way

- **`--hf-repo` and `--hf-file`**: the server fetches the model itself, so the fetch happens in
  the profiled container. It prints nothing while it downloads (see below).
- **`LLAMA_CACHE=/cache` on an `emptyDir`**: a writable cache that survives a container
  restart, which is how the download was measured.
- **Readiness probe on `/health`**, every 2 s: it answers 503 until the model is loaded, so
  `Ready` marks the moment the server serves.

## Measurements behind the pack (same day, same image)

- **The silent interval is the download.** Between `common_params_print_info: verbosity = 3`
  (line 4) and `server_setup` (line 5) nothing is printed for 10.86 s in this capture. After a
  container restart of the same pod (`kill 1`), with the model already in the `emptyDir`
  cache, the same interval was 0.00 s. That log is not a fixture; it is recorded here as
  evidence for the pack's weight-fetch markers.
- **Default level against `-lv 4`.** A throwaway pod with `-lv 4` printed 188 lines,
  including `build 11889 (d854d98ba)`, `load_tensors: loading model tensors`, the model buffer
  sizes and `llama_kv_cache: CPU KV buffer size = 1.69 MiB`, none of which the default level
  prints. It still printed nothing during the download. Not a fixture either; see
  `DK-PACK-ENH-20261026-llamacpp-verbose`.

## Commands

The cluster got its own kubeconfig so no other kubeconfig or context was touched.

```sh
export KUBECONFIG="$PWD/kind-doriki.kubeconfig"
kind create cluster --name doriki --kubeconfig "$KUBECONFIG" --wait 120s

kubectl --context kind-doriki apply -f llamacpp.yaml
kubectl --context kind-doriki -n doriki rollout status deployment/llamacpp --timeout=600s

POD=$(kubectl --context kind-doriki -n doriki get pods -l app=llamacpp -o jsonpath='{.items[0].metadata.name}')
kubectl --context kind-doriki -n doriki get pod "$POD" -o json > pod.json
kubectl --context kind-doriki -n doriki get events --field-selector involvedObject.name="$POD" -o json > events.json
kubectl --context kind-doriki -n doriki logs "$POD" --timestamps > container.log
```

## What is in the files that a reader should know

- `container.log` has 15 lines. The first two (`warn: LLAMA_ARG_HOST ...`) come from the
  image's own environment, overridden by the manifest's `--host` and `--port`.
- The in-line times (`0.12.437.395`) are minutes, seconds, milliseconds and microseconds since
  the process started, not a wall clock; the pack uses only the runtime timestamps.
- `events.json` has `"kind": "List"`. The readiness probe failed 7 times (one merged
  `Unhealthy` event, `count: 7`) while the server downloaded and loaded the model.
- Pod, node and IP names are kind defaults on a local cluster (`doriki-control-plane`). No
  tokens: the only `token` in `pod.json` is the projected service-account volume's file name;
  the container's only environment variable is `LLAMA_CACHE`.

## Checksums

The sha256 of each captured file as committed. `packs/fixtures_test.go` recomputes them, so a
file rewritten by an editor or a tool fails the gate.

- `container.log`: `sha256:f6ce64f57b7721b8c69ec3cc99d3dc17ad8cf0c3da19ce5dc56bb9e6bc9ccaa3`
- `events.json`: `sha256:63a37da673540bdc3f722f967445e146777057b10cbb4ba7e5afa4e620872487`
- `llamacpp.yaml`: `sha256:1ea37935ec15d4e79bf4ecb41a8eb285689768bf53303312dff7b29b1d2bdcbd`
- `pod.json`: `sha256:27747ce07f938f8f59dd39b58866158a499d2c1c4b4e421005affb43724c3c5b`
