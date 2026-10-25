package manifest

import (
	"reflect"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

var t0 = time.Date(2026, 10, 25, 9, 23, 40, 0, time.UTC)

func image(attrs map[string]string) model.Event {
	return model.Event{TS: t0, Source: model.SourceManifest, Kind: model.KindContainerImage, Attrs: attrs, Confidence: 1}
}

func fact(key, value string) model.Event {
	return model.Event{TS: t0, Source: model.SourceManifest, Kind: model.KindFact, Confidence: 1,
		Attrs: map[string]string{model.AttrFactKey: key, model.AttrFactValue: value}}
}

// The manifest takes the pod and node from the collector's image event, the engine and parser
// pack from the selected pack, and the version and model from the pack's facts (DESIGN.md
// section 4.3). Facts with keys it does not know are ignored; the pack test rejects them.
func TestBuild(t *testing.T) {
	events := []model.Event{
		image(map[string]string{model.AttrImage: "img", model.AttrNamespace: "default", model.AttrPod: "vllm-1", model.AttrNode: "n1"}),
		fact("engine.version", "0.32.0"),
		fact("model.ref", "facebook/opt-125m"),
		fact("unknown.key", "x"),
		{Source: model.SourceEngineLog, Kind: model.KindLogLine, Raw: "ignored"},
	}
	got := Build(events, model.Pack{Engine: "vllm", Name: "vllm/0.32"})
	want := &model.Manifest{
		ManifestVersion: model.ManifestVersion,
		Pod:             &model.ManifestPod{Namespace: "default", Name: "vllm-1"},
		Node:            &model.ManifestNode{Name: "n1"},
		Engine:          &model.ManifestEngine{Name: "vllm", Version: "0.32.0", ParserPack: "vllm/0.32"},
		Model:           &model.ManifestModel{Ref: "facebook/opt-125m"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build = %+v\nwant %+v", got, want)
	}
}

// Under the generic pack the engine is unknown: only the parser pack is named, and no model.
// A pod object without a node leaves the node out rather than writing an empty one.
func TestBuildGeneric(t *testing.T) {
	got := Build([]model.Event{image(map[string]string{model.AttrImage: "nginx", model.AttrNamespace: "default", model.AttrPod: "p"})},
		model.Pack{Engine: "generic", Name: "generic"})
	want := &model.Manifest{
		ManifestVersion: model.ManifestVersion,
		Pod:             &model.ManifestPod{Namespace: "default", Name: "p"},
		Engine:          &model.ManifestEngine{ParserPack: "generic"},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("Build = %+v\nwant %+v", got, want)
	}
}

// Every fact key a pack may use is one Build knows.
func TestFactKeys(t *testing.T) {
	for _, k := range FactKeys() {
		got := Build([]model.Event{fact(k, "v")}, model.Pack{Engine: "e", Name: "e/1"})
		if reflect.DeepEqual(got, Build(nil, model.Pack{Engine: "e", Name: "e/1"})) {
			t.Errorf("fact %s changed nothing", k)
		}
	}
}
