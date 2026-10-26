package enginelog

import (
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

// inlineSkewWarnAbove is the largest difference between a line's runtime timestamp and the
// engine's own that passes without a warning; the same 2 s as clock-skew (DESIGN.md 4.1).
const inlineSkewWarnAbove = 2 * time.Second

type compiledMarker struct {
	model.PackMarker
	re         *regexp.Regexp
	confidence float32
}

type compiledFact struct {
	key string
	re  *regexp.Regexp
}

// compiled is a pack with its regular expressions compiled once.
type compiled struct {
	ts      *regexp.Regexp
	layout  string
	markers []compiledMarker
	facts   []compiledFact
}

func compile(p model.Pack) (compiled, error) {
	var c compiled
	fail := func(what string, err error) (compiled, error) {
		return compiled{}, fmt.Errorf("pack %s: %s: %w", p.Name, what, err)
	}
	if p.Timestamp != nil {
		re, err := regexp.Compile(p.Timestamp.Regex)
		if err != nil {
			return fail("timestamp", err)
		}
		if re.SubexpIndex("ts") < 0 {
			return fail("timestamp", fmt.Errorf("regex has no named group ts"))
		}
		c.ts, c.layout = re, p.Timestamp.Layout
	}
	for _, m := range p.Markers {
		re, err := regexp.Compile(m.Regex)
		if err != nil {
			return fail("marker "+m.ID, err)
		}
		if m.StartFromDuration != "" && (!strings.HasSuffix(m.ID, ".end") || re.SubexpIndex(m.StartFromDuration) < 0) {
			return fail("marker "+m.ID, fmt.Errorf("start_from_duration needs an id ending in .end and a group named %q", m.StartFromDuration))
		}
		conf := float32(1)
		if m.Confidence != nil {
			conf = *m.Confidence
		}
		c.markers = append(c.markers, compiledMarker{m, re, conf})
	}
	for _, f := range p.Facts {
		re, err := regexp.Compile(f.Regex)
		if err != nil {
			return fail("fact "+f.Key, err)
		}
		c.facts = append(c.facts, compiledFact{f.Key, re})
	}
	return c, nil
}

// Apply runs a parser pack over the engine log lines among events (DESIGN.md section 5.2) and
// returns the events with every log line dated, matched and possibly replaced:
//
//   - A line with a runtime timestamp keeps it as its time. The engine's own in-line time,
//     when the pack can read it, is only compared with it; a difference over 2 s on any line
//     is a log-inline-skew warning.
//   - An untimed line takes its in-line time, read as UTC in the year nearest the container
//     start, with a log-inline-time warning; with no in-line time it is dropped, with the
//     log-untimed warning.
//   - A line matching markers becomes one log.marker event per matching marker, its named
//     groups as attributes; a start_from_duration marker also yields the derived start.
//   - Each fact becomes one manifest event, from its first matching line.
//
// Events from other sources pass through unchanged.
func Apply(pack model.Pack, events []model.Event) ([]model.Event, []model.Warning, error) {
	c, err := compile(pack)
	if err != nil {
		return nil, nil, err
	}
	anchor := containerStart(events)

	var (
		out                   []model.Event
		total, untimed, dated int
		maxSkew               time.Duration
		factDone              = make([]bool, len(c.facts))
	)
	for _, e := range events {
		if e.Source != model.SourceEngineLog || e.Kind != model.KindLogLine {
			out = append(out, e)
			continue
		}
		total++
		text := fullLine(e)
		if e.Attrs[model.AttrUntimed] == "true" {
			inline, ok := c.inlineTime(text, anchor)
			if !ok {
				untimed++
				continue
			}
			dated++
			e.TS, e.Attrs = inline, withoutUntimed(e.Attrs)
		} else if inline, ok := c.inlineTime(text, e.TS); ok {
			maxSkew = max(maxSkew, absDuration(e.TS.Sub(inline)))
		}

		for i, f := range c.facts {
			if factDone[i] {
				continue
			}
			if v := group(f.re, text, "v"); v != "" {
				factDone[i] = true
				out = append(out, model.Event{TS: e.TS, Source: model.SourceManifest, Kind: model.KindFact, Raw: e.Raw, Confidence: 1,
					Attrs: map[string]string{model.AttrFactKey: f.key, model.AttrFactValue: v}})
			}
		}

		matched, err := c.match(e)
		if err != nil {
			return nil, nil, fmt.Errorf("pack %s: %w", pack.Name, err)
		}
		if len(matched) == 0 {
			out = append(out, e)
		}
		out = append(out, matched...)
	}

	var warnings []model.Warning
	if untimed > 0 {
		warnings = append(warnings, model.Warning{
			Code: "log-untimed",
			Text: fmt.Sprintf("%d of %d log lines have no timestamp and were skipped; capture logs with kubectl logs --timestamps", untimed, total),
		})
	}
	if dated > 0 {
		warnings = append(warnings, model.Warning{
			Code: "log-inline-time",
			Text: fmt.Sprintf("%d of %d log lines have no runtime timestamp; their times come from the engine's own clock (1 s resolution, read as UTC, year from the container start); capture logs with kubectl logs --timestamps", dated, total),
		})
	}
	if maxSkew > inlineSkewWarnAbove {
		warnings = append(warnings, model.Warning{
			Code: "log-inline-skew",
			Text: fmt.Sprintf("the engine's own log clock differs from the runtime's by up to %.1fs; timing uses the runtime timestamps", maxSkew.Seconds()),
		})
	}
	return out, warnings, nil
}

// match returns the marker events for a dated log line: one per matching marker, plus the
// derived start of a start_from_duration marker.
func (c compiled) match(e model.Event) ([]model.Event, error) {
	var out []model.Event
	text := fullLine(e)
	for _, m := range c.markers {
		sub := m.re.FindStringSubmatch(text)
		if sub == nil {
			continue
		}
		attrs := map[string]string{}
		for i, name := range m.re.SubexpNames() {
			if name != "" && sub[i] != "" {
				attrs[name] = sub[i]
			}
		}
		marker := model.Event{TS: e.TS, Source: model.SourceEngineLog, Kind: model.KindLogMarker, Marker: m.ID, Raw: e.Raw, Confidence: m.confidence}
		if len(attrs) > 0 {
			marker.Attrs = attrs
		}
		out = append(out, marker)

		if m.StartFromDuration == "" {
			continue
		}
		logged := attrs[m.StartFromDuration]
		seconds, err := strconv.ParseFloat(logged, 64)
		if err != nil || seconds < 0 || math.IsInf(seconds, 0) {
			return nil, fmt.Errorf("marker %s: duration %q is not a number of seconds", m.ID, logged)
		}
		start := marker
		start.Marker = m.DerivedStartID()
		start.TS = e.TS.Add(-time.Duration(seconds * float64(time.Second)))
		start.Confidence = min(m.confidence, model.DerivedStartConfidence)
		start.Attrs = map[string]string{model.AttrStartDerived: "end minus " + logged + " s reported by the engine"}
		for k, v := range attrs {
			start.Attrs[k] = v
		}
		out = append(out, start)
	}
	return out, nil
}

// inlineTime reads the engine's own timestamp from a line. The layout has no year and no
// zone: it is read as UTC, in whichever of near's year and its neighbours puts it closest to
// near, so a line logged just after New Year is not dated a year early.
func (c compiled) inlineTime(raw string, near time.Time) (time.Time, bool) {
	if c.ts == nil || near.IsZero() {
		return time.Time{}, false
	}
	s := group(c.ts, raw, "ts")
	if s == "" {
		return time.Time{}, false
	}
	t, err := time.ParseInLocation(c.layout, s, time.UTC)
	if err != nil {
		return time.Time{}, false
	}
	best := time.Time{}
	for _, y := range []int{near.Year() - 1, near.Year(), near.Year() + 1} {
		cand := t.AddDate(y-t.Year(), 0, 0)
		if best.IsZero() || absDuration(cand.Sub(near)) < absDuration(best.Sub(near)) {
			best = cand
		}
	}
	return best, true
}

// containerStart is the latest container start: the logs come from the current container.
func containerStart(events []model.Event) time.Time {
	var start time.Time
	for _, e := range events {
		if e.Source == model.SourceContainerStatus && e.Kind == model.KindContainerStarted && e.TS.After(start) {
			start = e.TS
		}
	}
	return start
}

// fullLine is the text packs match: the whole line when the collector kept it, else Raw. Raw
// is cut to 512 bytes for storage, so matching it would miss anything past the cut.
func fullLine(e model.Event) string {
	if e.Line != "" {
		return e.Line
	}
	return e.Raw
}

func group(re *regexp.Regexp, s, name string) string {
	sub := re.FindStringSubmatch(s)
	if i := re.SubexpIndex(name); sub != nil && i >= 0 {
		return sub[i]
	}
	return ""
}

func withoutUntimed(attrs map[string]string) map[string]string {
	if len(attrs) == 1 {
		return nil
	}
	out := map[string]string{}
	for k, v := range attrs {
		if k != model.AttrUntimed {
			out[k] = v
		}
	}
	return out
}

func absDuration(d time.Duration) time.Duration {
	if d < 0 {
		return -d
	}
	return d
}
