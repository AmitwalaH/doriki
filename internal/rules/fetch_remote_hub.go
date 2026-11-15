package rules

import (
	"github.com/AmitwalaH/doriki/internal/model"
)

// fetch-remote-hub: downloading the weights from the Hugging Face Hub is a large part of the
// boot (DESIGN.md section 6.2). A pod already on a mirror, or offline, has another
// model.source (manifest.ModelSource), so the rule stays silent for it. Sources for the fixes:
// HF_ENDPOINT, read by huggingface_hub at
// github.com/huggingface/huggingface_hub/blob/v1.31.0/src/huggingface_hub/constants.py line 69
// (vLLM 0.32.0 requires huggingface_hub >= 1.31.0; the environment-variables docs page no
// longer lists it); MODEL_ENDPOINT, read by llama.cpp before HF_ENDPOINT at
// github.com/ggml-org/llama.cpp/blob/b11889/common/common.cpp lines 1556-1560; Dragonfly's
// hf:// backend, d7y.io/docs/operations/integrations/hugging-face/; ModelCar,
// kserve.github.io/website/docs/model-serving/storage/providers/oci.
const (
	// fetchShare is the trigger: weight-fetch at 25% or more of the total.
	fetchShare = 0.25
	// hubSource is the manifest's model.source for the Hub (manifest.ModelSource).
	hubSource = "huggingface.co"
)

func fetchRemoteHub(r model.Report) (model.Advice, bool) {
	p, share, ok := phaseShare(r, model.PhaseWeightFetch)
	if !ok || share < fetchShare || r.Manifest == nil || r.Manifest.Model == nil || r.Manifest.Model.Source != hubSource {
		return model.Advice{}, false
	}
	m := r.Manifest.Model
	ev := evidence(r, p, share)
	ev["source"], ev["model"] = m.Source, m.Ref

	// The mirror variable is the one the engine reads: HF_ENDPOINT is huggingface_hub's, which
	// vLLM downloads with; llama.cpp reads MODEL_ENDPOINT first. Under an unknown engine both
	// are named. --revision is vLLM's flag only; llama.cpp build 11889 has no such flag.
	engine := ""
	if r.Manifest.Engine != nil {
		engine = r.Manifest.Engine.Name
	}
	mirror, pin, pinFix := "a cluster-local Hub mirror (HF_ENDPOINT; MODEL_ENDPOINT under llama.cpp)", "", ""
	switch engine {
	case "vllm":
		mirror, pin, pinFix = "a cluster-local Hub mirror (HF_ENDPOINT)", ", and pin --revision", "; pin --revision"
	case "llamacpp":
		mirror = "a cluster-local Hub mirror (MODEL_ENDPOINT)"
	}
	text := took(r, p, share) + " downloading " + m.Ref + " from " + m.Source + "; serve it from " + mirror +
		", Dragonfly's hf:// proxy or a ModelCar OCI image" + pin + "."
	fix := mirror + "; a Dragonfly hf:// proxy; the model as an OCI image (ModelCar)" + pinFix
	return model.Advice{Rule: "fetch-remote-hub", Text: text, Evidence: ev, Fix: fix}, true
}
