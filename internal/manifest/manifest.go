package manifest

import (
	"encoding/json"
	"maps"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/AmitwalaH/doriki/internal/model"
)

// genericEngine is the fallback pack's engine name; under it the engine itself is unknown.
const genericEngine = "generic"

// facts maps each pack fact key (model.PackFact.Key) to the manifest field it fills. It is
// the one list of keys a pack may use.
var facts = map[string]func(m *model.Manifest, v string){
	"engine.version": func(m *model.Manifest, v string) { engine(m).Version = v },
	"model.ref": func(m *model.Manifest, v string) {
		if m.Model == nil {
			m.Model = &model.ManifestModel{}
		}
		m.Model.Ref = v
	},
	"model.loaded_gib": func(m *model.Manifest, v string) {
		if gib, err := strconv.ParseFloat(v, 64); err == nil && gib > 0 {
			if m.Model == nil {
				m.Model = &model.ManifestModel{}
			}
			m.Model.LoadedGiB = gib
		}
	},
	// hit or miss. A pack may log both (one compile range from the cache, another compiled);
	// any miss means the boot paid for compilation, so a later hit does not undo it.
	"caches.torch_compile": func(m *model.Manifest, v string) {
		if m.Caches == nil {
			m.Caches = &model.ManifestCaches{}
		}
		if m.Caches.TorchCompile != model.CacheMiss {
			m.Caches.TorchCompile = v
		}
	},
}

// objectFacts maps each fact key the collector reads from the Kubernetes objects (model.Fact*)
// to the manifest field it fills. Env facts are matched by model.FactEnvPrefix in Build.
var objectFacts = map[string]func(m *model.Manifest, v string){
	model.FactPodUID:      func(m *model.Manifest, v string) { pod(m).UID = v },
	model.FactPodOwner:    func(m *model.Manifest, v string) { pod(m).Owner = v },
	model.FactPodOwnerRef: func(m *model.Manifest, v string) { pod(m).OwnerRef = v },
	model.FactImageDigest: func(m *model.Manifest, v string) { container(m).ImageDigest = v },
	model.FactArgs: func(m *model.Manifest, v string) {
		var args []string
		if json.Unmarshal([]byte(v), &args) == nil {
			container(m).Args = args
		}
	},
	model.FactNodeInstanceType:   func(m *model.Manifest, v string) { node(m).InstanceType = v },
	model.FactNodeZone:           func(m *model.Manifest, v string) { node(m).Zone = v },
	model.FactNodeKubelet:        func(m *model.Manifest, v string) { node(m).Kubelet = v },
	model.FactAcceleratorVendor:  func(m *model.Manifest, v string) { accelerator(m).Vendor = v },
	model.FactAcceleratorProduct: func(m *model.Manifest, v string) { accelerator(m).Product = v },
	model.FactAcceleratorCount: func(m *model.Manifest, v string) {
		if n, err := strconv.Atoi(v); err == nil {
			accelerator(m).Count = n
		}
	},
}

// FactKeys returns the fact keys a pack may declare, sorted.
func FactKeys() []string {
	return slices.Sorted(maps.Keys(facts))
}

func pod(m *model.Manifest) *model.ManifestPod {
	if m.Pod == nil {
		m.Pod = &model.ManifestPod{}
	}
	return m.Pod
}

func node(m *model.Manifest) *model.ManifestNode {
	if m.Node == nil {
		m.Node = &model.ManifestNode{}
	}
	return m.Node
}

func container(m *model.Manifest) *model.ManifestContainer {
	if m.Container == nil {
		m.Container = &model.ManifestContainer{}
	}
	return m.Container
}

func accelerator(m *model.Manifest) *model.ManifestAccelerator {
	if m.Accelerator == nil {
		m.Accelerator = &model.ManifestAccelerator{}
	}
	return m.Accelerator
}

func engine(m *model.Manifest) *model.ManifestEngine {
	if m.Engine == nil {
		m.Engine = &model.ManifestEngine{}
	}
	return m.Engine
}

// Build assembles the manifest (DESIGN.md section 4.3) from the collectors' manifest events
// and the selected pack: pod, node, container and accelerator from the Kubernetes objects, the
// engine and parser pack from the pack, and the facts the pack read from the engine log. Env
// facts are taken as they come: run Sanitize on the events first. It reads no clock and does
// no I/O; the fingerprint is Fingerprint's.
func Build(events []model.Event, pack model.Pack) *model.Manifest {
	m := &model.Manifest{ManifestVersion: model.ManifestVersion}
	engine(m).ParserPack = pack.Name
	if pack.Engine != genericEngine {
		engine(m).Name = pack.Engine
	}
	for _, e := range events {
		if e.Source != model.SourceManifest {
			continue
		}
		switch e.Kind {
		case model.KindContainerImage:
			if ns, name := e.Attrs[model.AttrNamespace], e.Attrs[model.AttrPod]; ns != "" || name != "" {
				pod(m).Namespace, pod(m).Name = ns, name
			}
			if n := e.Attrs[model.AttrNode]; n != "" {
				node(m).Name = n
			}
			if img := e.Attrs[model.AttrImage]; img != "" {
				container(m).Image = img
			}
		case model.KindFact:
			key, value := e.Attrs[model.AttrFactKey], e.Attrs[model.AttrFactValue]
			if name, ok := strings.CutPrefix(key, model.FactEnvPrefix); ok {
				c := container(m)
				if c.EnvAllowlist == nil {
					c.EnvAllowlist = map[string]string{}
				}
				c.EnvAllowlist[name] = value
			} else if set, ok := facts[key]; ok {
				set(m, value)
			} else if set, ok := objectFacts[key]; ok {
				set(m, value)
			}
		}
	}
	return m
}

