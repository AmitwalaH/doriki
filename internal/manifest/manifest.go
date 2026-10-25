package manifest

import (
	"maps"
	"slices"

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
}

// FactKeys returns the fact keys a pack may declare, sorted.
func FactKeys() []string {
	return slices.Sorted(maps.Keys(facts))
}

func engine(m *model.Manifest) *model.ManifestEngine {
	if m.Engine == nil {
		m.Engine = &model.ManifestEngine{}
	}
	return m.Engine
}

// Build assembles the manifest (DESIGN.md section 4.3) from the collectors' manifest events
// and the selected pack: pod and node from the pod object, the engine and parser pack from
// the pack, and the facts the pack read from the engine log. It reads no clock and does no
// I/O.
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
				m.Pod = &model.ManifestPod{Namespace: ns, Name: name}
			}
			if node := e.Attrs[model.AttrNode]; node != "" {
				m.Node = &model.ManifestNode{Name: node}
			}
		case model.KindFact:
			if set, ok := facts[e.Attrs[model.AttrFactKey]]; ok {
				set(m, e.Attrs[model.AttrFactValue])
			}
		}
	}
	return m
}
