package phase

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Thresholds from DESIGN.md section 6.1 steps 4 and 5.
const (
	invalidAfter = time.Second // an end this far before its start is a clock bug or restart
	gapOver      = time.Second // unattributed time longer than this is reported as a gap
)

// Variables so tests can swap in sentinels and prove Assign uses the shared encodings.
var (
	phaseOrder    = model.Phases
	compareEvents = model.CompareEvents
)

// Options select what Assign reports. The zero value reports the latest attempt with no
// engine phases.
type Options struct {
	// Markers are the marker ids the selected pack declares. An engine phase whose markers
	// no declared id ends with cannot be observed, so it is absent rather than open.
	Markers []string
	// Attempt selects a boot attempt by restart count; nil means the latest.
	Attempt *int
	// AllAttempts also fills Report.Attempts with every observed attempt.
	AllAttempts bool
	// Pack names the selected pack in warnings.
	Pack string
	// NotApplicable are the pack's phases this engine never has (decision 0008).
	NotApplicable []model.PhaseName
}

// matcher recognises the events that start or end a phase.
type matcher struct {
	kind         string       // matches Event.Kind, when set
	source       model.Source // with kind set, empty means any source
	attr         string       // with kind set, the event must also have this attribute "true"
	markerSuffix string       // matches Event.Marker ending in this, when set
}

func (m matcher) match(e model.Event) bool {
	switch {
	case m.markerSuffix != "":
		return e.Source == model.SourceEngineLog && strings.HasSuffix(e.Marker, m.markerSuffix)
	case m.kind != "":
		return e.Kind == m.kind && (m.source == "" || e.Source == m.source) && (m.attr == "" || e.Attrs[m.attr] == "true")
	default: // any event from the source
		return m.source != "" && e.Source == m.source
	}
}

// observable reports whether the selected pack can ever produce this matcher's event.
func (m matcher) observable(markers []string) bool {
	if m.markerSuffix == "" {
		return true
	}
	return slices.ContainsFunc(markers, func(id string) bool { return strings.HasSuffix(id, m.markerSuffix) })
}

// spec is one row of DESIGN.md section 4.2's table. A zero start matcher never matches.
type spec struct{ start, end matcher }

// phaseAttrs are the attributes a phase takes from a start event of any source. A phase
// takes every attribute of its engine markers (named groups from the pack) except
// collectorAttrs: collector bookkeeping such as the restart count stays on the event.
var (
	phaseAttrs     = []string{model.AttrCached}
	collectorAttrs = []string{model.AttrRestartCount}
)

// podLevel phases happen once per pod, not once per container start, so every attempt shows
// the pod's own (decision 0007).
var podLevel = []model.PhaseName{model.PhaseScheduling, model.PhaseNodeProvision}

// engine reports whether the phase is bounded by a pack marker. Once the pod is Ready, an
// absent engine phase was skipped or not recognised, not pending (decision 0005).
func (s spec) engine() bool { return s.start.markerSuffix != "" || s.end.markerSuffix != "" }

// skipSuffix is the marker suffix an engine logs when it skips the phase: ".compile.start"
// gives ".compile.skipped" (decision 0005). Phases whose start is not a ".start" marker
// cannot be skipped.
func (s spec) skipSuffix() string {
	if base, ok := strings.CutSuffix(s.start.markerSuffix, ".start"); ok {
		return base + ".skipped"
	}
	return ""
}

// optional phases only happen in some boots (a node created for the pod), so their absence
// is normal and never makes a report incomplete.
var optional = []model.PhaseName{model.PhaseNodeProvision}

// producible reports whether the selected pack can ever produce both of the phase's events.
func (s spec) producible(markers []string) bool {
	return s.start.observable(markers) && s.end.observable(markers)
}

// Expected returns, in the fixed order, the phases a boot is expected to complete under a
// pack declaring these markers: every non-optional phase the pack can produce. A report
// with any of these open or absent is incomplete (DESIGN.md section 7.3, decision 0003).
func Expected(markers []string) []model.PhaseName {
	var out []model.PhaseName
	for _, name := range phaseOrder() {
		if !slices.Contains(optional, name) && specs[name].producible(markers) {
			out = append(out, name)
		}
	}
	return out
}

