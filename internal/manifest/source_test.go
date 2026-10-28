package manifest

import (
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// A model ref shaped like a Hub repository id (org/name) is downloaded from huggingface.co;
// a local path, a bare name or no ref has no known source.
func TestModelSource(t *testing.T) {
	for ref, want := range map[string]string{
		"facebook/opt-125m":       "huggingface.co", // vLLM fixture
		"ggml-org/models":         "huggingface.co", // llama.cpp fixture, --hf-repo
		"Qwen/Qwen3-35B-A3B":      "huggingface.co",
		"/models/opt-125m":        "",
		"./opt-125m":              "",
		"/models/tiny.gguf":       "",
		"opt-125m":                "",
		"a/b/c":                   "",
		"s3://bucket/model":       "",
		"":                        "",
		"org/name with a space/x": "",
	} {
		m := &model.Manifest{}
		if ref != "" {
			m.Model = &model.ManifestModel{Ref: ref}
		}
		if got := ModelSource(m); got != want {
			t.Errorf("ModelSource(%q) = %q, want %q", ref, got, want)
		}
	}
}

// The collector's kubelet fact fills node.kubelet.
func TestBuildKubelet(t *testing.T) {
	got := Build([]model.Event{fact(model.FactNodeKubelet, "v1.35.0")}, model.Pack{Engine: "e", Name: "e/1"})
	if got.Node == nil || got.Node.Kubelet != "v1.35.0" {
		t.Errorf("node = %+v, want kubelet v1.35.0", got.Node)
	}
}
