# 0004: A cached image is a complete 0 s image-pull, not an absent one

**Date:** 2026-10-24 · **Decided by:** the owner · **Status:** decided, implemented 2026-10-25 (M1b)

When a pod starts on a node that already has its image, the kubelet logs no `Pulling` event,
only `Pulled` with "Container image ... already present on machine". Reporting `image-pull` as
`absent` would read as "not observed" and, under decision 0003, would make a healthy pod on a
warm node exit 1. Instead, `image-pull` is reported as `complete` with 0 s, starting and ending
at that `Pulled` event, with the attribute `cached: true`, so the number still traces to the one
event the kubelet wrote and the M3 `pull-dominant` rule can read "image not previously on
node" from it. It is implemented in M1b together with a real warm-node fixture; until then
`DK-PHASE-ENH-20261024-cached-image-pull` stays open. Revisit if a kubelet version words the
cached case differently, since the detection reads the event message.
