package manifest

import (
	"encoding/json"
	"maps"
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

// ModelSource is the host m's model downloads from: huggingface.co when the pack's model ref
// (each pack's model.ref fact) is a Hub repository id, else "". vLLM's --download-dir does not
// change it: it only says where the Hub download is cached (vLLM 0.32.0 config/load.py:62,
// model_executor/model_loader/default_loader.py:246). Like Fingerprint, it is separate from
// Build, so Build stays the facts as read.
func ModelSource(m *model.Manifest) string {
	if m.Model == nil || !hubRepo.MatchString(m.Model.Ref) {
		return ""
	}
	return hubHost
}
