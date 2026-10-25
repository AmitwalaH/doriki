package phase

import (
	"reflect"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

var t0 = time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)

func at(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }

func k8s(kind string, s float64) model.Event {
	return model.Event{TS: at(s), Source: model.SourceK8sEvent, Kind: kind, Confidence: 1}
}

func status(kind string, s float64) model.Event {
	return model.Event{TS: at(s), Source: model.SourceContainerStatus, Kind: kind, Confidence: 1}
}

func started(s float64, restartCount int) model.Event {
	e := status(model.KindContainerStarted, s)
	e.Attrs = map[string]string{model.AttrRestartCount: strconv.Itoa(restartCount)}
	return e
}

func logLine(s float64) model.Event {
	return model.Event{TS: at(s), Source: model.SourceEngineLog, Kind: model.KindLogLine, Confidence: 1}
}

func marker(id string, s float64) model.Event {
	return model.Event{TS: at(s), Source: model.SourceEngineLog, Kind: model.KindLogMarker, Marker: id, Confidence: 1}
}

func secs(f float64) *float64 { return &f }

// A plain boot with no engine pack: the generic pack's view.
func genericBoot() []model.Event {
	return []model.Event{
		status(model.KindPodCreated, 0),
		k8s(model.KindScheduled, 1),
		k8s(model.KindPulling, 3),
		k8s(model.KindPulled, 10),
		started(11, 0),
		logLine(12),
		logLine(15),
		status(model.KindPodReady, 20),
	}
}

func mustAssign(t *testing.T, events []model.Event, opts Options) model.Report {
	t.Helper()
	r, err := Assign(events, opts)
	if err != nil {
		t.Fatalf("Assign: %v", err)
	}
	return r
}

func phaseByName(r model.Report, name model.PhaseName) model.Phase {
	for _, p := range r.Phases {
		if p.Name == name {
			return p
		}
	}
	return model.Phase{}
}

func assertPhase(t *testing.T, got, want model.Phase) {
	t.Helper()
	if !reflect.DeepEqual(got, want) {
		t.Errorf("phase %s:\n got %+v (seconds %v)\nwant %+v (seconds %v)",
			want.Name, got, deref(got.Seconds), want, deref(want.Seconds))
	}
}

func deref(f *float64) any {
	if f == nil {
		return nil
	}
	return *f
}

// The M1a goal: Kubernetes phases complete, every engine phase absent, including
// server-ready and first-token even though Ready=True is observed, because the generic pack
// declares no engine markers. Gap and total come from the same events.
func TestAssignGenericBoot(t *testing.T) {
	r := mustAssign(t, genericBoot(), Options{})

	cs, k := model.SourceContainerStatus, model.SourceK8sEvent
	want := []model.Phase{
		{Name: model.PhaseScheduling, Start: at(0), End: at(1), Seconds: secs(1), State: model.StateComplete, Sources: []model.Source{k, cs}},
		{Name: model.PhaseNodeProvision, State: model.StateAbsent},
		{Name: model.PhaseImagePull, Start: at(3), End: at(10), Seconds: secs(7), State: model.StateComplete, Sources: []model.Source{k}},
		{Name: model.PhaseContainerInit, Start: at(11), End: at(12), Seconds: secs(1), State: model.StateComplete, Sources: []model.Source{cs, model.SourceEngineLog}},
		{Name: model.PhaseEngineInit, State: model.StateAbsent},
		{Name: model.PhaseWeightFetch, State: model.StateAbsent},
		{Name: model.PhaseWeightLoad, State: model.StateAbsent},
		{Name: model.PhaseCompile, State: model.StateAbsent},
		{Name: model.PhaseGraphCapture, State: model.StateAbsent},
		{Name: model.PhaseKVAlloc, State: model.StateAbsent},
		{Name: model.PhaseServerReady, State: model.StateAbsent},
		{Name: model.PhaseFirstToken, State: model.StateAbsent},
	}
	if len(r.Phases) != len(want) {
		t.Fatalf("got %d phases, want %d", len(r.Phases), len(want))
	}
	for i := range want {
		assertPhase(t, r.Phases[i], want[i])
	}

	// scheduling ends at 1, image-pull starts at 3: 2 s unattributed. image-pull to
	// container-init is exactly 1 s, which is not over the threshold.
	wantGaps := []model.Gap{{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: 2}}
	if !reflect.DeepEqual(r.Gaps, wantGaps) {
		t.Errorf("gaps = %+v, want %+v", r.Gaps, wantGaps)
	}
	if got := deref(r.Totals.PendingToReadySeconds); got != 20.0 {
		t.Errorf("pending_to_ready = %v, want 20", got)
	}
	if r.Totals.PendingToFirstTokenSeconds != nil {
		t.Errorf("pending_to_first_token = %v, want omitted", *r.Totals.PendingToFirstTokenSeconds)
	}
	if r.Attempt != 0 || r.AttemptsTotal != 1 || r.Attempts != nil || r.Warnings != nil {
		t.Errorf("attempt %d of %d, attempts %v, warnings %v; want 0 of 1, none, none",
			r.Attempt, r.AttemptsTotal, r.Attempts, r.Warnings)
	}
}

