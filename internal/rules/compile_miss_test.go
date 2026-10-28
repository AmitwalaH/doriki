package rules_test

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

const compileMiss = "compile-miss"

// No fixture can fire it: the only engine with a compile phase, vLLM, runs eager in the CPU
// capture, so compile is skipped and no cache line is logged. A GPU fixture is wanted.
func TestCompileMissFixture(t *testing.T) {
	if a, ok := find(fixture(t, "vllm/testdata/0.32"), compileMiss); ok {
		t.Errorf("vLLM compile is skipped, but %s fired: %s", compileMiss, a.Text)
	}
}

// compiled is a minimal report: a compile phase of the given share of 100 s, the engine, and
// the torch.compile cache state as a pack fact would set it.
func compiled(seconds float64, engine, cache string) model.Report {
	r := minimal(100, complete(model.PhaseCompile, seconds))
	r.Manifest.Engine = &model.ManifestEngine{Name: engine}
	if cache != "" {
		r.Manifest.Caches = &model.ManifestCaches{TorchCompile: cache}
	}
	return r
}

// Fires at 20% and above on a cache miss; medium under 40%. Not on a hit or an unknown state.
// Minimal reports.
func TestCompileMissFires(t *testing.T) {
	for _, c := range []struct {
		seconds     float64
		cache, want string
	}{
		{19, "miss", ""}, {20, "miss", "medium"}, {45, "miss", "high"}, {45, "hit", ""}, {45, "", ""},
	} {
		a, ok := find(compiled(c.seconds, "vllm", c.cache), compileMiss)
		if got := map[bool]string{true: a.Severity}[ok]; got != c.want {
			t.Errorf("compile %v%%, cache %q: severity %q, want %q", c.seconds, c.cache, got, c.want)
			continue
		}
		if ok && a.Evidence["torch_compile"] != "miss" {
			t.Errorf("evidence %v", a.Evidence)
		}
	}
}

// Never on a skipped compile, the engine's own word that it did not compile.
func TestCompileMissSkipped(t *testing.T) {
	r := compiled(45, "vllm", "miss")
	r.Phases[0].State = model.StateSkipped
	if a, ok := find(r, compileMiss); ok {
		t.Errorf("fired on a skipped compile: %s", a.Text)
	}
}

// VLLM_CACHE_ROOT is vLLM's setting; another engine's advice does not name it.
func TestCompileMissEngineSetting(t *testing.T) {
	a, _ := find(compiled(45, "vllm", "miss"), compileMiss)
	if !strings.Contains(a.Fix, "VLLM_CACHE_ROOT") {
		t.Errorf("vllm fix %q does not name VLLM_CACHE_ROOT", a.Fix)
	}
	a, _ = find(compiled(45, "sglang", "miss"), compileMiss)
	if strings.Contains(a.Fix, "VLLM_") || a.Fix == "" {
		t.Errorf("other engine fix %q", a.Fix)
	}
}
