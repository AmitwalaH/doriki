# vllm 0.32: CPU fixture from kind

A real capture of vLLM 0.32.0 serving `facebook/opt-125m` on CPU in a local, throwaway `kind`
cluster. The image was not on the node, so the capture includes a real pull (the `Pulled` event
says 4m27.995s), and the model was downloaded from huggingface.co inside the pod (the log says
108.186023 seconds).

- **Engine:** vLLM 0.32.0, community CPU build, `noavx512` variant (image labels
  `vllm.version: 0.32.0`, `vllm.variant: noavx512`)
- **Image:** `ghcr.io/mekayelanik/vllm-cpu`, pulled by digest
  `sha256:8ef8561070bd1bd06e15420b76cc55834086e8bcf6dc860dd65b793f2ff8bf5b` (from
  `containerStatuses[0].imageID`)
- **Model:** `facebook/opt-125m`, public, downloaded without a token
- **Hardware:** CPU only (x86_64, AVX2, no AVX-512; 12 logical cores, 7 GiB); WSL2 with Docker
- **Cluster:** kind v0.33.0, Kubernetes v1.37.0 node image, containerd 2.3.4; kubectl v1.37.1
- **Captured:** 2026-10-25 by the M1b agent
- **Edited:** never. These are the files exactly as kubectl wrote them.

## Why the pod is configured this way

- **`--enforce-eager`** (passed through the image's `VLLM_EXTRA_ARGS`): without it the engine
  crashes during warm-up. This image is missing a torch header that `torch.compile` needs on
  CPU: `fatal error: torch/csrc/inductor/cpp_prefix.h: No such file or directory`
  (`torch._inductor.exc.InductorError: CppCompileError`). That is a defect in the image build,
  seen with plain `docker run` in the M1b spike. Eager mode skips torch.compile, so the
  `compile` phase cannot be observed in this fixture; the log says so in its "Enforce eager
  set, disabling torch.compile" line.
- **`/dev/shm` as a memory-backed `emptyDir` of 1Gi**: a pod's default `/dev/shm` is 64 MiB, and
  vLLM needs 160 MiB (`Insufficient space in /dev/shm for shared-memory allocation: 160 MiB
  required, 64 MiB free`); without it the container crash-looped in the spike.
- **`VLLM_CPU_KVCACHE_SPACE=1`** (GiB) and a **4Gi memory limit**: the engine used about 3.1 GiB
  in the spike; the image's default KV cache is 25% of RAM.
- **Readiness probe on `/health`**, every 2 s, so `Ready` marks the moment vLLM serves.

CPU vLLM does not capture CUDA graphs, so `graph-capture` cannot be observed either. It does
allocate a KV cache and logs it (`CPU KV cache size: 29,056 tokens`).

## Commands

The cluster got its own kubeconfig so no other kubeconfig or context was touched. The node had
no vLLM image beforehand (`crictl images` showed none).

```sh
export KUBECONFIG="$PWD/kind-doriki.kubeconfig"
kind create cluster --name doriki --kubeconfig "$KUBECONFIG" --wait 120s

kubectl --context kind-doriki apply -f vllm.yaml        # manifest below
kubectl --context kind-doriki rollout status deployment/vllm --timeout=1800s

POD=$(kubectl --context kind-doriki get pods -l app=vllm -o jsonpath='{.items[0].metadata.name}')
kubectl --context kind-doriki get pod "$POD" -o json > pod.json
kubectl --context kind-doriki get events --field-selector involvedObject.name="$POD" -o json > events.json
kubectl --context kind-doriki logs "$POD" --timestamps > container.log

kind delete cluster --name doriki --kubeconfig "$KUBECONFIG"
```

`vllm.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: vllm
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: vllm
  template:
    metadata:
      labels:
        app: vllm
    spec:
      containers:
        - name: vllm
          image: ghcr.io/mekayelanik/vllm-cpu@sha256:8ef8561070bd1bd06e15420b76cc55834086e8bcf6dc860dd65b793f2ff8bf5b
          env:
            - name: VLLM_MODEL
              value: facebook/opt-125m
            - name: VLLM_CPU_KVCACHE_SPACE
              value: "1"
            - name: VLLM_EXTRA_ARGS
              value: --enforce-eager
          ports:
            - containerPort: 8000
          resources:
            limits:
              memory: 4Gi
          volumeMounts:
            - name: dshm
              mountPath: /dev/shm
          readinessProbe:
            httpGet:
              path: /health
              port: 8000
            periodSeconds: 2
            failureThreshold: 1000
      volumes:
        - name: dshm
          emptyDir:
            medium: Memory
            sizeLimit: 1Gi
```

## What is in the files that a reader should know

- `events.json` has 6 events: `Scheduled`, `Pulling`, `Pulled` (a real pull), `Created`,
  `Started`, and one `Unhealthy` with `count: 25` (the readiness probe before the server was
  up). The container did not restart.
- `container.log` has 339 lines, every one with a `--timestamps` prefix. Lines from vLLM's own
  logger also carry an in-line time with 1 s resolution and a process prefix, such as
  `(Worker pid=171) INFO 10-25 09:31:09 [weight_utils.py:532] ...`; lines from the image's
  entrypoint script, progress bars, warnings and uvicorn (`INFO:     Application startup
  complete.`) do not. 37 lines carry terminal colour codes from the entrypoint's banner.
- The entrypoint's banner prints the image project's GitHub URL and the host's core count and
  memory; these are data as captured, not edited.
- No tokens: `HF_TOKEN` appears only in the Hugging Face library's "Please set a HF_TOKEN"
  warning, with no value; the image's own `HF_TOKEN` env is empty and is not in the pod spec;
  the entrypoint prints `API key:` empty because none was set. The only `token` in `pod.json`
  is the projected service-account volume's file name. Pod, node and IP names are kind
  defaults on a local cluster (`doriki-control-plane`, `10.244.0.0/16`).

## Checksums

The sha256 of each captured file as committed. `packs/fixtures_test.go` recomputes them, so a
file rewritten by an editor or a tool fails the gate.

- `container.log`: `sha256:7ff2c4ee16142d45a3bc6018d3f4d2e093ad39dac85df238dd9c891ee6316e49`
- `events.json`: `sha256:a32f30def40267e2175982a5714883e92bb18a96bc09562871de7dddf500931d`
- `pod.json`: `sha256:6ebb430beb9e77441e54c1e08d2d129019116f9ebbfaee57330dea9eb93bb886`
