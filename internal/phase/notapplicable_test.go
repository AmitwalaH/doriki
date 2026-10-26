package phase

import (
	"slices"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Decision 0008: a phase the pack says this engine never has is skipped, "not applicable",
// instead of absent; nothing else changes. It is not expected (the pack declares no markers
// for it), raises no warning, and a phase the log does show is reported as observed even if
// listed.
func TestNotApplicable(t *testing.T) {
	events := []model.Event{
		status(model.KindPodCreated, 0), k8s(model.KindScheduled, 1), cachedPull(2),
		started(3, 0), logLine(4),
		marker("x.weights.load.start", 5), marker("x.weights.load.end", 6),
		status(model.KindPodReady, 7),
	}
	markers := []string{"x.weights.load.start", "x.weights.load.end"}
	na := []model.PhaseName{model.PhaseCompile, model.PhaseGraphCapture, model.PhaseWeightLoad}
	r := mustAssign(t, events, Options{Markers: markers, NotApplicable: na})

	for _, name := range []model.PhaseName{model.PhaseCompile, model.PhaseGraphCapture} {
		assertPhase(t, phaseByName(r, name), model.Phase{Name: name, State: model.StateSkipped,
			Attrs: map[string]string{model.AttrNotApplicable: "true"}})
	}
	if p := phaseByName(r, model.PhaseWeightLoad); p.State != model.StateComplete {
		t.Errorf("weight-load = %+v, want complete: an observed phase stays observed", p)
	}
	if p := phaseByName(r, model.PhaseKVAlloc); p.State != model.StateAbsent {
		t.Errorf("kv-alloc = %+v, want absent: not listed", p)
	}
	if len(r.Warnings) != 0 {
		t.Errorf("warnings = %v, want none", r.Warnings)
	}
	without := mustAssign(t, events, Options{Markers: markers})
	if !equalPhasesExcept(without.Phases, r.Phases, model.PhaseCompile, model.PhaseGraphCapture) {
		t.Errorf("not_applicable changed phases other than the listed absent ones")
	}
}

func equalPhasesExcept(a, b []model.Phase, skip ...model.PhaseName) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if slices.Contains(skip, a[i].Name) {
			continue
		}
		if a[i].State != b[i].State || !a[i].Start.Equal(b[i].Start) || !a[i].End.Equal(b[i].End) {
			return false
		}
	}
	return true
}