// Missing start is absent; a start with no end is open (DESIGN.md section 6.1 step 3).
func TestAbsentAndOpen(t *testing.T) {
	noPulled := slices.DeleteFunc(genericBoot(), func(e model.Event) bool { return e.Kind == model.KindPulled })
	assertPhase(t, phaseByName(mustAssign(t, noPulled, Options{}), model.PhaseImagePull),
		model.Phase{Name: model.PhaseImagePull, Start: at(3), State: model.StateOpen, Sources: []model.Source{model.SourceK8sEvent}})

	noPull := slices.DeleteFunc(genericBoot(), func(e model.Event) bool {
		return e.Kind == model.KindPulled || e.Kind == model.KindPulling
	})
	assertPhase(t, phaseByName(mustAssign(t, noPull, Options{}), model.PhaseImagePull),
		model.Phase{Name: model.PhaseImagePull, State: model.StateAbsent})
}

// An end more than 1 s before its start is invalid: it is listed in warnings, has no
// seconds, and is skipped by gaps. Within 1 s it is timestamp resolution: complete, 0 s.
func TestInvalidPhase(t *testing.T) {
	events := []model.Event{
		status(model.KindPodCreated, 0), k8s(model.KindScheduled, 1),
		k8s(model.KindPulled, 5), k8s(model.KindPulling, 10),
		started(11, 0), logLine(14), status(model.KindPodReady, 20),
	}
	r := mustAssign(t, events, Options{})
	assertPhase(t, phaseByName(r, model.PhaseImagePull), model.Phase{
		Name: model.PhaseImagePull, Start: at(10), End: at(5), State: model.StateInvalid,
		Sources: []model.Source{model.SourceK8sEvent},
	})
	wantWarn := []model.Warning{{Code: "phase-invalid", Text: "image-pull ends 5.0s before it starts; left out of gaps and totals"}}
	if !reflect.DeepEqual(r.Warnings, wantWarn) {
		t.Errorf("warnings = %+v, want %+v", r.Warnings, wantWarn)
	}
	// With image-pull invalid, the next complete pair is scheduling (ends 1) and
	// container-init (starts 11).
	wantGaps := []model.Gap{{After: model.PhaseScheduling, Before: model.PhaseContainerInit, Seconds: 10}}
	if !reflect.DeepEqual(r.Gaps, wantGaps) {
		t.Errorf("gaps = %+v, want %+v", r.Gaps, wantGaps)
	}

	events[2] = k8s(model.KindPulled, 9.5)
	assertPhase(t, phaseByName(mustAssign(t, events, Options{}), model.PhaseImagePull), model.Phase{
		Name: model.PhaseImagePull, Start: at(10), End: at(9.5), Seconds: secs(0), State: model.StateComplete,
		Sources: []model.Source{model.SourceK8sEvent},
	})
}

