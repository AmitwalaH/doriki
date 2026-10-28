package rules

import (
	"fmt"

	"github.com/AmitwalaH/doriki/internal/model"
)

// load-slow-storage: the weights load slower than local disk would allow (DESIGN.md section
// 6.2). The size is the engine's loaded-weight memory (manifest model.loaded_gib), the only size
// v0.1 reads without a network call; it differs from the size on disk when the dtype or
// quantization changes, so the evidence says it is estimated. Source for the fix: vLLM 0.32.0
// --load-format runai_streamer and tensorizer (config/load.py:48-51).
//
// Its share is a proxy (owner decision, M3 plan Q3): weight-load over the pending-to-ready
// total, which sets the severity; the trigger is the throughput.
const (
	// minGBPerSecond is the trigger: under 0.5 GB/s (decimal, as DESIGN.md writes it).
	minGBPerSecond = 0.5
	// gbPerGiB converts the engine's GiB to the threshold's GB.
	gbPerGiB = 1.073741824
)

func loadSlowStorage(r model.Report) (model.Advice, bool) {
	p, share, ok := phaseShare(r, model.PhaseWeightLoad)
	if !ok || *p.Seconds <= 0 || r.Manifest == nil || r.Manifest.Model == nil || r.Manifest.Model.LoadedGiB <= 0 {
		return model.Advice{}, false
	}
	gib := r.Manifest.Model.LoadedGiB
	gbps := gib * gbPerGiB / *p.Seconds
	if gbps >= minGBPerSecond {
		return model.Advice{}, false
	}
	ev := evidence(r, p, share)
	ev["loaded_gib"], ev["gb_per_second"], ev["size"] = gib, gbps, "estimated from loaded-weight memory"

	// The streaming loaders are vLLM's --load-format values; another engine gets the storage
	// advice without them.
	loader, fix := "", "the weights on local NVMe instead of network storage"
	if r.Manifest.Engine != nil && r.Manifest.Engine.Name == "vllm" {
		loader = ", or stream them with --load-format runai_streamer or tensorizer"
		fix += "; --load-format runai_streamer or tensorizer"
	}
	text := fmt.Sprintf("%s loading %.1f GiB of weights (estimated from loaded-weight memory) at %.2f GB/s, under %.1f GB/s; "+
		"move the weights to local NVMe%s.", took(r, p, share), gib, gbps, minGBPerSecond, loader)
	return model.Advice{Rule: "load-slow-storage", Text: text, Evidence: ev, Fix: fix}, true
}
