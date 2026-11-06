package phase

import (
	"cmp"
	"math"
	"math/rand"
	"reflect"
	"slices"
	"testing"
	"testing/quick"

	"github.com/AmitwalaH/doriki/internal/model"
)

// propertyMarkers is a pack that declares a marker for every engine phase, so random inputs
// reach every phase rule.
var propertyMarkers = []string{
	"x.weights.fetch.start", "x.weights.fetch.end", "x.weights.load.start", "x.weights.load.end",
	"x.compile.start", "x.compile.end", "x.graph.start", "x.graph.end", "x.kv.start", "x.kv.end",
	"x.server.listening", "x.request.first_token",
}

// eventSet is a random multiset of the events Assign understands, with repeats, restarts,
// ends before starts and equal timestamps across sources.
type eventSet []model.Event

func (eventSet) Generate(r *rand.Rand, _ int) reflect.Value {
	makers := []func(s float64) model.Event{
		func(s float64) model.Event { return status(model.KindPodCreated, s) },
		func(s float64) model.Event { return k8s(model.KindScheduled, s) },
		func(s float64) model.Event { return k8s(model.KindPulling, s) },
		func(s float64) model.Event { return k8s(model.KindPulled, s) },
		func(s float64) model.Event { return started(s, r.Intn(3)) },
		func(s float64) model.Event { return logLine(s) },
		func(s float64) model.Event { return status(model.KindPodReady, s) },
		func(s float64) model.Event { return marker(propertyMarkers[r.Intn(len(propertyMarkers))], s) },
	}
	n := r.Intn(40)
	events := make(eventSet, n)
	for i := range events {
		// Half-second steps over two minutes make equal timestamps common.
		events[i] = makers[r.Intn(len(makers))](float64(r.Intn(240)) / 2)
	}
	return reflect.ValueOf(events)
}

func inputSpan(events []model.Event) float64 {
	if len(events) == 0 {
		return 0
	}
	ts := make([]float64, len(events))
	for i, e := range events {
		ts[i] = e.TS.Sub(t0).Seconds()
	}
	return slices.Max(ts) - slices.Min(ts)
}

// checkAttempt holds for the top-level report and for every listed attempt: phases come in
// the fixed order, no phase starts before the phase above it, and totals are between 0 and
// the wall-clock span of the input.
func checkAttempt(t *testing.T, phases []model.Phase, totals model.Totals, span float64) bool {
	t.Helper()
	names := make([]model.PhaseName, len(phases))
	for i, p := range phases {
		names[i] = p.Name
	}
	if !slices.Equal(names, model.Phases()) {
		t.Logf("phase order %v", names)
		return false
	}
	var prev *model.Phase
	for i := range phases {
		p := &phases[i]
		if p.State == model.StateAbsent {
			continue
		}
		if prev != nil && p.Start.Before(prev.Start) {
			t.Logf("%s starts at %v, before %s at %v", p.Name, p.Start, prev.Name, prev.Start)
			return false
		}
		prev = p
	}
	for _, v := range []*float64{totals.PendingToReadySeconds, totals.PendingToFirstTokenSeconds} {
		if v != nil && (*v < 0 || *v > span) {
			t.Logf("total %v outside [0, %v]", *v, span)
			return false
		}
	}
	return true
}

// DESIGN.md section 9.4: for 1,000 random event multisets, Assign gives the same report for
// any order of the same events, keeps the fixed phase order, and never reports a total
// longer than the input's wall-clock span. The seed is fixed so a failure reproduces.
func TestAssignProperties(t *testing.T) {
	property := func(events eventSet, shuffleSeed int64, allAttempts bool) bool {
		opts := Options{Markers: propertyMarkers, AllAttempts: allAttempts}
		want, err := Assign(events, opts)
		if err != nil {
			t.Logf("Assign: %v", err)
			return false
		}
		shuffled := slices.Clone(events)
		rand.New(rand.NewSource(shuffleSeed)).Shuffle(len(shuffled), func(i, j int) { //nolint:gosec // G404: seeded on purpose so a property failure reproduces
			shuffled[i], shuffled[j] = shuffled[j], shuffled[i]
		})
		got, err := Assign(shuffled, opts)
		if err != nil || !reflect.DeepEqual(got, want) {
			t.Logf("shuffled input changed the report (err %v)", err)
			return false
		}

		span := inputSpan(events)
		if !checkAttempt(t, got.Phases, got.Totals, span) {
			return false
		}
		for _, a := range got.Attempts {
			if !checkAttempt(t, a.Phases, a.Totals, span) {
				t.Logf("in attempt %d", a.Attempt)
				return false
			}
		}
		return true
	}
	cfg := &quick.Config{MaxCount: 1000, Rand: rand.New(rand.NewSource(1))} //nolint:gosec // G404: seeded on purpose so a property failure reproduces
	if err := quick.Check(property, cfg); err != nil {
		t.Error(err)
	}
}

