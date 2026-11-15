package rules_test

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/manifest"
	"github.com/AmitwalaH/doriki/internal/model"
)

const fetchRemoteHub = "fetch-remote-hub"

// No real fixture crosses 25%: vLLM's weight-fetch is 108.2 of 456 s (23.7%), just under,
// with its model from huggingface.co; llama.cpp's is 6%.
func TestFetchRemoteHubFixtures(t *testing.T) {
	r := fixture(t, "vllm/testdata/0.32")
	if r.Manifest.Model == nil || r.Manifest.Model.Source != "huggingface.co" {
		t.Fatalf("vLLM model = %+v, want source huggingface.co", r.Manifest.Model)
	}
	if a, ok := find(r, fetchRemoteHub); ok {
		t.Errorf("vLLM weight-fetch is 23.7%%, under 25%%, but %s fired: %s", fetchRemoteHub, a.Text)
	}
	if a, ok := find(fixture(t, "llamacpp/testdata/11889"), fetchRemoteHub); ok {
		t.Errorf("llama.cpp weight-fetch is 6%%, but %s fired: %s", fetchRemoteHub, a.Text)
	}
}

// hubReport is a minimal report: a weight-fetch of the given share of 100 s, its model from
// the given source.
func hubReport(seconds float64, engine, source string) model.Report {
	r := minimal(100, complete(model.PhaseWeightFetch, seconds))
	r.Manifest.Engine = &model.ManifestEngine{Name: engine}
	r.Manifest.Model = &model.ManifestModel{Ref: "org/model", Source: source}
	return r
}

// Fires at 25% and above when the model comes from huggingface.co; medium under 40%.
// Minimal reports.
func TestFetchRemoteHubFires(t *testing.T) {
	for seconds, want := range map[float64]string{24: "", 25: "medium", 39: "medium", 40: "high", 80: "high"} {
		a, ok := find(hubReport(seconds, "vllm", "huggingface.co"), fetchRemoteHub)
		if got := map[bool]string{true: a.Severity}[ok]; got != want {
			t.Errorf("weight-fetch %v%%: severity %q, want %q", seconds, got, want)
			continue
		}
		if ok && (!strings.Contains(a.Text, "org/model") || !strings.Contains(a.Text, "huggingface.co") || a.Evidence["source"] != "huggingface.co") {
			t.Errorf("weight-fetch %v%%: text %q, evidence %v", seconds, a.Text, a.Evidence)
		}
	}
}

// Not when the source is unknown (a local path) or another host.
func TestFetchRemoteHubOtherSource(t *testing.T) {
	for _, source := range []string{"", "mirror.internal"} {
		if a, ok := find(hubReport(60, "vllm", source), fetchRemoteHub); ok {
			t.Errorf("source %q: fired: %s", source, a.Text)
		}
	}
}

// The revision pin names vLLM's --revision only under vLLM; llama.cpp has no such flag.
func TestFetchRemoteHubRevisionFlag(t *testing.T) {
	a, _ := find(hubReport(60, "vllm", "huggingface.co"), fetchRemoteHub)
	if !strings.Contains(a.Fix, "--revision") {
		t.Errorf("vllm fix %q does not pin --revision", a.Fix)
	}
	a, _ = find(hubReport(60, "llamacpp", "huggingface.co"), fetchRemoteHub)
	if strings.Contains(a.Fix, "--revision") {
		t.Errorf("llama.cpp fix %q names a vLLM flag", a.Fix)
	}
}

// A weight-fetch below confidence 1.0 (llama.cpp's heuristic, vLLM's derived start) is
// called estimated, in the evidence and the text.
func TestFetchRemoteHubEstimated(t *testing.T) {
	r := hubReport(60, "vllm", "huggingface.co")
	c := float32(0.9)
	r.Phases[0].Confidence = &c
	a, _ := find(r, fetchRemoteHub)
	if a.Evidence["estimated"] != true || !strings.Contains(a.Text, "estimated") {
		t.Errorf("confidence 0.9: evidence %v, text %q", a.Evidence, a.Text)
	}
	a, _ = find(hubReport(60, "vllm", "huggingface.co"), fetchRemoteHub)
	if _, ok := a.Evidence["estimated"]; ok || strings.Contains(a.Text, "estimated") {
		t.Errorf("confidence 1.0: evidence %v, text %q", a.Evidence, a.Text)
	}
}

// envReport is a weight-fetch of 60 s of 100 s for the Hub-shaped ref org/model under the given
// engine and env, its model.source set by manifest.ModelSource as the pipeline sets it
// (internal/cli/profile.go), so the env reaches the rule only through model.source.
func envReport(engine string, env map[string]string) model.Report {
	r := hubReport(60, engine, "")
	if engine == "" {
		r.Manifest.Engine = nil
	}
	r.Manifest.Container = &model.ManifestContainer{EnvAllowlist: env}
	r.Manifest.Model.Source = manifest.ModelSource(r.Manifest)
	return r
}

// A pod already pulling through a mirror is not told to use one; the same pod without the
// mirror is, in vLLM's words.
func TestFetchRemoteHubMirrorInUse(t *testing.T) {
	if a, ok := find(envReport("vllm", map[string]string{"HF_ENDPOINT": "http://hf-mirror.internal"}), fetchRemoteHub); ok {
		t.Errorf("HF_ENDPOINT set to a mirror, but %s fired: %s", fetchRemoteHub, a.Text)
	}
	a, ok := find(envReport("vllm", nil), fetchRemoteHub)
	if !ok {
		t.Fatalf("no mirror: %s did not fire", fetchRemoteHub)
	}
	if !strings.Contains(a.Fix, "HF_ENDPOINT") || strings.Contains(a.Fix, "MODEL_ENDPOINT") || !strings.Contains(a.Text, "HF_ENDPOINT") {
		t.Errorf("vLLM: fix %q, text %q; want HF_ENDPOINT only", a.Fix, a.Text)
	}
}

// The mirror variable is the one the engine reads: MODEL_ENDPOINT for llama.cpp (it reads
// HF_ENDPOINT only as a fallback), both when the engine is unknown.
func TestFetchRemoteHubMirrorVariable(t *testing.T) {
	a, ok := find(envReport("llamacpp", nil), fetchRemoteHub)
	if !ok {
		t.Fatalf("llama.cpp: %s did not fire", fetchRemoteHub)
	}
	if !strings.Contains(a.Fix, "MODEL_ENDPOINT") || strings.Contains(a.Fix, "HF_ENDPOINT") || !strings.Contains(a.Text, "MODEL_ENDPOINT") {
		t.Errorf("llama.cpp: fix %q, text %q; want MODEL_ENDPOINT only", a.Fix, a.Text)
	}
	a, ok = find(envReport("", nil), fetchRemoteHub)
	if !ok {
		t.Fatalf("unknown engine: %s did not fire", fetchRemoteHub)
	}
	if want := "HF_ENDPOINT; MODEL_ENDPOINT under llama.cpp"; !strings.Contains(a.Fix, want) || !strings.Contains(a.Text, want) {
		t.Errorf("unknown engine: fix %q, text %q; want %q", a.Fix, a.Text, want)
	}
}
