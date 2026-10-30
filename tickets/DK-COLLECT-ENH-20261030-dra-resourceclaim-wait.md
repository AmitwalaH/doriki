# DK-COLLECT-ENH-20261030-dra-resourceclaim-wait: A pod waiting on a DRA ResourceClaim shows a long scheduling bar and no reason

**Date Discovered:** 2026-10-30
**Severity:** MEDIUM
**Status:** OPEN
**Label:** [ticket]
**Found by:** first outside feedback (a user report, not reproduced here: no cluster with DRA
and no fixture); API facts read in the Kubernetes docs and in `k8s.io/api v0.37.1`
**Filed by:** the capture-polish agent, on the owner's instruction
**Target:** v0.2

## Problem
A user runs `doriki` on a pod that asks for a device through Dynamic Resource Allocation (DRA):
the pod has `spec.resourceClaims`, and its ResourceClaim is not yet allocated. The pod sits
Pending. doriki draws a long `scheduling` bar and says nothing about why. The pod's events say
`FailedScheduling`, but, as the user reported it, not which claim or which device class it is
waiting for. doriki reads only the pod, its events, the node and the log, so it cannot say
more.

Cause: no collector reads ResourceClaim objects (`internal/collect/k8s`), and `scheduling`
(DESIGN.md section 4.2) has no detail or sub-span.

## What the API says (read 2026-10-30)
- **DRA is stable from Kubernetes 1.35.** The concept page,
  https://kubernetes.io/docs/concepts/scheduling-eviction/dynamic-resource-allocation/, says:
  "Feature state: Stable since Kubernetes v1.35", and "It was first available in the v1.30
  release." The page does not say which release first served `resource.k8s.io/v1`; this ticket
  does not claim one.
- **The claim object** is `resource.k8s.io/v1` `ResourceClaim`, in Go
  `k8s.io/api/resource/v1.ResourceClaim` (`k8s.io/api v0.37.1`, as pinned in `go.mod`), read
  with client-go's `ResourceV1().ResourceClaims(namespace)`.
- **Which claims a pod uses:** `corev1.PodSpec.ResourceClaims []PodResourceClaim`. Each entry
  names the claim directly in `PodResourceClaim.ResourceClaimName`, or a template in
  `ResourceClaimTemplateName`; for a template, the generated claim's name is in
  `corev1.PodStatus.ResourceClaimStatuses[].ResourceClaimName` (unset means no claim was
  needed).
- **Device class:** `spec.devices.requests[].exactly.deviceClassName`
  (`ExactDeviceRequest.DeviceClassName`), or for a request with alternatives
  `spec.devices.requests[].firstAvailable[].deviceClassName` (`DeviceSubRequest.DeviceClassName`).
- **Allocated or not:** `status.allocation` (`ResourceClaimStatus.Allocation
  *AllocationResult`). The API reference,
  https://kubernetes.io/docs/reference/kubernetes-api/workload-resources/resource-claim-v1/,
  says "Allocation is set once the claim has been allocated successfully."
- **When it was allocated:** `status.allocation.allocationTimestamp`
  (`AllocationResult.AllocationTimestamp *metav1.Time`). It is not dependable. The API reference
  says: "This field is not guaranteed to be set, in which case that time is unknown. This is a
  beta field and requires enabling the DRADeviceBindingConditions and
  DRAResourceClaimDeviceStatus feature gate."

## Proposal
1. When the pod has `spec.resourceClaims`, the collector reads each named ResourceClaim
   (`get` on `resourceclaims` in the pod's namespace), resolving template claims through
   `status.resourceClaimStatuses`.
2. While a claim has no `status.allocation`, the `scheduling` phase's detail is
   "waiting for claim X (device class Y)", one per unallocated claim; with several requests,
   every device class is named.
3. The wait is a sub-span of `scheduling`:
   - **With a claim-side timestamp:** from the pod's creation to
     `status.allocation.allocationTimestamp`.
   - **Without one** (the field unset, which is the common case while the feature gates are
     off): from the pod's creation to its scheduled time, labelled as the claim wait without a
     claim-side timestamp, so the report never presents it as measured on the claim.
   - **Still unallocated:** open, as `scheduling` itself is.
4. **Version floor.** doriki supports Kubernetes from 1.29 (`internal/cli/version.go`), and DRA
   is stable only from 1.35. Where the cluster does not serve `resource.k8s.io/v1`, or the
   RBAC does not allow the read, doriki skips the claim detail with a warning and the report is
   otherwise unchanged, as for `nodes` today (DESIGN.md section 7.4). Only `v1` is read; the
   `v1beta1`/`v1beta2` types in the pinned client are not, unless a user asks for them.
5. **RBAC.** `deploy/rbac.yaml` (the ClusterRole `doriki-reader` the README points to) gains
   `get`/`list` on `resourceclaims` in API group `resource.k8s.io`, optional like `nodes`.
6. **Capture.** `doriki capture` writes `resourceclaims.json` (a list of the pod's claims),
   redacted like the other objects; strict hashes claim names as it hashes the pod's. The file
   loader reads it when present, so `--from-files` replays the detail.
7. **Report.** A new attribute and sub-span only; no field renamed or removed (report JSON is
   append-only).

## Fixture needed
None exists, and logs and objects are never fabricated. This needs a real stuck pod: its
`pod.json`, `events.json` and `resourceclaims.json` (`kubectl get resourceclaim <name> -o
json` for each claim), ideally one capture while Pending and one after it scheduled, plus the
cluster version and whether `DRADeviceBindingConditions` is on. The exact `FailedScheduling`
message should be read from that capture, not assumed.

## Why it matters
A GPU pod that waits for a device is exactly the long, silent scheduling bar doriki exists to
explain. Today the user sees time and no reason, and has to know to look at ResourceClaims.

## Related
DESIGN.md sections 4.2, 7.4, 7.1 (`capture`); `deploy/rbac.yaml`; decision 0007 (pod-level
phases: `scheduling` is shown on every attempt, so the claim detail would be too).