// A pack that declares markers makes engine phases possible. A declared phase with no
// observed start is absent; first-token, declared but with no first-token event yet, is
// open from Ready. Overlapping phases (fetch and load at once) give no gap.
func TestEnginePhases(t *testing.T) {
	opts := Options{Markers: []string{
		"vllm.weights.fetch.start", "vllm.weights.fetch.end",
		"vllm.weights.load.start", "vllm.weights.load.end",
		"vllm.compile.start", "vllm.compile.end",
		"vllm.server.listening", "vllm.request.first_token",
	}}
	events := append(genericBoot(),
		marker("vllm.weights.load.start", 5), // before the container started: not this boot's
		marker("vllm.weights.fetch.start", 13),
		marker("vllm.weights.load.start", 14),
		marker("vllm.weights.fetch.end", 16),
		marker("vllm.weights.load.end", 17),
		marker("vllm.server.listening", 18),
	)
	r := mustAssign(t, events, opts)
	el := model.SourceEngineLog
	assertPhase(t, phaseByName(r, model.PhaseWeightFetch), model.Phase{
		Name: model.PhaseWeightFetch, Start: at(13), End: at(16), Seconds: secs(3), State: model.StateComplete, Sources: []model.Source{el}})
	assertPhase(t, phaseByName(r, model.PhaseWeightLoad), model.Phase{
		Name: model.PhaseWeightLoad, Start: at(14), End: at(17), Seconds: secs(3), State: model.StateComplete, Sources: []model.Source{el}})
	assertPhase(t, phaseByName(r, model.PhaseCompile), model.Phase{Name: model.PhaseCompile, State: model.StateAbsent})
	assertPhase(t, phaseByName(r, model.PhaseServerReady), model.Phase{
		Name: model.PhaseServerReady, Start: at(18), End: at(20), Seconds: secs(2), State: model.StateComplete,
		Sources: []model.Source{model.SourceContainerStatus, el}})
	assertPhase(t, phaseByName(r, model.PhaseFirstToken), model.Phase{
		Name: model.PhaseFirstToken, Start: at(20), State: model.StateOpen, Sources: []model.Source{model.SourceContainerStatus}})

	// container-init ends 12, weight-fetch starts 13: 1 s, no gap. fetch overlaps load.
	// load ends 17, server-ready starts 18: 1 s, no gap.
	wantGaps := []model.Gap{{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: 2}}
	if !reflect.DeepEqual(r.Gaps, wantGaps) {
		t.Errorf("gaps = %+v, want %+v", r.Gaps, wantGaps)
	}

	events = append(events, marker("vllm.request.first_token", 22.5))
	r = mustAssign(t, events, opts)
	if got := phaseByName(r, model.PhaseFirstToken); got.State != model.StateComplete || deref(got.Seconds) != 2.5 {
		t.Errorf("first-token = %+v, want complete 2.5 s", got)
	}
	if got := deref(r.Totals.PendingToFirstTokenSeconds); got != 22.5 {
		t.Errorf("pending_to_first_token = %v, want 22.5", got)
	}
}