var (
	podCreated = matcher{kind: model.KindPodCreated, source: model.SourceContainerStatus}
	podReady   = matcher{kind: model.KindPodReady, source: model.SourceContainerStatus}
)

var specs = map[model.PhaseName]spec{
	// Scheduled is the k8s event, or the PodScheduled condition when the event has expired.
	model.PhaseScheduling: {podCreated, matcher{kind: model.KindScheduled}},
	// node-provision needs the node's Ready time, which no collector reads yet.
	model.PhaseNodeProvision: {},
	model.PhaseImagePull: {
		matcher{kind: model.KindPulling, source: model.SourceK8sEvent},
		matcher{kind: model.KindPulled, source: model.SourceK8sEvent},
	},
	model.PhaseContainerInit: {
		matcher{kind: model.KindContainerStarted, source: model.SourceContainerStatus},
		matcher{source: model.SourceEngineLog}, // the first engine log line
	},
	// engine-init starts where container-init ends, at the first engine log line; the pack
	// says where it ends (decision 0006).
	model.PhaseEngineInit:   {matcher{source: model.SourceEngineLog}, matcher{markerSuffix: ".engine.init.end"}},
	model.PhaseWeightFetch:  {matcher{markerSuffix: ".weights.fetch.start"}, matcher{markerSuffix: ".weights.fetch.end"}},
	model.PhaseWeightLoad:   {matcher{markerSuffix: ".weights.load.start"}, matcher{markerSuffix: ".weights.load.end"}},
	model.PhaseCompile:      {matcher{markerSuffix: ".compile.start"}, matcher{markerSuffix: ".compile.end"}},
	model.PhaseGraphCapture: {matcher{markerSuffix: ".graph.start"}, matcher{markerSuffix: ".graph.end"}},
	model.PhaseKVAlloc:      {matcher{markerSuffix: ".kv.start"}, matcher{markerSuffix: ".kv.end"}},
	model.PhaseServerReady:  {matcher{markerSuffix: ".server.listening"}, podReady},
	// --probe will be a second way to end first-token (M3).
	model.PhaseFirstToken: {podReady, matcher{markerSuffix: ".request.first_token"}},
}

// altStarts are second events that can start a phase. An image already on the node has no
// Pulling, only a cached Pulled: a complete 0 s pull that starts and ends at that event
// (decision 0004).
var altStarts = map[model.PhaseName]matcher{
	model.PhaseImagePull: {kind: model.KindPulled, source: model.SourceK8sEvent, attr: model.AttrCached},
}

func startsAt(name model.PhaseName, s spec, e model.Event) bool {
	return s.start.match(e) || altStarts[name].match(e)
}

// Assign turns events into phases, gaps and totals for one boot attempt (DESIGN.md section
// 6.1 steps 3 to 7). It is pure: it sorts a copy of events with the shared order, so any
// permutation of the same events gives the same report. Callers normalize events first and
// fill the report's version, time, manifest and events themselves.
func Assign(events []model.Event, opts Options) (model.Report, error) {
	sorted := slices.Clone(events)
	slices.SortFunc(sorted, compareEvents)

	attempts := splitAttempts(sorted)
	chosen := len(attempts) - 1
	if opts.Attempt != nil {
		chosen = slices.IndexFunc(attempts, func(a attempt) bool { return a.number == *opts.Attempt })
		if chosen < 0 {
			return model.Report{}, fmt.Errorf("attempt %d was not observed; observed: %s", *opts.Attempt, attemptNumbers(attempts))
		}
	}

	created, hasCreated := first(sorted, podCreated)
	pod := podPhases(sorted, opts.Markers)
	report := model.Report{ReportVersion: model.ReportVersion, AttemptsTotal: attemptsTotal(attempts)}
	for i, a := range attempts {
		phases, gaps, warnings := assignAttempt(a.events, opts.Markers, opts.Pack, pod, a.from, opts.NotApplicable)
		totals := attemptTotals(a.events, phases, created, hasCreated)
		if i == chosen {
			report.Attempt = a.number
			report.Phases, report.Gaps, report.Totals, report.Warnings = phases, gaps, totals, warnings
		}
		if opts.AllAttempts {
			report.Attempts = append(report.Attempts, model.Attempt{Attempt: a.number, Phases: phases, Gaps: gaps, Totals: totals})
		}
	}
	return report, nil
}

