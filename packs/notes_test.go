package packs

import (
	"slices"
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
	"github.com/AmitwalaH/doriki/internal/phase"
)

// A pack's version_note is added to the warning when its log_regex reads no version, so the
// report says why (M2 slice 5, llama.cpp prints its build only at -lv 4).
func TestVersionNote(t *testing.T) {
	p := model.Pack{Name: "x/1", Engine: "x", Versions: "1",
		Detect: model.PackDetect{ImageRegex: "x-image", LogRegex: `build (?P<version>\d+)`, VersionNote: "x prints its build only at -lv 4"}}
	_, warnings, err := selectFrom([]model.Pack{p}, "x-image", "no version here")
	if err != nil {
		t.Fatal(err)
	}
	if len(warnings) != 1 || warnings[0].Code != "pack-version-unknown" || !strings.HasSuffix(warnings[0].Text, "; x prints its build only at -lv 4") {
		t.Errorf("warnings = %v, want pack-version-unknown ending with the note", warnings)
	}
}

// Every embedded pack's not_applicable names a phase, and one its markers cannot produce:
// listing a phase the pack can see would hide it when the log does not show it (decision 0008).
func TestNotApplicableConsistent(t *testing.T) {
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	for _, p := range all {
		var ids []string
		for _, m := range p.Markers {
			ids = append(ids, m.ID)
			if d := m.DerivedStartID(); d != "" {
				ids = append(ids, d)
			}
		}
		for _, name := range p.NotApplicable {
			if !slices.Contains(model.Phases(), name) {
				t.Errorf("pack %s: not_applicable %q is not a phase", p.Name, name)
			}
			if slices.Contains(phase.Expected(ids), name) {
				t.Errorf("pack %s: not_applicable %s, but its markers can produce it", p.Name, name)
			}
		}
	}
}
