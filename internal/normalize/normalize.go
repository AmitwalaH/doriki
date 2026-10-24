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

// engineLogSkew returns how far the first engine log line is stamped before the latest
// container start, or 0 if it is not earlier or either is missing. A line before its
// container started is impossible, so that difference is clock skew. A line after the start
// is left alone: that time is real container-init and a log clock running ahead cannot be
// told apart from it with this anchor. The latest start is used because logs come from the
// current container; revisit when previous-container logs are collected.
func engineLogSkew(events []model.Event) time.Duration {
	var start, firstLog time.Time
	for _, e := range events {
		switch {
		case e.Source == model.SourceContainerStatus && e.Kind == model.KindContainerStarted:
			if e.TS.After(start) {
				start = e.TS
			}
		case e.Source == model.SourceEngineLog:
			if firstLog.IsZero() || e.TS.Before(firstLog) {
				firstLog = e.TS
			}
		}
	}
	if start.IsZero() || firstLog.IsZero() || !firstLog.Before(start) {
		return 0
	}
	return start.Sub(firstLog)
}