type attempt struct {
	number int
	from   time.Time // where the attempt begins; zero for the first
	events []model.Event
}

// splitAttempts cuts the sorted events into boot attempts, one per container start (DESIGN.md
// section 6.1 step 7). Attempt N begins where the previous container ended, a
// ContainerTerminated carrying N-1's restart count, so a re-pull before a restart belongs to
// the restart (DK-PHASE-BUG-20261024-attempt-boundary-repull); without that end it begins at
// its own start. An event tagged with an attempt (AttrRestartCount: starts, ends, a previous
// container's log) goes to that attempt. Any other event goes by time, and a boundary owns
// its whole second: Kubernetes times have 1 s resolution, so a re-pull and the end it follows
// often share one. The first attempt also keeps everything before its start. With no
// container start there is one attempt, number 0, holding every event.
func splitAttempts(sorted []model.Event) []attempt {
	var out []attempt
	byNumber := map[int]int{}
	for _, e := range sorted {
		if e.Source != model.SourceContainerStatus || e.Kind != model.KindContainerStarted {
			continue
		}
		// A start without a readable restart count is numbered by its position.
		n, err := strconv.Atoi(e.Attrs[model.AttrRestartCount])
		if err != nil {
			n = len(out)
		}
		byNumber[n] = len(out)
		out = append(out, attempt{number: n, from: e.TS})
	}
	if len(out) == 0 {
		return []attempt{{number: 0, events: sorted}}
	}
	out[0].from = time.Time{}
	for _, e := range sorted {
		if e.Source != model.SourceContainerStatus || e.Kind != model.KindContainerTerminated {
			continue
		}
		n, err := strconv.Atoi(e.Attrs[model.AttrRestartCount])
		if next, ok := byNumber[n+1]; err == nil && ok && next > 0 && !e.TS.After(out[next].from) {
			out[next].from = e.TS
		}
	}
	for _, e := range sorted {
		i := -1
		if n, err := strconv.Atoi(e.Attrs[model.AttrRestartCount]); err == nil {
			if j, ok := byNumber[n]; ok {
				i = j
			}
		}
		if i < 0 {
			i = 0
			for j := 1; j < len(out); j++ {
				if !e.TS.Before(out[j].from) {
					i = j
				}
			}
		}
		out[i].events = append(out[i].events, e)
	}
	return out
}

// podPhases assigns the pod-level phases once, over every event.
func podPhases(sorted []model.Event, markers []string) map[model.PhaseName]model.Phase {
	out := map[model.PhaseName]model.Phase{}
	var cursor time.Time
	for _, name := range phaseOrder() {
		if !slices.Contains(podLevel, name) {
			continue
		}
		p := assignPhase(name, specs[name], sorted, markers, cursor)
		if p.State != model.StateAbsent && p.State != model.StateSkipped {
			cursor = p.Start
		}
		out[name] = p
	}
	return out
}

// attemptsTotal counts boots from the highest restart count seen, since the pod object
// only shows the latest starts.
func attemptsTotal(attempts []attempt) int {
	total := len(attempts)
	for _, a := range attempts {
		total = max(total, a.number+1)
	}
	return total
}

func attemptNumbers(attempts []attempt) string {
	s := make([]string, len(attempts))
	for i, a := range attempts {
		s[i] = strconv.Itoa(a.number)
	}
	return strings.Join(s, ", ")
}

