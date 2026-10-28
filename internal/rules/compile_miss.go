package rules

import (
	"github.com/AmitwalaH/doriki/internal/model"
)

// compile-miss: torch.compile ran without its cache and took a large part of the boot (DESIGN.md
// section 6.2). Source for the fix: vLLM 0.32.0 keeps the cache under
// VLLM_CACHE_ROOT/torch_compile_cache (compilation/backends.py:1075, envs.py:712), keyed by
// the model and compile configuration, so a cache on a persistent volume is reused only while
// those stay the same.
const (
	// compileShare is the trigger: compile at 20% or more of the total.
	compileShare = 0.20
)

func compileMiss(r model.Report) (model.Advice, bool) {
	p, share, ok := phaseShare(r, model.PhaseCompile)
	if !ok || share < compileShare || r.Manifest == nil || r.Manifest.Caches == nil || r.Manifest.Caches.TorchCompile != model.CacheMiss {
		return model.Advice{}, false
	}
	ev := evidence(r, p, share)
	ev["torch_compile"] = model.CacheMiss

	// VLLM_CACHE_ROOT is vLLM's; another engine gets the advice without a setting it lacks.
	where, fix := "the engine's torch.compile cache", "the engine's torch.compile cache directory on a PersistentVolumeClaim"
	if r.Manifest.Engine != nil && r.Manifest.Engine.Name == "vllm" {
		where, fix = "VLLM_CACHE_ROOT", "VLLM_CACHE_ROOT on a PersistentVolumeClaim"
	}
	text := took(r, p, share) + " compiling without a torch.compile cache hit; keep " + where +
		" on a persistent volume and keep the GPU type, tensor parallel size, dtype and engine version the same between boots."
	fix += "; the same GPU type, tensor parallel size, dtype and engine version between boots"
	return model.Advice{Rule: "compile-miss", Text: text, Evidence: ev, Fix: fix}, true
}
