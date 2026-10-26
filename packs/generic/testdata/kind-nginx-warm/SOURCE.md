# kind-nginx-warm: generic pack fixture, image already on the node

A real capture of the same nginx pod as `kind-nginx`, booting a second time on the same node of
a local, throwaway `kind` cluster. The image was already on the node, so the kubelet wrote no
`Pulling` event, only `Pulled` with "Container image ... already present on machine and can be
accessed by the pod". This exercises decision 0004: `image-pull` is complete at 0 s with
`cached: true`, and the command exits 0.

- **Engine:** none (nginx 1.29.3, Alpine image)
- **Image:** `nginx:1.29.3-alpine`, digest
  `sha256:b3c656d55d7ad751196f21b7fd2e8d4da9cb430e32f646adcf92441b72f82b14` (from
  `containerStatuses[0].imageID`; the same image as `kind-nginx`)
- **Hardware:** CPU only; WSL2 with Docker
- **Cluster:** kind v0.33.0, Kubernetes v1.37.0 node image, containerd 2.3.4; kubectl v1.37.1
- **Captured:** 2026-10-25 by the M1b agent
- **Edited:** never. These are the files exactly as kubectl wrote them.

## Commands

The cluster got its own kubeconfig so no other kubeconfig or context was touched. `nginx.yaml` is
the manifest in `../kind-nginx/SOURCE.md`, unchanged. The first rollout pulls the image and is
not captured; the second, after deleting the Deployment, starts a new pod on the same node.

```sh
export KUBECONFIG="$PWD/kind-doriki.kubeconfig"
kind create cluster --name doriki --kubeconfig "$KUBECONFIG" --wait 120s

# First run: pulls the image onto the node. Not captured.
kubectl --context kind-doriki apply -f nginx.yaml
kubectl --context kind-doriki rollout status deployment/nginx --timeout=300s

# Second run: same image, same node.
kubectl --context kind-doriki delete deployment nginx --wait=true
kubectl --context kind-doriki wait pods -l app=nginx --for=delete --timeout=120s
kubectl --context kind-doriki apply -f nginx.yaml
kubectl --context kind-doriki rollout status deployment/nginx --timeout=300s

POD=$(kubectl --context kind-doriki get pods -l app=nginx -o jsonpath='{.items[0].metadata.name}')
kubectl --context kind-doriki get pod "$POD" -o json > pod.json
kubectl --context kind-doriki get events --field-selector involvedObject.name="$POD" -o json > events.json
kubectl --context kind-doriki logs "$POD" --timestamps > container.log

kind delete cluster --name doriki --kubeconfig "$KUBECONFIG"
```

## What is in the files that a reader should know

- `events.json` has 5 events and no `Pulling`: `Scheduled`, `Pulled` (already present),
  `Created`, `Started`, and one `Unhealthy` from the first readiness probe.
- Event times have 1 s resolution, so scheduling and image-pull are both 0 s.
- Pod, node and IP names are kind defaults on a local cluster (`doriki-control-plane`,
  `10.244.0.0/16`). No tokens: the only `token` in `pod.json` is the projected service-account
  volume's file name, and the container has no environment variables.

## Checksums

The sha256 of each captured file as committed. `packs/fixtures_test.go` recomputes them, so a
file rewritten by an editor or a tool fails the gate.

- `container.log`: `sha256:683f1a2718fced06ea4a3f027032aea4097e5ec1481c47253712ca7b456f24d2`
- `events.json`: `sha256:f44ac3d1227872c3fc4f57418ffd2023ce3bd1c0a713b2005de76d58e0303c90`
- `pod.json`: `sha256:fa999ef7fea8ff55face552e25ae4cee888905cf591b519a79135dd8daf72cde`
