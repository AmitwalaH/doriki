package phase

import (
	"reflect"
	"strconv"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

func terminated(s float64, restartCount int) model.Event {
	e := status(model.KindContainerTerminated, s)
	e.Attrs = map[string]string{model.AttrRestartCount: strconv.Itoa(restartCount)}
	return e
}

func previousLine(s float64, restartCount int) model.Event {
	e := logLine(s)
	e.Attrs = map[string]string{model.AttrRestartCount: strconv.Itoa(restartCount)}
	return e
}

// One crash and a restart, shaped like the kind-restart capture: the previous container ends,
// the kubelet re-pulls (cached) in the same second, and the new container starts. Attempt 1
// runs from the previous container's end, so it owns the re-pull
// (DK-PHASE-BUG-20261024-attempt-boundary-repull). A line from the previous container that is
// stamped just past the boundary (finishedAt has 1 s resolution) stays in attempt 0 by its
// tag. Scheduling belongs to the pod and is shown on both attempts (decision 0007).
func restartBoot() []model.Event {
	return []model.Event{
		status(model.KindPodCreated, 0),
		k8s(model.KindScheduled, 0),
		cachedPull(0),
		started(1, 0),
		previousLine(1.1, 0),
		previousLine(3.3, 0), // the crash, logged after finishedAt's whole second
		terminated(3, 0),
		cachedPull(3), // same second as the termination; sorts before it
		started(3, 1),
		logLine(3.8),
		status(model.KindPodReady, 3),
	}
}

func TestAttemptBoundaryAtTermination(t *testing.T) {
	r := mustAssign(t, restartBoot(), Options{AllAttempts: true})
	if r.Attempt != 1 || r.AttemptsTotal != 2 || len(r.Attempts) != 2 {
		t.Fatalf("attempt %d of %d, list %d; want 1 of 2 and both listed", r.Attempt, r.AttemptsTotal, len(r.Attempts))
	}
	scheduling := model.Phase{Name: model.PhaseScheduling, Start: at(0), End: at(0), Seconds: secs(0), State: model.StateComplete,
		Sources: []model.Source{model.SourceK8sEvent, model.SourceContainerStatus}}
	cached := map[string]string{model.AttrCached: "true"}

	assertPhase(t, phaseByName(r, model.PhaseScheduling), scheduling)
	assertPhase(t, phaseByName(r, model.PhaseImagePull), model.Phase{
		Name: model.PhaseImagePull, Start: at(3), End: at(3), Seconds: secs(0), State: model.StateComplete,
		Attrs: cached, Sources: []model.Source{model.SourceK8sEvent}})
	assertPhase(t, phaseByName(r, model.PhaseContainerInit), model.Phase{
		Name: model.PhaseContainerInit, Start: at(3), End: at(3.8), Seconds: secs(0.8), State: model.StateComplete,
		Sources: []model.Source{model.SourceContainerStatus, model.SourceEngineLog}})
	if got := deref(r.Totals.PendingToReadySeconds); got != 3.0 {
		t.Errorf("pending_to_ready = %v, want 3 (from pod creation)", got)
	}
	for _, w := range r.Warnings {
		t.Errorf("unexpected warning %s: %s", w.Code, w.Text)
	}

	first := model.Report{Phases: r.Attempts[0].Phases}
	assertPhase(t, phaseByName(first, model.PhaseScheduling), scheduling)
	assertPhase(t, phaseByName(first, model.PhaseImagePull), model.Phase{
		Name: model.PhaseImagePull, Start: at(0), End: at(0), Seconds: secs(0), State: model.StateComplete,
		Attrs: cached, Sources: []model.Source{model.SourceK8sEvent}})
	assertPhase(t, phaseByName(first, model.PhaseContainerInit), model.Phase{
		Name: model.PhaseContainerInit, Start: at(1), End: at(1.1), Seconds: secs(0.1), State: model.StateComplete,
		Sources: []model.Source{model.SourceContainerStatus, model.SourceEngineLog}})
	if r.Attempts[0].Totals.PendingToReadySeconds != nil {
		t.Errorf("attempt 0 pending_to_ready = %v, want omitted (Ready came in attempt 1)", *r.Attempts[0].Totals.PendingToReadySeconds)
	}
}

// Without a termination time the boundary falls back to the attempt's own start, so a re-pull
// before that start stays in the previous attempt: the limit the termination time removes.
func TestAttemptBoundaryFallback(t *testing.T) {
	var kept []model.Event
	for _, e := range restartBoot() {
		switch {
		case e.Kind == model.KindContainerTerminated:
		case e.Kind == model.KindPulled && e.TS.Equal(at(3)):
			kept = append(kept, cachedPull(2.5))
		default:
			kept = append(kept, e)
		}
	}
	r := mustAssign(t, kept, Options{})
	if p := phaseByName(r, model.PhaseImagePull); p.State != model.StateAbsent {
		t.Errorf("image-pull = %+v, want absent: the re-pull came before the only boundary known", p)
	}
	if p := phaseByName(r, model.PhaseScheduling); p.State != model.StateComplete {
		t.Errorf("scheduling = %+v, want complete on every attempt", p)
	}
}

// The attempt tag is collector bookkeeping, like the restart count on a start: it stays on
// the event and never becomes a phase attribute, even on engine phases, which take every
// other attribute of their markers.
func TestRestartCountNotAPhaseAttr(t *testing.T) {
	events := []model.Event{
		status(model.KindPodCreated, 0),
		started(1, 0),
		markerWith("x.weights.load.start", 2, 1, map[string]string{model.AttrRestartCount: "0", "model": "m"}),
		markerWith("x.weights.load.end", 3, 1, map[string]string{model.AttrRestartCount: "0"}),
		terminated(4, 0),
		started(5, 1),
	}
	two := 0
	r := mustAssign(t, events, Options{Markers: enginePack, Attempt: &two})
	p := phaseByName(r, model.PhaseWeightLoad)
	if p.State != model.StateComplete || p.Attrs["model"] != "m" || p.Attrs[model.AttrRestartCount] != "" {
		t.Errorf("weight-load = %+v, want complete with model and without restart_count", p)
	}
}

// On a later attempt, the time between a pod-level phase and the attempt's boundary was spent
// by the earlier attempts: it is a gap with reason "earlier attempts", not unattributed time.
// What lies past the boundary is an ordinary gap. The first attempt has no boundary, so none
// of its gaps has a reason.
func TestEarlierAttemptsGap(t *testing.T) {
	r := mustAssign(t, restartBoot(), Options{AllAttempts: true})
	want := []model.Gap{{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: 3, Reason: model.GapEarlierAttempts}}
	if !reflect.DeepEqual(r.Gaps, want) {
		t.Errorf("latest gaps = %+v, want %+v", r.Gaps, want)
	}
	for _, g := range r.Attempts[0].Gaps {
		if g.Reason != "" {
			t.Errorf("attempt 0 gap %+v has a reason", g)
		}
	}

	// The re-pull comes 3 s after the boundary: 3 s of earlier attempts, then a 3 s gap.
	var late []model.Event
	for _, e := range restartBoot() {
		switch {
		case e.Kind == model.KindPulled && e.TS.Equal(at(3)):
			late = append(late, cachedPull(6))
		case e.Kind == model.KindContainerStarted && e.TS.Equal(at(3)):
			late = append(late, started(6, 1))
		case e.Source == model.SourceEngineLog && e.TS.Equal(at(3.8)):
			late = append(late, logLine(6.8))
		case e.Kind == model.KindPodReady:
			late = append(late, status(model.KindPodReady, 7))
		default:
			late = append(late, e)
		}
	}
	r = mustAssign(t, late, Options{})
	want = []model.Gap{
		{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: 3, Reason: model.GapEarlierAttempts},
		{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: 3},
	}
	if !reflect.DeepEqual(r.Gaps, want) {
		t.Errorf("late re-pull gaps = %+v, want %+v", r.Gaps, want)
	}
}