// DESIGN.md section 9.3, events expired: only the pod object and the log are left. Under
// section 6.1 step 3 as written, scheduling has a start (creation) and no end, so it is open.
func TestEventsExpired(t *testing.T) {
	events := []model.Event{
		status(model.KindPodCreated, 0), started(11, 0), logLine(12), status(model.KindPodReady, 20),
	}
	r := mustAssign(t, events, Options{})
	assertPhase(t, phaseByName(r, model.PhaseScheduling), model.Phase{
		Name: model.PhaseScheduling, Start: at(0), State: model.StateOpen, Sources: []model.Source{model.SourceContainerStatus}})
	assertPhase(t, phaseByName(r, model.PhaseImagePull), model.Phase{Name: model.PhaseImagePull, State: model.StateAbsent})
	if got := phaseByName(r, model.PhaseContainerInit); got.State != model.StateComplete {
		t.Errorf("container-init = %+v, want complete", got)
	}
	if got := deref(r.Totals.PendingToReadySeconds); got != 20.0 {
		t.Errorf("pending_to_ready = %v, want 20", got)
	}

	// When the Scheduled event has expired, the collector emits Scheduled from the pod's
	// PodScheduled condition (container-status). It ends scheduling like the event does.
	events = append(events, status(model.KindScheduled, 2))
	assertPhase(t, phaseByName(mustAssign(t, events, Options{}), model.PhaseScheduling), model.Phase{
		Name: model.PhaseScheduling, Start: at(0), End: at(2), Seconds: secs(2), State: model.StateComplete,
		Sources: []model.Source{model.SourceContainerStatus}})
}

// DESIGN.md section 9.3, container restart mid-boot. The pod has restarted 3 times; the
// pod object only shows the last two starts. The latest attempt is reported by default and
// does not repeat scheduling or image-pull; Ready belongs to it, so it carries the total.
func TestAttempts(t *testing.T) {
	events := []model.Event{
		status(model.KindPodCreated, 0), k8s(model.KindScheduled, 1),
		k8s(model.KindPulling, 2), k8s(model.KindPulled, 10),
		started(40, 2), logLine(41),
		started(60, 3), logLine(63),
		status(model.KindPodReady, 70),
	}

	latest := mustAssign(t, events, Options{})
	if latest.Attempt != 3 || latest.AttemptsTotal != 4 || latest.Attempts != nil {
		t.Errorf("attempt %d of %d (list %v), want 3 of 4 and no list", latest.Attempt, latest.AttemptsTotal, latest.Attempts)
	}
	assertPhase(t, phaseByName(latest, model.PhaseScheduling), model.Phase{Name: model.PhaseScheduling, State: model.StateAbsent})
	assertPhase(t, phaseByName(latest, model.PhaseContainerInit), model.Phase{
		Name: model.PhaseContainerInit, Start: at(60), End: at(63), Seconds: secs(3), State: model.StateComplete,
		Sources: []model.Source{model.SourceContainerStatus, model.SourceEngineLog}})
	if got := deref(latest.Totals.PendingToReadySeconds); got != 70.0 {
		t.Errorf("latest pending_to_ready = %v, want 70", got)
	}

	two := 2
	earlier := mustAssign(t, events, Options{Attempt: &two})
	if earlier.Attempt != 2 {
		t.Errorf("attempt = %d, want 2", earlier.Attempt)
	}
	// The earliest observed attempt carries the pre-start phases. Ready came after the
	// next start, so this attempt has no total.
	if got := phaseByName(earlier, model.PhaseImagePull); got.State != model.StateComplete || deref(got.Seconds) != 8.0 {
		t.Errorf("attempt 2 image-pull = %+v, want complete 8 s", got)
	}
	if got := phaseByName(earlier, model.PhaseContainerInit); deref(got.Seconds) != 1.0 {
		t.Errorf("attempt 2 container-init = %+v, want 1 s", got)
	}
	if earlier.Totals.PendingToReadySeconds != nil {
		t.Errorf("attempt 2 pending_to_ready = %v, want omitted", *earlier.Totals.PendingToReadySeconds)
	}

	all := mustAssign(t, events, Options{AllAttempts: true})
	if len(all.Attempts) != 2 || all.Attempts[0].Attempt != 2 || all.Attempts[1].Attempt != 3 {
		t.Fatalf("attempts = %+v, want 2 then 3", all.Attempts)
	}
	if !reflect.DeepEqual(all.Attempts[1].Phases, latest.Phases) || !reflect.DeepEqual(all.Attempts[0].Phases, earlier.Phases) {
		t.Errorf("attempts list does not match the per-attempt reports")
	}

	zero := 0
	if _, err := Assign(events, Options{Attempt: &zero}); err == nil {
		t.Errorf("attempt 0 was not observed, want an error")
	}
}

