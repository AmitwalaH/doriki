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

// The env decides where a Hub-shaped ref really downloads from, each variable counting under
// the engine that reads it and under an unknown engine: offline mode downloads nothing,
// ModelScope replaces the Hub, a mirror endpoint replaces huggingface.co. Minimal manifests.
func TestModelSourceEnv(t *testing.T) {
	for _, c := range []struct {
		name, engine, ref string
		env               map[string]string
		want              string
	}{
		{"no env", "vllm", "org/name", nil, "huggingface.co"},
		{"HF_ENDPOINT mirror", "vllm", "org/name", map[string]string{"HF_ENDPOINT": "http://hf-mirror.internal"}, "hf-mirror.internal"},
		{"HF_ENDPOINT mirror, port and path", "vllm", "org/name", map[string]string{"HF_ENDPOINT": "https://Mirror.Example:8443/hf/"}, "mirror.example"},
		{"HF_ENDPOINT is the Hub", "vllm", "org/name", map[string]string{"HF_ENDPOINT": "https://huggingface.co"}, "huggingface.co"},
		{"HF_ENDPOINT a Hub subdomain", "vllm", "org/name", map[string]string{"HF_ENDPOINT": "https://eu.HuggingFace.co/"}, "huggingface.co"},
		{"HF_ENDPOINT a lookalike host", "vllm", "org/name", map[string]string{"HF_ENDPOINT": "https://nothuggingface.co"}, "nothuggingface.co"},
		{"HF_ENDPOINT empty", "vllm", "org/name", map[string]string{"HF_ENDPOINT": ""}, "huggingface.co"},
		{"unparseable endpoint", "vllm", "org/name", map[string]string{"HF_ENDPOINT": "http://[bad"}, "http://[bad"},
		{"endpoint without a scheme", "vllm", "org/name", map[string]string{"HF_ENDPOINT": "hf-mirror.internal"}, "hf-mirror.internal"},
		{"llama.cpp: MODEL_ENDPOINT wins over HF_ENDPOINT", "llamacpp", "org/name",
			map[string]string{"MODEL_ENDPOINT": "http://model-mirror", "HF_ENDPOINT": "http://hf-mirror"}, "model-mirror"},
		{"llama.cpp: HF_ENDPOINT as the fallback", "llamacpp", "org/name", map[string]string{"HF_ENDPOINT": "http://hf-mirror"}, "hf-mirror"},
		{"unknown engine: MODEL_ENDPOINT wins", "", "org/name",
			map[string]string{"MODEL_ENDPOINT": "http://model-mirror", "HF_ENDPOINT": "http://hf-mirror"}, "model-mirror"},
		{"vLLM does not read MODEL_ENDPOINT", "vllm", "org/name", map[string]string{"MODEL_ENDPOINT": "http://model-mirror"}, "huggingface.co"},
		{"vLLM: HF_HUB_OFFLINE=1", "vllm", "org/name", map[string]string{"HF_HUB_OFFLINE": "1", "HF_ENDPOINT": "http://hf-mirror"}, ""},
		{"vLLM: HF_HUB_OFFLINE=yes", "vllm", "org/name", map[string]string{"HF_HUB_OFFLINE": "yes"}, ""},
		{"vLLM: HF_HUB_OFFLINE=0", "vllm", "org/name", map[string]string{"HF_HUB_OFFLINE": "0"}, "huggingface.co"},
		{"vLLM: TRANSFORMERS_OFFLINE=1", "vllm", "org/name", map[string]string{"TRANSFORMERS_OFFLINE": "1"}, ""},
		// huggingface_hub v1.31.0 constants.py:192 reads, verbatim:
		//   HF_HUB_OFFLINE = _is_true(os.environ.get("HF_HUB_OFFLINE") or os.environ.get("TRANSFORMERS_OFFLINE"))
		// The `or` picks the first non-empty value before the truthy test, so a set
		// HF_HUB_OFFLINE, even "0", decides and TRANSFORMERS_OFFLINE is not read.
		{"vLLM: HF_HUB_OFFLINE=0 hides TRANSFORMERS_OFFLINE", "vllm", "org/name",
			map[string]string{"HF_HUB_OFFLINE": "0", "TRANSFORMERS_OFFLINE": "1"}, "huggingface.co"},
		{"llama.cpp does not read HF_HUB_OFFLINE", "llamacpp", "org/name", map[string]string{"HF_HUB_OFFLINE": "1"}, "huggingface.co"},
		{"llama.cpp: LLAMA_ARG_OFFLINE=1", "llamacpp", "org/name", map[string]string{"LLAMA_ARG_OFFLINE": "1", "MODEL_ENDPOINT": "http://m"}, ""},
		{"llama.cpp: LLAMA_ARG_OFFLINE=enabled", "llamacpp", "org/name", map[string]string{"LLAMA_ARG_OFFLINE": "enabled"}, ""},
		{"llama.cpp: LLAMA_ARG_OFFLINE=TRUE is not truthy", "llamacpp", "org/name", map[string]string{"LLAMA_ARG_OFFLINE": "TRUE"}, "huggingface.co"},
		{"vLLM does not read LLAMA_ARG_OFFLINE", "vllm", "org/name", map[string]string{"LLAMA_ARG_OFFLINE": "1"}, "huggingface.co"},
		{"vLLM: VLLM_USE_MODELSCOPE=True", "vllm", "org/name", map[string]string{"VLLM_USE_MODELSCOPE": "True", "HF_ENDPOINT": "http://hf-mirror"}, "modelscope.cn"},
		{"vLLM: VLLM_USE_MODELSCOPE=yes is not truthy", "vllm", "org/name", map[string]string{"VLLM_USE_MODELSCOPE": "yes"}, "huggingface.co"},
		{"vLLM: offline beats ModelScope", "vllm", "org/name", map[string]string{"VLLM_USE_MODELSCOPE": "1", "HF_HUB_OFFLINE": "1"}, ""},
		{"llama.cpp does not read VLLM_USE_MODELSCOPE", "llamacpp", "org/name", map[string]string{"VLLM_USE_MODELSCOPE": "1"}, "huggingface.co"},
		{"a path ref with HF_ENDPOINT", "vllm", "/models/opt-125m", map[string]string{"HF_ENDPOINT": "http://hf-mirror"}, ""},
	} {
		m := &model.Manifest{Model: &model.ManifestModel{Ref: c.ref}}
		if c.engine != "" {
			m.Engine = &model.ManifestEngine{Name: c.engine}
		}
		if c.env != nil {
			m.Container = &model.ManifestContainer{EnvAllowlist: c.env}
		}
		if got := ModelSource(m); got != c.want {
			t.Errorf("%s: ModelSource = %q, want %q", c.name, got, c.want)
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