func assignAttempt(events []model.Event, markers []string, pack string, pod map[model.PhaseName]model.Phase, from time.Time, notApplicable []model.PhaseName) ([]model.Phase, []model.Gap, []model.Warning) {
	var (
		phases   []model.Phase
		warnings []model.Warning
		cursor   time.Time // the start of the last phase found; later phases start at or after it
	)
	for _, name := range phaseOrder() {
		p, ok := pod[name]
		if !ok {
			p = assignPhase(name, specs[name], events, markers, cursor)
		}
		// A phase the engine never has is skipped, not absent: a label only (decision 0008).
		if p.State == model.StateAbsent && slices.Contains(notApplicable, name) {
			p = model.Phase{Name: name, State: model.StateSkipped, Attrs: map[string]string{model.AttrNotApplicable: "true"}}
		}
		if p.State != model.StateAbsent && p.State != model.StateSkipped {
			cursor = p.Start
		}
		if p.State == model.StateInvalid {
			warnings = append(warnings, model.Warning{
				Code: "phase-invalid",
				Text: fmt.Sprintf("%s ends %.1fs before it starts; left out of gaps and totals", name, p.Start.Sub(p.End).Seconds()),
			})
		}
		phases = append(phases, p)
	}
	return phases, gaps(phases, from), append(warnings, unobserved(phases, events, markers, pack)...)
}

// unobserved warns about each expected engine phase still absent once the pod is Ready
// (decision 0005). Before Ready the pod may still be booting, so there is nothing to say.
func unobserved(phases []model.Phase, events []model.Event, markers []string, pack string) []model.Warning {
	if _, ready := first(events, podReady); !ready {
		return nil
	}
	expected := Expected(markers)
	var out []model.Warning
	for _, p := range phases {
		if p.State == model.StateAbsent && specs[p.Name].engine() && slices.Contains(expected, p.Name) {
			out = append(out, model.Warning{
				Code:  model.WarnPhaseUnobserved,
				Phase: p.Name,
				Text:  fmt.Sprintf("%s: no log line matched before the pod was Ready (pack %s); the engine may have skipped it, or this engine version words it differently", p.Name, pack),
			})
		}
	}
	return out
}

// assignPhase applies DESIGN.md section 6.1 steps 3 and 4 to one phase. The end is the
// first end event at or after the start; if there is none but an end event exists before
// the start, the latest such event is used, so a clock bug shows as invalid instead of open.
func assignPhase(name model.PhaseName, s spec, events []model.Event, markers []string, cursor time.Time) model.Phase {
	absent := model.Phase{Name: name, State: model.StateAbsent}
	if !s.producible(markers) {
		return absent
	}
	startIdx := slices.IndexFunc(events, func(e model.Event) bool { return !e.TS.Before(cursor) && startsAt(name, s, e) })
	if startIdx < 0 {
		// The skip marker may be logged long before the phase would run (vLLM says so at
		// config time), so it is looked for in the whole attempt, not after the cursor.
		if skip := (matcher{markerSuffix: s.skipSuffix()}); skip.markerSuffix != "" {
			if e, ok := first(events, skip); ok {
				return model.Phase{Name: name, State: model.StateSkipped, Attrs: map[string]string{"skipped_by": e.Marker},
					Sources: []model.Source{e.Source}, Confidence: below1(e.Confidence)}
			}
		}
		return absent
	}
	start := events[startIdx]
	p := model.Phase{Name: name, Start: start.TS, State: model.StateOpen, Sources: []model.Source{start.Source}, Confidence: below1(start.Confidence)}
	for _, k := range phaseAttrs {
		if v, ok := start.Attrs[k]; ok {
			p.Attrs = setAttr(p.Attrs, k, v)
		}
	}
	if s.start.markerSuffix != "" {
		p.Attrs = takeAttrs(p.Attrs, start.Attrs)
	}

	endIdx := slices.IndexFunc(events, func(e model.Event) bool { return !e.TS.Before(start.TS) && s.end.match(e) })
	if endIdx < 0 {
		for i, e := range events {
			if e.TS.Before(start.TS) && s.end.match(e) {
				endIdx = i
			}
		}
	}
	if endIdx < 0 {
		return p
	}
	end := events[endIdx]
	p.End = end.TS
	if s.end.markerSuffix != "" {
		p.Attrs = takeAttrs(p.Attrs, end.Attrs)
	}
	if end.Confidence < start.Confidence {
		p.Confidence = below1(end.Confidence)
	}
	if !slices.Contains(p.Sources, end.Source) {
		p.Sources = append(p.Sources, end.Source)
	}
	slices.SortFunc(p.Sources, func(a, b model.Source) int {
		return compareEvents(model.Event{Source: a}, model.Event{Source: b})
	})

	d := end.TS.Sub(start.TS)
	if d < -invalidAfter {
		p.State = model.StateInvalid
		return p
	}
	seconds := max(d, 0).Seconds() // within 1 s either way is timestamp resolution
	p.Seconds = &seconds
	p.State = model.StateComplete
	return p
}

