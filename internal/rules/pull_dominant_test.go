package rules_test

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

const pullDominant = "pull-dominant"

// It fires, high, on every real fixture whose image was pulled and took 35% or more of the
// boot (vLLM 268 of 456 s, llama.cpp 168 of 184 s, kind-nginx 20 of 21 s), and not on the two
// fixtures whose image was already on the node. No fixture has a readable kubelet version, so
// each says so and suggests no image volume.
func TestPullDominantFixtures(t *testing.T) {
	for dir, share := range map[string]float64{
		"vllm/testdata/0.32":          268.0 / 456,
		"llamacpp/testdata/11889":     168.0 / 184,
		"generic/testdata/kind-nginx": 20.0 / 21,
	} {
		a, ok := find(fixture(t, dir), pullDominant)
		if !ok {
			t.Errorf("%s: no %s advice", dir, pullDominant)
			continue
		}
		if a.Severity != "high" || a.Evidence["share"] != share {
			t.Errorf("%s: severity %s, share %v; want high, %v", dir, a.Severity, a.Evidence["share"], share)
		}
		if !strings.Contains(a.Text, "kubelet version could not be read") || strings.Contains(a.Fix, "image volume") {
			t.Errorf("%s: unknown kubelet, but text %q, fix %q", dir, a.Text, a.Fix)
		}
		if !strings.Contains(a.Fix, "DaemonSet") || !strings.Contains(a.Fix, "SOCI") {
			t.Errorf("%s: fix %q names no prepull DaemonSet or lazy pull", dir, a.Fix)
		}
	}
	for _, dir := range []string{"generic/testdata/kind-nginx-warm", "generic/testdata/kind-restart"} {
		if a, ok := find(fixture(t, dir), pullDominant); ok {
			t.Errorf("%s: image cached on the node, but %s fired: %s", dir, pullDominant, a.Text)
		}
	}
}

// Image volumes are suggested only from kubelet 1.35, when they are on by default; at 1.34
// they are beta and off, so the advice says why it does not suggest them. The vLLM fixture's
// report with only node.kubelet changed (no fixture has a kubelet version).
func TestPullDominantKubeletVersion(t *testing.T) {
	for kubelet, volumes := range map[string]bool{
		"v1.34.1":           false,
		"v1.31.0":           false,
		"v1.35.0":           true,
		"v1.36.2-eks-1a2b3": true,
		"v2.0.0":            true,
	} {
		r := fixture(t, "vllm/testdata/0.32")
		r.Manifest.Node.Kubelet = kubelet
		a, ok := find(r, pullDominant)
		if !ok {
			t.Fatalf("%s: no advice", kubelet)
		}
		if got := strings.Contains(a.Fix, "image volume"); got != volumes {
			t.Errorf("%s: image volume in fix = %v, want %v: %q", kubelet, got, volumes, a.Fix)
		}
		if !volumes && !strings.Contains(a.Text, "off by default before Kubernetes 1.35") {
			t.Errorf("%s: text does not say why image volumes are not suggested: %q", kubelet, a.Text)
		}
		if a.Evidence["kubelet"] != kubelet {
			t.Errorf("%s: evidence kubelet = %v", kubelet, a.Evidence["kubelet"])
		}
	}
}

// The trigger is image-pull >= 35% of the total, and a share under 40% is medium. Minimal
// reports: no fixture sits near the threshold.
func TestPullDominantThreshold(t *testing.T) {
	for seconds, want := range map[float64]string{34: "", 35: "medium", 39: "medium", 40: "high"} {
		a, ok := find(minimal(100, complete(model.PhaseImagePull, seconds)), pullDominant)
		if got := map[bool]string{true: a.Severity}[ok]; got != want {
			t.Errorf("image-pull %v%%: severity %q, want %q", seconds, got, want)
		}
	}
}

// Never on an image-pull that is not complete, and never without a total to share against.
func TestPullDominantNotComplete(t *testing.T) {
	for _, state := range []model.PhaseState{model.StateOpen, model.StateAbsent, model.StateInvalid} {
		p := complete(model.PhaseImagePull, 90)
		p.State = state
		if _, ok := find(minimal(100, p), pullDominant); ok {
			t.Errorf("fired on a %s image-pull", state)
		}
	}
	r := minimal(100, complete(model.PhaseImagePull, 90))
	r.Totals.PendingToReadySeconds = nil
	if _, ok := find(r, pullDominant); ok {
		t.Errorf("fired with no total")
	}
}