// Assign must use model.Phases, the single encoding of the order. A sentinel order must
// show up in the output; a private copy of the real order would ignore it.
func TestUsesSharedPhaseOrder(t *testing.T) {
	saved := phaseOrder
	t.Cleanup(func() { phaseOrder = saved })
	phaseOrder = func() []model.PhaseName { return []model.PhaseName{model.PhaseImagePull, model.PhaseScheduling} }

	r := mustAssign(t, genericBoot(), Options{})
	var got []model.PhaseName
	for _, p := range r.Phases {
		got = append(got, p.Name)
	}
	if want := phaseOrder(); !slices.Equal(got, want) {
		t.Errorf("phases %v, want sentinel order %v", got, want)
	}
}

// Assign must sort with model.CompareEvents, the single encoding of the event order.
func TestUsesSharedEventOrder(t *testing.T) {
	saved := compareEvents
	t.Cleanup(func() { compareEvents = saved })
	calls := 0
	compareEvents = func(a, b model.Event) int { calls++; return model.CompareEvents(a, b) }

	mustAssign(t, genericBoot(), Options{})
	if calls == 0 {
		t.Errorf("Assign never called the shared event order")
	}
}

// Assign never changes the caller's events.
func TestAssignInputNotMutated(t *testing.T) {
	in := genericBoot()
	slices.Reverse(in)
	saved := slices.Clone(in)
	mustAssign(t, in, Options{})
	if !slices.EqualFunc(in, saved, func(a, b model.Event) bool { return model.CompareEvents(a, b) == 0 }) {
		t.Errorf("input changed")
	}
}

// Decision 0003: the phases a report is expected to complete are those the selected pack can
// produce. node-provision is optional (DESIGN.md section 4.2) and never expected.
func TestExpected(t *testing.T) {
	generic := Expected(nil)
	want := []model.PhaseName{model.PhaseScheduling, model.PhaseImagePull, model.PhaseContainerInit}
	if !slices.Equal(generic, want) {
		t.Errorf("Expected(generic) = %v, want %v", generic, want)
	}

	withLoad := Expected([]string{"vllm.weights.load.start", "vllm.weights.load.end", "vllm.server.listening"})
	want = []model.PhaseName{model.PhaseScheduling, model.PhaseImagePull, model.PhaseContainerInit,
		model.PhaseWeightLoad, model.PhaseServerReady}
	if !slices.Equal(withLoad, want) {
		t.Errorf("Expected(load+listening) = %v, want %v", withLoad, want)
	}

	// Expected and Assign share one rule: under any pack, a phase Assign can report as
	// something other than absent is either expected or optional.
	opts := Options{Markers: propertyMarkers}
	events := append(genericBoot(), marker("x.server.listening", 18), marker("x.request.first_token", 25))
	r := mustAssign(t, events, opts)
	for _, p := range r.Phases {
		if p.State != model.StateAbsent && !slices.Contains(Expected(opts.Markers), p.Name) {
			t.Errorf("%s is %s but not expected", p.Name, p.State)
		}
	}
}

func cachedPull(s float64) model.Event {
	e := k8s(model.KindPulled, s)
	e.Attrs = map[string]string{model.AttrCached: "true"}
	return e
}

