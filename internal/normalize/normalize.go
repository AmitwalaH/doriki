package normalize

import (
	"fmt"
	"slices"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

// skewWarnAbove is the engine-log clock correction above which the report warns
// (DESIGN.md section 4.1).
const skewWarnAbove = 2 * time.Second

// compareEvents is a variable so tests can swap in a sentinel order and prove Normalize uses
// the shared one.
var compareEvents = model.CompareEvents

// Normalize corrects engine-log clock skew, sorts events into the single model order and
// drops exact duplicates. It returns a new slice and leaves the input unchanged.
func Normalize(in []model.Event) ([]model.Event, []model.Warning) {
	events := slices.Clone(in)
	var warnings []model.Warning

	if skew := engineLogSkew(events); skew > 0 {
		for i := range events {
			if events[i].Source == model.SourceEngineLog {
				events[i].TS = events[i].TS.Add(skew)
			}
		}
		if skew > skewWarnAbove {
			s := skew.Seconds()
			warnings = append(warnings, model.Warning{
				Code: "clock-skew",
				Text: fmt.Sprintf("engine log clock is %.1fs behind kubelet; engine log times shifted %.1fs later", s, s),
			})
		}
	}

	slices.SortFunc(events, compareEvents)
	events = slices.CompactFunc(events, func(a, b model.Event) bool { return compareEvents(a, b) == 0 })
	return events, warnings
}

// engineLogSkew returns how far an engine log line is stamped before its own container's start,
// at most, or 0 if none is earlier or either is missing. A line before its container started
// is impossible, so that difference is clock skew. A line after the start is left alone: that
// time is real container-init and a log clock running ahead cannot be told apart from it with
// this anchor. A line from a previous container carries that attempt (AttrRestartCount) and is
// measured against that attempt's start; an untagged line comes from the current container,
// the latest start. All lines come from one node's clock, so the largest skew moves them all.
func engineLogSkew(events []model.Event) time.Duration {
	starts := map[string]time.Time{} // attempt -> start
	var latest time.Time
	for _, e := range events {
		if e.Source == model.SourceContainerStatus && e.Kind == model.KindContainerStarted {
			if a := e.Attrs[model.AttrRestartCount]; a != "" {
				starts[a] = e.TS
			}
			if e.TS.After(latest) {
				latest = e.TS
			}
		}
	}
	firstLog := map[string]time.Time{}
	for _, e := range events {
		if e.Source != model.SourceEngineLog {
			continue
		}
		a := e.Attrs[model.AttrRestartCount]
		if t, ok := firstLog[a]; !ok || e.TS.Before(t) {
			firstLog[a] = e.TS
		}
	}
	var skew time.Duration
	for a, first := range firstLog {
		start := latest
		if a != "" {
			start = starts[a]
		}
		if !start.IsZero() && first.Before(start) {
			skew = max(skew, start.Sub(first))
		}
	}
	return skew
}