// The generator must reach the cases the properties are about; a generator that only made
// empty sets would pass every property.
func TestPropertyGeneratorCoverage(t *testing.T) {
	r := rand.New(rand.NewSource(1)) //nolint:gosec // G404: seeded on purpose so a property failure reproduces
	seen := map[model.PhaseState]int{}
	restarts, ties := 0, 0
	for range 1000 {
		events := eventSet{}.Generate(r, 0).Interface().(eventSet)
		rep, err := Assign(events, Options{Markers: propertyMarkers})
		if err != nil {
			t.Fatalf("Assign: %v", err)
		}
		for _, p := range rep.Phases {
			seen[p.State]++
		}
		if rep.AttemptsTotal > 1 {
			restarts++
		}
		sorted := slices.Clone(events)
		slices.SortFunc(sorted, model.CompareEvents)
		for i := 1; i < len(sorted); i++ {
			if sorted[i].TS.Equal(sorted[i-1].TS) && sorted[i].Source != sorted[i-1].Source {
				ties++
				break
			}
		}
	}
	for _, s := range []model.PhaseState{model.StateComplete, model.StateOpen, model.StateAbsent, model.StateInvalid} {
		if seen[s] == 0 {
			t.Errorf("generator never produced a %s phase", s)
		}
	}
	if restarts == 0 || ties == 0 {
		t.Errorf("restarts %d, cross-source ties %d: want both > 0", restarts, ties)
	}
	t.Logf("states %v, inputs with restarts %d, with cross-source ties %d", seen, restarts, ties)
}

// uncovered is the input's span minus the union of the complete phases, in seconds: the most
// time the report can honestly call a gap. A phase whose end is not after its start covers
// nothing.
func uncovered(phases []model.Phase, span float64) float64 {
	type interval struct{ start, end float64 }
	var in []interval
	for _, p := range phases {
		if p.State == model.StateComplete && p.End.After(p.Start) {
			in = append(in, interval{p.Start.Sub(t0).Seconds(), p.End.Sub(t0).Seconds()})
		}
	}
	slices.SortFunc(in, func(a, b interval) int { return cmp.Compare(a.start, b.start) })
	covered, reach := 0.0, math.Inf(-1)
	for _, iv := range in {
		if iv.start > reach {
			reach = iv.start
		}
		if iv.end > reach {
			covered += iv.end - reach
			reach = iv.end
		}
	}
	return span - covered
}

// DK-PHASE-BUG-20261029-gap-after-nested-phase: for the same random inputs as
// TestAssignProperties, the gaps of the report and of every listed attempt never add up to
// more than the span minus the union of complete phases, so no time a phase covers is ever
// also reported as a gap, however phases overlap or nest.
func TestGapsNeverCoverAPhase(t *testing.T) {
	property := func(events eventSet, allAttempts bool) bool {
		r, err := Assign(events, Options{Markers: propertyMarkers, AllAttempts: allAttempts})
		if err != nil {
			t.Logf("Assign: %v", err)
			return false
		}
		span := inputSpan(events)
		check := func(phases []model.Phase, gaps []model.Gap) bool {
			sum := 0.0
			for _, g := range gaps {
				sum += g.Seconds
			}
			if limit := uncovered(phases, span); sum > limit+1e-9 {
				t.Logf("gaps %+v add up to %v s, more than the %v s no complete phase covers", gaps, sum, limit)
				for _, p := range phases {
					if p.State == model.StateComplete {
						t.Logf("  %s %v to %v", p.Name, p.Start.Sub(t0).Seconds(), p.End.Sub(t0).Seconds())
					}
				}
				return false
			}
			return true
		}
		if !check(r.Phases, r.Gaps) {
			return false
		}
		for _, a := range r.Attempts {
			if !check(a.Phases, a.Gaps) {
				t.Logf("in attempt %d", a.Attempt)
				return false
			}
		}
		return true
	}
	cfg := &quick.Config{MaxCount: 1000, Rand: rand.New(rand.NewSource(1))} //nolint:gosec // G404: seeded on purpose so a property failure reproduces
	if err := quick.Check(property, cfg); err != nil {
		t.Error(err)
	}
}