// below1 is a confidence as written on a phase: only when below 1.0.
func below1(c float32) *float32 {
	if c >= 1 {
		return nil
	}
	return &c
}

// takeAttrs copies an engine marker's attributes onto a phase, leaving out collectorAttrs.
func takeAttrs(attrs, from map[string]string) map[string]string {
	for k, v := range from {
		if !slices.Contains(collectorAttrs, k) {
			attrs = setAttr(attrs, k, v)
		}
	}
	return attrs
}

func setAttr(attrs map[string]string, k, v string) map[string]string {
	if attrs == nil {
		attrs = map[string]string{}
	}
	attrs[k] = v
	return attrs
}

// gaps reports time over 1 s between consecutive complete phases (DESIGN.md section 6.1
// step 5). Overlapping phases leave no gap. Each gap is measured from the latest end among
// the complete phases so far, not from the previous phase in the fixed order, so time inside
// a longer earlier phase is never a gap after a phase nested in it; that phase is the gap's
// After (DK-PHASE-BUG-20261029-gap-after-nested-phase). On equal ends the later phase in the
// order is kept. from is a later attempt's boundary (zero for the first attempt): time before
// it, after a pod-level phase, was spent by the earlier attempts, so that part is a gap with
// reason "earlier attempts" (decision 0007); the rest is unattributed.
func gaps(phases []model.Phase, from time.Time) []model.Gap {
	var out []model.Gap
	var prev *model.Phase // the complete phase that ends latest so far
	for i := range phases {
		p := &phases[i]
		if p.State != model.StateComplete {
			continue
		}
		if prev != nil {
			start := prev.End
			if from.After(start) {
				end := from
				if p.Start.Before(end) {
					end = p.Start
				}
				if d := end.Sub(start); d > gapOver {
					out = append(out, model.Gap{After: prev.Name, Before: p.Name, Seconds: d.Seconds(), Reason: model.GapEarlierAttempts})
				}
				start = end
			}
			if d := p.Start.Sub(start); d > gapOver {
				out = append(out, model.Gap{After: prev.Name, Before: p.Name, Seconds: d.Seconds()})
			}
		}
		if prev == nil || !p.End.Before(prev.End) {
			prev = p
		}
	}
	return out
}

// attemptTotals measures from pod creation to this attempt's Ready and first token
// (DESIGN.md section 6.1 step 6). Wall-clock span, never a sum of phases. A value whose end
// point is missing, or precedes creation, is omitted.
func attemptTotals(events []model.Event, phases []model.Phase, created model.Event, hasCreated bool) model.Totals {
	var t model.Totals
	if !hasCreated {
		return t
	}
	since := func(ts time.Time) *float64 {
		if ts.Before(created.TS) {
			return nil
		}
		s := ts.Sub(created.TS).Seconds()
		return &s
	}
	if ready, ok := first(events, podReady); ok {
		t.PendingToReadySeconds = since(ready.TS)
	}
	for _, p := range phases {
		if p.Name == model.PhaseFirstToken && p.State == model.StateComplete {
			t.PendingToFirstTokenSeconds = since(p.End)
		}
	}
	return t
}

func first(events []model.Event, m matcher) (model.Event, bool) {
	if i := slices.IndexFunc(events, m.match); i >= 0 {
		return events[i], true
	}
	return model.Event{}, false
}