// hubHost is where a Hub repository id downloads from.
const hubHost = "huggingface.co"

// hubRepo is a Hugging Face Hub repository id, org/name, as vLLM's --model and llama.cpp's
// --hf-repo take it. Anything else (an absolute or relative path, a bare name, a URL) is not
// known to come from the Hub.
var hubRepo = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$`)

// modelScopeHost is where vLLM downloads a Hub-shaped ref from under VLLM_USE_MODELSCOPE.
const modelScopeHost = "modelscope.cn"

// llamacppEngine is the llama.cpp pack's engine name. llama.cpp reads its own variables
// (MODEL_ENDPOINT, LLAMA_ARG_OFFLINE) and not huggingface_hub's; every other engine is taken
// to download through huggingface_hub, and an unknown engine is checked for both.
const llamacppEngine = "llamacpp"

// ModelSource is the host m's model downloads from, "" when nothing is known to be
// downloaded. Only a Hub repository id (each pack's model.ref fact) is downloaded at all; a
// relative directory such as models/opt-125m has the same shape and is a known limit (the
// engine checks its filesystem, which doriki cannot see). For a Hub id the captured env
// decides, each variable only under the engine that reads it, in this order:
//   - offline mode downloads nothing: HF_HUB_OFFLINE, or else TRANSFORMERS_OFFLINE, for
//     huggingface_hub; LLAMA_ARG_OFFLINE for llama.cpp.
//   - VLLM_USE_MODELSCOPE downloads from ModelScope instead
//     (vLLM 0.32.0 model_executor/model_loader/weight_utils.py:167).
//   - an endpoint replaces the Hub: MODEL_ENDPOINT, or else HF_ENDPOINT, for llama.cpp
//     (b11889 common/common.cpp:1556-1560); HF_ENDPOINT for huggingface_hub (v1.31.0
//     constants.py:69).
//   - otherwise huggingface.co.
//
// vLLM's --download-dir does not change it: it only says where the Hub download is cached
// (vLLM 0.32.0 config/load.py:62, model_executor/model_loader/default_loader.py:246). Like
// Fingerprint, it is separate from Build, so Build stays the facts as read.
func ModelSource(m *model.Manifest) string {
	if m.Model == nil || !hubRepo.MatchString(m.Model.Ref) {
		return ""
	}
	var env map[string]string
	if m.Container != nil {
		env = m.Container.EnvAllowlist
	}
	name := ""
	if m.Engine != nil {
		name = m.Engine.Name
	}
	hub := name != llamacppEngine                 // reads huggingface_hub's variables
	llama := name == llamacppEngine || name == "" // reads llama.cpp's variables

	offline := env["HF_HUB_OFFLINE"]
	if offline == "" {
		offline = env["TRANSFORMERS_OFFLINE"] // huggingface_hub v1.31.0 constants.py:192
	}
	if (hub && hubTrue(offline)) || (llama && llamaTrue(env["LLAMA_ARG_OFFLINE"])) {
		return ""
	}
	if hub && vllmTrue(env["VLLM_USE_MODELSCOPE"]) {
		return modelScopeHost
	}
	endpoint := ""
	if llama {
		endpoint = env["MODEL_ENDPOINT"]
	}
	if endpoint == "" {
		endpoint = env["HF_ENDPOINT"]
	}
	if endpoint == "" {
		return hubHost
	}
	return endpointHost(endpoint)
}

// endpointHost is the host of a mirror endpoint URL; huggingface.co for the Hub itself or a
// subdomain of it. A value with no readable host is returned as given, so the report shows
// what was set rather than nothing.
func endpointHost(endpoint string) string {
	u, err := url.Parse(endpoint)
	if err != nil || u.Hostname() == "" {
		return endpoint
	}
	host := strings.ToLower(u.Hostname())
	if host == hubHost || strings.HasSuffix(host, "."+hubHost) {
		return hubHost
	}
	return host
}

// hubTrue is huggingface_hub's truthy test: "1", "ON", "YES" or "TRUE" in any case
// (huggingface_hub v1.31.0 constants.py:12 and :19).
func hubTrue(v string) bool {
	switch strings.ToUpper(v) {
	case "1", "ON", "YES", "TRUE":
		return true
	}
	return false
}

// vllmTrue is vLLM's test for VLLM_USE_MODELSCOPE: "1" or "true", trimmed, in any case
// (vLLM 0.32.0 envs.py:734-735). Unlike huggingface_hub's, "yes" and "on" are false.
func vllmTrue(v string) bool {
	v = strings.ToLower(strings.TrimSpace(v))
	return v == "1" || v == "true"
}

// llamaTrue is llama.cpp's test for a flag set from the env, such as LLAMA_ARG_OFFLINE
// (--offline, b11889 common/arg.cpp:3981-3986): "on", "enabled", "true" or "1", case
// sensitive (common/arg.cpp:1345-1346, applied to env values at :800).
func llamaTrue(v string) bool {
	return v == "on" || v == "enabled" || v == "true" || v == "1"
}