// readyAt is the Ready time the totals imply, in seconds from t0: pod creation, the earliest
// PodCreated as Assign takes it, plus pending_to_ready. ok is false when there is no total.
func readyAt(events []model.Event, totals model.Totals) (float64, bool) {
	created := math.Inf(1)
	for _, e := range events {
		if e.Source == model.SourceContainerStatus && e.Kind == model.KindPodCreated {
			created = min(created, e.TS.Sub(t0).Seconds())
		}
	}
	if totals.PendingToReadySeconds == nil || math.IsInf(created, 1) {
		return 0, false
	}
	return created + *totals.PendingToReadySeconds, true
}

// owed is the least time, in seconds, the gaps must account for between the earliest complete
// phase and ready. Each stretch no complete phase covers, between two consecutive phase
// bounds, may go unreported by up to 2 s: an attempt boundary can split it into two pieces,
// each left out when 1 s or less (gapOver). A complete phase covers from its earlier time to
// its later one, so a phase clamped to 0 s still bounds the stretches beside it.
func owed(phases []model.Phase, ready float64) float64 {
	type interval struct{ lo, hi float64 }
	var in []interval
	for _, p := range phases {
		if p.State == model.StateComplete {
			s, e := p.Start.Sub(t0).Seconds(), p.End.Sub(t0).Seconds()
			in = append(in, interval{min(s, e), max(s, e)})
		}
	}
	if len(in) == 0 {
		return 0
	}
	slices.SortFunc(in, func(a, b interval) int { return cmp.Compare(a.lo, b.lo) })
	slack := 2 * gapOver.Seconds()
	total, reach := 0.0, in[0].lo
	stretch := func(from, to float64) {
		if d := min(to, ready) - from; d > slack {
			total += d - slack
		}
	}
	for _, iv := range in {
		if iv.lo > reach {
			stretch(reach, iv.lo)
		}
		reach = max(reach, iv.hi)
	}
	stretch(reach, ready)
	return total
}

// DK-PHASE-ENH-20261024-unattributed-before-ready: for the same random inputs, once the pod is
// Ready the bars add up to the total. From the earliest complete phase to Ready, the gaps of
// the report and of every listed attempt account for all the time no complete phase covers,
// short of the threshold at the ends of each stretch (see owed). Without a gap to Ready, an
// input whose last complete phase ends more than 2 s before Ready fails.
func TestBarsAddUpWhenReady(t *testing.T) {
	reached := 0 // inputs where the gaps owed something, so the property was not vacuous
	property := func(events eventSet, allAttempts bool) bool {
		r, err := Assign(events, Options{Markers: propertyMarkers, AllAttempts: allAttempts})
		if err != nil {
			t.Logf("Assign: %v", err)
			return false
		}
		check := func(phases []model.Phase, gaps []model.Gap, totals model.Totals) bool {
			ready, ok := readyAt(events, totals)
			if !ok {
				return true
			}
			sum := 0.0
			for _, g := range gaps {
				sum += g.Seconds
			}
			want := owed(phases, ready)
			if want > 0 {
				reached++
			}
			if sum < want-1e-9 {
				t.Logf("Ready at %v: gaps %+v add up to %v s, less than the %v s owed", ready, gaps, sum, want)
				for _, p := range phases {
					if p.State == model.StateComplete {
						t.Logf("  %s %v to %v", p.Name, p.Start.Sub(t0).Seconds(), p.End.Sub(t0).Seconds())
					}
				}
				return false
			}
			return true
		}
		if !check(r.Phases, r.Gaps, r.Totals) {
			return false
		}
		for _, a := range r.Attempts {
			if !check(a.Phases, a.Gaps, a.Totals) {
				t.Logf("in attempt %d", a.Attempt)
				return false
			}
		}
		return true
	}
	cfg := &quick.Config{MaxCount: 1000, Rand: rand.New(rand.NewSource(1))} //nolint:gosec // G404: seeded on purpose so a property failure reproduces
	if err := quick.Check(property, cfg); err != nil {
		t.Error(err)
	}
	if reached == 0 {
		t.Error("no input owed any gap time; the generator never reached the case")
	}
	t.Logf("inputs or attempts that owed gap time: %d", reached)
}