// Decision 0004: with no Pulling event, a cached Pulled is a complete 0 s image-pull that
// starts and ends at that event and carries cached. An uncached Pulled alone (its Pulling
// expired) still leaves image-pull absent: only the kubelet's "already present" says there
// was no pull. The phase carries no other attribute of its events.
func TestCachedImagePull(t *testing.T) {
	events := []model.Event{
		status(model.KindPodCreated, 0),
		k8s(model.KindScheduled, 1),
		cachedPull(2),
		started(3, 0),
		logLine(4),
		status(model.KindPodReady, 5),
	}
	r := mustAssign(t, events, Options{})
	assertPhase(t, phaseByName(r, model.PhaseImagePull), model.Phase{
		Name: model.PhaseImagePull, Start: at(2), End: at(2), Seconds: secs(0), State: model.StateComplete,
		Attrs: map[string]string{model.AttrCached: "true"}, Sources: []model.Source{model.SourceK8sEvent},
	})
	if a := phaseByName(r, model.PhaseContainerInit).Attrs; a != nil {
		t.Errorf("container-init attrs = %v, want none (restart_count stays on the event)", a)
	}
	if !slices.Contains(Expected(nil), model.PhaseImagePull) {
		t.Errorf("image-pull is not expected, so a cached pull would not be checked by the exit code")
	}

	events[2] = k8s(model.KindPulled, 2)
	r = mustAssign(t, events, Options{})
	if p := phaseByName(r, model.PhaseImagePull); p.State != model.StateAbsent {
		t.Errorf("uncached Pulled alone: image-pull = %+v, want absent", p)
	}
}

func markerWith(id string, s float64, conf float32, attrs map[string]string) model.Event {
	e := marker(id, s)
	e.Confidence, e.Attrs = conf, attrs
	return e
}

var enginePack = []string{
	"x.weights.fetch.start", "x.weights.fetch.end", "x.weights.load.start", "x.weights.load.end",
	"x.compile.start", "x.compile.end", "x.compile.skipped", "x.graph.start", "x.graph.end",
	"x.kv.start", "x.kv.end", "x.server.listening",
}

// An engine boot: compile is skipped by the engine (a ".skipped" marker logged before the
// weights), graph-capture never logged, the pod Ready at 30.
func engineBoot() []model.Event {
	return append(genericBoot()[:6:6],
		markerWith("x.compile.skipped", 13, 1, nil),
		markerWith("x.weights.fetch.start", 14, 0.9, map[string]string{model.AttrStartDerived: "end minus 6 s reported by the engine", "model": "m"}),
		markerWith("x.weights.fetch.end", 20, 1, map[string]string{"model": "m", "seconds": "6"}),
		marker("x.weights.load.start", 20),
		markerWith("x.weights.load.end", 21, 1, map[string]string{"seconds": "1.0"}),
		marker("x.kv.start", 22),
		marker("x.kv.end", 23),
		marker("x.server.listening", 29),
		status(model.KindPodReady, 30),
	)
}

// Decision 0005 addition: a phase with no start but its ".skipped" marker is skipped, names
// the marker, has no time span, and does not move later phases' search back to the marker.
// A phase whose start is observed is not skipped even if the marker is there too.
func TestSkippedPhase(t *testing.T) {
	r := mustAssign(t, engineBoot(), Options{Markers: enginePack, Pack: "x/1"})
	assertPhase(t, phaseByName(r, model.PhaseCompile), model.Phase{
		Name: model.PhaseCompile, State: model.StateSkipped,
		Attrs: map[string]string{"skipped_by": "x.compile.skipped"}, Sources: []model.Source{model.SourceEngineLog},
	})
	if kv := phaseByName(r, model.PhaseKVAlloc); kv.State != model.StateComplete || !kv.Start.Equal(at(22)) {
		t.Errorf("kv-alloc = %+v, want complete from 22", kv)
	}

	events := append(engineBoot(), marker("x.compile.start", 21.5), marker("x.compile.end", 21.8))
	if c := phaseByName(mustAssign(t, events, Options{Markers: enginePack}), model.PhaseCompile); c.State != model.StateComplete {
		t.Errorf("compile observed and skipped marker present: %+v, want complete", c)
	}
}

