# 0005: Once the pod is Ready, an engine phase not seen does not hold exit 1

**Date:** 2026-10-25 · **Decided by:** the owner · **Status:** decided, implemented 2026-10-25 (M1b)

Decision 0003 named its own revisit trigger: a pack that declares a marker its engine does not
always log. The vLLM pack does, on purpose. Graph capture never happens on CPU, and eager mode
turns off torch.compile, so a CPU vLLM pod that booted fine would exit 1. From now on, once the
pod's Ready condition is observed, an expected phase bounded by engine markers that is still
`absent` does not count toward exit 1: the engine is serving, so the phase was skipped or its
log line was not recognised, not pending. It is reported with a `phase-unobserved` warning
naming the phase and the pack, so the gap stays visible. Before Ready the rule of 0003 holds
unchanged, so a pod still booting exits 1. An `open` phase always counts, and so does an
absent Kubernetes phase (scheduling, image-pull, container-init), which gets no such warning.

**Addition, the same day:** a phase can also be `skipped`, a new state. A pack declares a
marker whose id ends in `.skipped` for the phase (for vLLM, `vllm.compile.skipped` on "Enforce
eager set, disabling torch.compile"); when the phase's start is not observed but that marker
is, the phase is `skipped`, names the marker in its `skipped_by` attribute, gets no
`phase-unobserved` warning, and never counts toward exit 1. An observed start wins over the
marker.

The rule lives in `internal/phase/assign.go` (`unobserved`, and the skip check in
`assignPhase`); `cli.exitCode` only honours the warning. Revisit if a real capture shows an
engine phase that runs after Ready (a first-token marker would), because this rule would then
excuse a phase that is merely late.
