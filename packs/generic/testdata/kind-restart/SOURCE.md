# kind-restart: generic pack fixture, one crash and one restart

A real capture of a busybox pod that exits 1 on its first start and runs on its second, on a
local, throwaway `kind` cluster. It exercises attempt boundaries
(`DK-PHASE-BUG-20261024-attempt-boundary-repull`) and decision 0007: the latest attempt starts
at the previous container's end (`lastState.terminated.finishedAt`), owns the kubelet's cached
re-pull, and shows scheduling, which belongs to the pod.

- **Engine:** none (busybox 1.37.0, a shell script)
- **Image:** `busybox:1.37.0`, pulled by digest
  `sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e` (the
  `containerStatuses[0].imageID` is the same digest)
- **Hardware:** CPU only; WSL2 with Docker
- **Cluster:** kind v0.33.0, Kubernetes v1.37.0 node image, containerd 2.3.4; kubectl v1.37.1
- **Captured:** 2026-10-26 by the M2 agent
- **Edited:** never. These are the files exactly as kubectl wrote them.

## Why the pod is shaped this way

- **The image was already on the node** (a warm-up pod pulled it first, not captured). So both
  starts write the same `Pulled` message ("already present on machine"), and the kubelet merges
  them into one event with `count: 2`, `firstTimestamp` 11:50:42 and `lastTimestamp` 11:50:45.
  That is the aggregation the ticket found in the vLLM spike. A cold first pull would have
  written a different message ("Successfully pulled") and no merged event.
- **A marker file on an `emptyDir`** survives the container restart, so the second start knows
  it is the second one. Exactly one restart: `restartCount` 1.
- **Readiness probe** `cat /tmp/ready` every 2 s; only the second start creates the file.

## Commands

The cluster got its own kubeconfig so no other kubeconfig or context was touched.

```sh
export KUBECONFIG="$PWD/kind-doriki.kubeconfig"
kind create cluster --name doriki --kubeconfig "$KUBECONFIG" --wait 120s

# Warm-up: put the image on the node. Not captured.
kubectl --context kind-doriki apply -f warm.yaml
kubectl --context kind-doriki wait pod/warm --for=jsonpath='{.status.phase}'=Succeeded --timeout=180s
kubectl --context kind-doriki delete pod warm --wait=true

kubectl --context kind-doriki apply -f restart.yaml
kubectl --context kind-doriki wait pod/restart-once --for=condition=Ready --timeout=180s

kubectl --context kind-doriki get pod restart-once -o json > pod.json
kubectl --context kind-doriki get events --field-selector involvedObject.name=restart-once -o json > events.json
kubectl --context kind-doriki logs restart-once --timestamps > container.log
kubectl --context kind-doriki logs restart-once --previous --timestamps > container-previous.log
```

`warm.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: warm
  namespace: default
spec:
  restartPolicy: Never
  containers:
    - name: warm
      image: busybox:1.37.0@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e
      command: ["true"]
```

`restart.yaml`:

```yaml
apiVersion: v1
kind: Pod
metadata:
  name: restart-once
  namespace: default
spec:
  containers:
    - name: main
      image: busybox:1.37.0@sha256:bdf57e528e45e4433820e045b29b4597825a1c9e38353532d90a01445013f82e
      # First start: leave a marker on the emptyDir and exit 1. Second start: the marker is
      # there, so become Ready and keep running. Exactly one restart.
      command:
        - sh
        - -c
        - |
          if [ -f /state/started-once ]; then
            echo "second start: marker found, serving"
            touch /tmp/ready
            exec sleep 3600
          fi
          echo "first start: no marker, exiting 1"
          touch /state/started-once
          sleep 2
          exit 1
      readinessProbe:
        exec:
          command: ["cat", "/tmp/ready"]
        periodSeconds: 2
      volumeMounts:
        - name: state
          mountPath: /state
  volumes:
    - name: state
      emptyDir: {}
```

## What is in the files that a reader should know

- Event and status times have 1 s resolution, and several things share a second: the first
  container ended at 11:50:45, the cached re-pull was last seen at 11:50:45, the second
  container started at 11:50:45, and the pod turned Ready at 11:50:45.
- `container-previous.log` is the first container's one line; `container.log` the second's.
  Under the generic pack they give each attempt's `container-init` its end; no engine phases.
- `events.json` has `"kind": "List"`, as `kubectl get events -o json` writes it. Besides the
  merged `Pulled`, `Created` and `Started` are also merged (`count: 2`), and there are two
  `Unhealthy` events from the readiness probe.
- Pod, node and IP names are kind defaults on a local cluster (`doriki-control-plane`). No
  tokens: the only `token` in `pod.json` is the projected service-account volume's file name,
  and the container has no environment variables.

## Checksums

The sha256 of each captured file as committed. `packs/fixtures_test.go` recomputes them, so a
file rewritten by an editor or a tool fails the gate.

- `container-previous.log`: `sha256:b5552b9e68c2433cd9c3832835a4bec236c95c4f08ad2ff5e667197b2f565bac`
- `container.log`: `sha256:abeb8860c6f8509ec7fa84531f827a842f3af3a06b715ade32444660c224db26`
- `events.json`: `sha256:4e53041db942358cc5e92f5896b242e032e979da02e386a6c657ce6da9f125df`
- `pod.json`: `sha256:6e6b31fdf53bf639d840c697965a5139d45fec34f2c7916900181ae4291a9363`