// A phase takes the attributes of its engine markers (end wins on a shared key) and the
// lowest confidence of its two events when below 1.0. Kubernetes and log-line events add no
// attributes: container-init ends at the first engine log event, which carries none here.
func TestEngineAttrsAndConfidence(t *testing.T) {
	r := mustAssign(t, engineBoot(), Options{Markers: enginePack})
	conf := float32(0.9)
	assertPhase(t, phaseByName(r, model.PhaseWeightFetch), model.Phase{
		Name: model.PhaseWeightFetch, Start: at(14), End: at(20), Seconds: secs(6), State: model.StateComplete,
		Attrs:      map[string]string{model.AttrStartDerived: "end minus 6 s reported by the engine", "model": "m", "seconds": "6"},
		Sources:    []model.Source{model.SourceEngineLog},
		Confidence: &conf,
	})
	if p := phaseByName(r, model.PhaseWeightLoad); p.Confidence != nil || p.Attrs["seconds"] != "1.0" {
		t.Errorf("weight-load = %+v, want no confidence written and seconds 1.0", p)
	}
	if p := phaseByName(r, model.PhaseContainerInit); p.Attrs != nil || p.Confidence != nil {
		t.Errorf("container-init = %+v, want no attributes or confidence", p)
	}
}

// Decision 0005: once the pod is Ready, an expected engine phase that is still absent gets a
// phase-unobserved warning naming the phase and the pack. Before Ready there is none (the pod
// may still be booting), and a skipped phase never gets one.
func TestPhaseUnobserved(t *testing.T) {
	r := mustAssign(t, engineBoot(), Options{Markers: enginePack, Pack: "x/1"})
	want := []model.Warning{{
		Code: model.WarnPhaseUnobserved, Phase: model.PhaseGraphCapture,
		Text: "graph-capture: no log line matched before the pod was Ready (pack x/1); the engine may have skipped it, or this engine version words it differently",
	}}
	if !reflect.DeepEqual(r.Warnings, want) {
		t.Errorf("warnings = %+v\nwant %+v", r.Warnings, want)
	}

	notReady := slices.DeleteFunc(engineBoot(), func(e model.Event) bool { return e.Kind == model.KindPodReady })
	if w := mustAssign(t, notReady, Options{Markers: enginePack, Pack: "x/1"}).Warnings; len(w) != 0 {
		t.Errorf("not Ready: warnings = %v, want none", w)
	}

	// A Kubernetes phase absent after Ready is not excused: no phase-unobserved for it.
	noPull := slices.DeleteFunc(engineBoot(), func(e model.Event) bool { return e.Kind == model.KindPulling })
	for _, w := range mustAssign(t, noPull, Options{Markers: enginePack, Pack: "x/1"}).Warnings {
		if w.Phase == model.PhaseImagePull {
			t.Errorf("image-pull got %+v", w)
		}
	}
}

// Decision 0006: engine-init runs from the first engine log line (where container-init ends)
// to the pack's ".engine.init.end" marker. Under a pack that does not declare that marker,
// such as generic, it is absent and not expected.
func TestEngineInit(t *testing.T) {
	events := append(engineBoot(), marker("x.engine.init.end", 13.5))
	r := mustAssign(t, events, Options{Markers: append(slices.Clone(enginePack), "x.engine.init.end")})
	assertPhase(t, phaseByName(r, model.PhaseEngineInit), model.Phase{
		Name: model.PhaseEngineInit, Start: at(12), End: at(13.5), Seconds: secs(1.5), State: model.StateComplete,
		Sources: []model.Source{model.SourceEngineLog},
	})
	if ci := phaseByName(r, model.PhaseContainerInit); !ci.End.Equal(at(12)) {
		t.Errorf("container-init ends at %v, want 12, where engine-init starts", ci.End)
	}

	r = mustAssign(t, genericBoot(), Options{})
	if p := phaseByName(r, model.PhaseEngineInit); p.State != model.StateAbsent {
		t.Errorf("generic: engine-init = %+v, want absent", p)
	}
	if slices.Contains(Expected(nil), model.PhaseEngineInit) {
		t.Errorf("generic: engine-init expected")
	}
}
