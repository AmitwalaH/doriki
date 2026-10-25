package packs

import (
	"slices"
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Every embedded file parses with no unknown fields, and its engine matches its directory,
// so the name Names derives from the path is the pack's real engine.
func TestAllEmbeddedPacksParse(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatalf("All: %v", err)
	}
	if len(all) == 0 {
		t.Fatal("no packs embedded")
	}
	for _, p := range all {
		if p.PackVersion != 1 {
			t.Errorf("%s: pack_version = %d, want 1", p.Engine, p.PackVersion)
		}
	}
	files, _ := embedded.ReadDir(".")
	for _, d := range files {
		if d.IsDir() && !slices.ContainsFunc(all, func(p model.Pack) bool { return p.Engine == d.Name() }) {
			t.Errorf("directory %s has no pack whose engine is %s", d.Name(), d.Name())
		}
	}
}

// The generic pack is the fallback (DESIGN.md section 5.3): it declares no markers and no
// detection, so it never claims a pod and contributes no engine phases.
func TestGenericPack(t *testing.T) {
	if got := Names(); !slices.Contains(got, Generic) {
		t.Errorf("Names() = %v, want it to include %q", got, Generic)
	}
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(p model.Pack) bool { return p.Engine == Generic })
	if i < 0 {
		t.Fatal("no generic pack")
	}
	g := all[i]
	if len(g.Markers) != 0 || g.Detect.ImageRegex != "" || g.Detect.LogRegex != "" {
		t.Errorf("generic pack declares markers or detection: %+v", g)
	}
}

// With only the generic pack embedded, every image falls back to it, with a warning naming
// the image so the user knows which pack to write.
func TestSelectFallsBackToGeneric(t *testing.T) {
	p, warnings, err := Select("nginx:1.29.3-alpine")
	if err != nil {
		t.Fatal(err)
	}
	if p.Engine != Generic {
		t.Errorf("selected %q, want generic", p.Engine)
	}
	want := model.Warning{Code: "no-pack", Text: `no parser pack matches image "nginx:1.29.3-alpine"; only Kubernetes phases are reported`}
	if len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings = %v, want %v", warnings, want)
	}
}

// Selection over a sentinel pack set: one image match wins with no warning; no match falls
// back; several matches are an error until log-based disambiguation lands (M1b).
func TestSelectFrom(t *testing.T) {
	generic := model.Pack{PackVersion: 1, Engine: Generic}
	a := model.Pack{PackVersion: 1, Engine: "a", Detect: model.PackDetect{ImageRegex: "engine-a"}}
	b := model.Pack{PackVersion: 1, Engine: "b", Detect: model.PackDetect{ImageRegex: "engine-(a|b)"}}

	p, warnings, err := selectFrom([]model.Pack{generic, a}, "ghcr.io/x/engine-a:1")
	if err != nil || p.Engine != "a" || warnings != nil {
		t.Errorf("got %q %v %v, want a with no warning", p.Engine, warnings, err)
	}
	p, _, err = selectFrom([]model.Pack{generic, a}, "nginx")
	if err != nil || p.Engine != Generic {
		t.Errorf("got %q %v, want generic", p.Engine, err)
	}
	if _, _, err = selectFrom([]model.Pack{generic, a, b}, "engine-a"); err == nil {
		t.Errorf("two packs match, want an error")
	}
	if _, _, err = selectFrom([]model.Pack{a}, "nginx"); err == nil {
		t.Errorf("no generic pack, want an error")
	}
}

// Names follows the layout: <engine>/<version>.yaml is "engine/version", and a directory's
// single pack.yaml is just "engine".
func TestNameFromPath(t *testing.T) {
	for in, want := range map[string]string{"generic/pack.yaml": "generic", "vllm/0.28.yaml": "vllm/0.28"} {
		if got := nameFromPath(in); got != want {
			t.Errorf("nameFromPath(%q) = %q, want %q", in, got, want)
		}
	}
}

// A misspelt key is an error, not a silently empty field.
func TestParseRejectsUnknownFields(t *testing.T) {
	if _, err := parse([]byte("pack_version: 1\nengine: x\nmarkers:\n  - id: a\n    regx: b\n")); err == nil {
		t.Errorf("misspelt marker key accepted")
	}
	if p, err := parse([]byte("pack_version: 1\nengine: x\nmarkers:\n  - id: a\n    regex: b\n")); err != nil || p.Markers[0].Regex != "b" {
		t.Errorf("valid pack: %+v, %v", p, err)
	}
}

// DESIGN.md section 5.3: when packs match the image, log_regex on the first 200 log lines
// reads the engine version and the pack whose versions range holds it wins. A version no
// pack covers falls back to generic with a warning naming it; a version line past line 200
// is not read.
func TestSelectByVersion(t *testing.T) {
	generic := model.Pack{PackVersion: 1, Engine: Generic, Name: Generic}
	pack := func(name, versions string) model.Pack {
		return model.Pack{PackVersion: 1, Engine: "a", Name: name, Versions: versions,
			Detect: model.PackDetect{ImageRegex: "engine-a", LogRegex: `engine v(?P<version>[0-9.]+)`}}
	}
	all := []model.Pack{generic, pack("a/1", "1.0.0"), pack("a/2", ">=2.0.0 <3.0.0")}

	for lines, want := range map[string]string{"x\nengine v1.0.0": "a/1", "engine v2.4.1\nx": "a/2"} {
		p, warnings, err := selectFrom(all, "ghcr.io/x/engine-a:1", strings.Split(lines, "\n")...)
		if err != nil || p.Name != want || warnings != nil {
			t.Errorf("%q: got %q %v %v, want %s with no warning", lines, p.Name, warnings, err, want)
		}
	}

	p, warnings, err := selectFrom(all, "engine-a", "engine v9.0.0")
	want := model.Warning{Code: "no-pack", Text: `image "engine-a" matches packs a/1, a/2, but the log says version 9.0.0, which none of them covers (1.0.0; >=2.0.0 <3.0.0); only Kubernetes phases are reported`}
	if err != nil || p.Engine != Generic || len(warnings) != 1 || warnings[0] != want {
		t.Errorf("uncovered version: got %q %v %v\nwant generic and %v", p.Name, warnings, err, want)
	}

	late := append(make([]string, 200), "engine v1.0.0")
	if _, _, err := selectFrom(all, "engine-a", late...); err == nil {
		t.Errorf("version on line 201 was read; want two packs left undecided, an error")
	}

	// One pack matching the image, its version not in the log (a pod still starting): that
	// pack, with a warning that the version was not checked.
	p, warnings, err = selectFrom([]model.Pack{generic, pack("a/1", "1.0.0")}, "engine-a", "booting")
	want = model.Warning{Code: "pack-version-unknown", Text: "the log does not say the engine version in its first 200 lines; using pack a/1, tested with 1.0.0"}
	if err != nil || p.Name != "a/1" || len(warnings) != 1 || warnings[0] != want {
		t.Errorf("version unknown: got %q %v %v, want a/1 and %v", p.Name, warnings, err, want)
	}
}

// All names each pack from its path, as Names does.
func TestAllSetsName(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range all {
		names = append(names, p.Name)
	}
	slices.Sort(names)
	if !slices.Equal(names, Names()) {
		t.Errorf("pack names %v, want %v", names, Names())
	}
}
