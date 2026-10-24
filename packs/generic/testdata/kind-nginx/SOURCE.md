# kind-nginx: generic pack fixture

A real capture of one nginx pod booting on a local, throwaway `kind` cluster. nginx has no
parser pack, so this exercises the `generic` fallback: Kubernetes phases only.

- **Engine:** none (nginx 1.29.3, Alpine image)
- **Image:** `nginx:1.29.3-alpine`, digest
  `sha256:b3c656d55d7ad751196f21b7fd2e8d4da9cb430e32f646adcf92441b72f82b14` (from
  `containerStatuses[0].imageID`)
- **Hardware:** CPU only; WSL2 with Docker Desktop
- **Cluster:** kind v0.33.0, Kubernetes v1.37.0 node image, containerd 2.3.4; kubectl v1.37.1
- **Captured:** 2026-10-24 by the M1a agent
- **Edited:** never. These are the files exactly as kubectl wrote them.

The image was not on the node beforehand, so the capture includes a real pull (the `Pulled`
event says 19.325 s).

## Commands

The cluster got its own kubeconfig so no other kubeconfig or context was touched.

```sh
export KUBECONFIG="$PWD/kind-doriki.kubeconfig"
kind create cluster --name doriki --kubeconfig "$KUBECONFIG" --wait 120s

kubectl --context kind-doriki apply -f nginx.yaml        # manifest below
kubectl --context kind-doriki rollout status deployment/nginx --timeout=300s

POD=$(kubectl --context kind-doriki get pods -l app=nginx -o jsonpath='{.items[0].metadata.name}')
kubectl --context kind-doriki get pod "$POD" -o json > pod.json
kubectl --context kind-doriki get events --field-selector involvedObject.name="$POD" -o json > events.json
kubectl --context kind-doriki logs "$POD" --timestamps > container.log

kind delete cluster --name doriki --kubeconfig "$KUBECONFIG"
```

`nginx.yaml`:

```yaml
apiVersion: apps/v1
kind: Deployment
metadata:
  name: nginx
  namespace: default
spec:
  replicas: 1
  selector:
    matchLabels:
      app: nginx
  template:
    metadata:
      labels:
        app: nginx
    spec:
      containers:
        - name: nginx
          image: nginx:1.29.3-alpine
          ports:
            - containerPort: 80
          readinessProbe:
            httpGet:
              path: /
              port: 80
            periodSeconds: 2
```

## What is in the files that a reader should know

- `events.json` has `"kind": "List"`, not `EventList`: that is what `kubectl get events -o json`
  writes.
- Pod, node and IP names are kind defaults on a local cluster (`doriki-control-plane`,
  `10.244.0.0/16`, `172.19.0.0/16`). No tokens: the only `token` in `pod.json` is the projected
  service-account volume's file name.
