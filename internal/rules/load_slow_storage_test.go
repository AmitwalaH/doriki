package rules_test

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

const loadSlowStorage = "load-slow-storage"

// No fixture knows its model's size: the GiB line is GPU-only (vLLM 0.32.0
// v1/worker/gpu_model_runner.py:5150), and the vLLM CPU capture and llama.cpp log none. So it
// fires on no fixture; a GPU fixture is wanted.
func TestLoadSlowStorageFixtures(t *testing.T) {
	for _, dir := range []string{"vllm/testdata/0.32", "llamacpp/testdata/11889"} {
		if a, ok := find(fixture(t, dir), loadSlowStorage); ok {
			t.Errorf("%s: no model size, but %s fired: %s", dir, loadSlowStorage, a.Text)
		}
	}
}

// loading is a minimal report: weight-load of the given seconds out of 100 s, and the loaded
// weight memory a pack fact would set (0 for unknown).
func loading(seconds, gib float64) model.Report {
	r := minimal(100, complete(model.PhaseWeightLoad, seconds))
	if gib > 0 {
		r.Manifest.Model = &model.ManifestModel{Ref: "org/model", LoadedGiB: gib}
	}
	return r
}

// Fires when loaded-weight memory over weight-load time is under 0.5 GB/s (decimal GB, as
// DESIGN.md section 6.2 writes it); the share proxy is weight-load over the total, high from
// 40%. Minimal reports.
func TestLoadSlowStorageFires(t *testing.T) {
	for _, c := range []struct {
		seconds, gib float64
		want         string
	}{
		{30, 10, "medium"}, // 10.74 GB / 30 s = 0.36 GB/s
		{60, 10, "high"},   // 0.18 GB/s, 60% of the boot
		{20, 10, ""},       // 0.54 GB/s: over
		{5, 10, ""},        // 2.1 GB/s
		{30, 0, ""},        // size unknown
	} {
		a, ok := find(loading(c.seconds, c.gib), loadSlowStorage)
		if got := map[bool]string{true: a.Severity}[ok]; got != c.want {
			t.Errorf("%v GiB in %v s: severity %q, want %q", c.gib, c.seconds, got, c.want)
		}
	}
}

// The throughput is GiB converted to GB: 10 GiB in 21.4 s is 0.502 GB/s (no advice) but 0.467
// GiB/s, so a rule comparing GiB/s to the 0.5 GB/s threshold fires wrongly here.
func TestLoadSlowStorageUnits(t *testing.T) {
	if a, ok := find(loading(21.4, 10), loadSlowStorage); ok {
		t.Errorf("10 GiB in 21.4 s is 0.502 GB/s, over 0.5; fired: %s", a.Text)
	}
	if _, ok := find(loading(21.5, 10), loadSlowStorage); !ok {
		t.Errorf("10 GiB in 21.5 s is 0.499 GB/s, under 0.5; did not fire")
	}
}

// The evidence and text say the size is estimated from loaded-weight memory, not file size, and
// carry the numbers used.
func TestLoadSlowStorageEvidence(t *testing.T) {
	a, ok := find(loading(30, 10), loadSlowStorage)
	if !ok {
		t.Fatal("did not fire")
	}
	if a.Evidence["size"] != "estimated from loaded-weight memory" || a.Evidence["loaded_gib"] != 10.0 ||
		!strings.Contains(a.Text, "estimated from loaded-weight memory") {
		t.Errorf("evidence %v, text %q", a.Evidence, a.Text)
	}
	gbps, _ := a.Evidence["gb_per_second"].(float64)
	if gbps < 0.357 || gbps > 0.358 {
		t.Errorf("gb_per_second = %v, want 10 GiB (10.737 GB) / 30 s = 0.358", gbps)
	}
	if !strings.Contains(a.Fix, "NVMe") {
		t.Errorf("fix %q", a.Fix)
	}
}
