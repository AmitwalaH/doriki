package rules

import (
	"github.com/AmitwalaH/doriki/internal/model"
)

// fetch-remote-hub: downloading the weights from the Hugging Face Hub is a large part of the
// boot (DESIGN.md section 6.2). Sources for the fixes: HF_ENDPOINT, read by huggingface_hub at
// github.com/huggingface/huggingface_hub/blob/v1.31.0/src/huggingface_hub/constants.py line 69
// (vLLM 0.32.0 requires huggingface_hub >= 1.31.0; the environment-variables docs page no
// longer lists it); Dragonfly's hf:// backend,
// d7y.io/docs/operations/integrations/hugging-face/; ModelCar,
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

	// HF_ENDPOINT is huggingface_hub's, which vLLM downloads with, and --revision is vLLM's
	// flag. llama.cpp build 11889 shows neither, so its advice names no setting it lacks.
	mirror, pin, pinFix := "a cluster-local Hub mirror", "", ""
	if r.Manifest.Engine != nil && r.Manifest.Engine.Name == "vllm" {
		mirror, pin, pinFix = "a cluster-local Hub mirror (HF_ENDPOINT)", ", and pin --revision", "; pin --revision"
	}
	text := took(r, p, share) + " downloading " + m.Ref + " from " + m.Source + "; serve it from " + mirror +
		", Dragonfly's hf:// proxy or a ModelCar OCI image" + pin + "."
	fix := mirror + "; a Dragonfly hf:// proxy; the model as an OCI image (ModelCar)" + pinFix
	return model.Advice{Rule: "fetch-remote-hub", Text: text, Evidence: ev, Fix: fix}, true
}
