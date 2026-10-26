package enginelog

import (
	"maps"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/collect/files"
	"github.com/AmitwalaH/doriki/internal/model"
)

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

func f32(f float32) *float32 { return &f }

// Lines below are copied from packs/vllm/testdata/0.32/container.log (the runtime prefix is
// what files strips into TS).
const (
	versionLine  = "(EngineCore pid=139) INFO 10-25 09:29:06 [core.py:133] Initializing a V1 LLM engine (v0.32.0) with config: model='facebook/opt-125m', speculative_config=None"
	downloadLine = "(Worker pid=171) INFO 10-25 09:31:09 [weight_utils.py:532] Time spent downloading weights for facebook/opt-125m: 108.186023 seconds"
	loadLine     = "(Worker pid=171) INFO 10-25 09:31:10 [default_loader.py:501] Loading weights took 0.49 seconds"
	readyLine    = "INFO:     Application startup complete."
)

var testPack = model.Pack{
	Name: "vllm/0.32",
	Timestamp: &model.PackTimestamp{
		Regex:  `(?:^|\) )(?:DEBUG|INFO|WARNING|ERROR|CRITICAL) (?P<ts>\d{2}-\d{2} \d{2}:\d{2}:\d{2}) \[`,
		Layout: "01-02 15:04:05",
	},
	Markers: []model.PackMarker{
		{ID: "vllm.weights.fetch.end", Regex: `Time spent downloading weights for (?P<model>\S+): (?P<seconds>[0-9.]+) seconds`, StartFromDuration: "seconds"},
		{ID: "vllm.weights.load.end", Regex: `Loading weights took (?P<seconds>[0-9.]+) seconds`},
		{ID: "vllm.server.listening", Regex: `Application startup complete\.`},
		{ID: "vllm.kv.end", Regex: `GPU KV cache size`, Confidence: f32(0.8)},
		{ID: "x.any.download", Regex: `downloading weights`},
	},
	Facts: []model.PackFact{{Key: "engine.version", Regex: `LLM engine \(v(?P<v>[0-9.]+)\)`}},
}

func started(at string) model.Event {
	return model.Event{TS: ts(at), Source: model.SourceContainerStatus, Kind: model.KindContainerStarted, Confidence: 1}
}

func line(at, raw string) model.Event {
	e := model.Event{Source: model.SourceEngineLog, Kind: model.KindLogLine, Raw: raw, Confidence: 1}
	if at == "" {
		e.Attrs = map[string]string{model.AttrUntimed: "true"}
	} else {
		e.TS = ts(at)
	}
	return e
}

func markers(events []model.Event, id string) []model.Event {
	var out []model.Event
	for _, e := range events {
		if e.Kind == model.KindLogMarker && e.Marker == id {
			out = append(out, e)
		}
	}
	return out
}

func mustApply(t *testing.T, p model.Pack, in []model.Event) ([]model.Event, []model.Warning) {
	t.Helper()
	out, warnings, err := Apply(p, in)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	return out, warnings
}

// A matching line becomes one marker event per matching marker, with its named groups as
// attributes and the marker's confidence (1.0 when unset); a line no marker matches stays a
// log line. Events from other sources pass through unchanged.
func TestMarkers(t *testing.T) {
	in := []model.Event{
		started("2026-10-25T09:28:09Z"),
		line("2026-10-25T09:31:10.103559029Z", loadLine),
		line("2026-10-25T09:31:16.038564038Z", readyLine),
		line("2026-10-25T09:31:16.5Z", "unrelated"),
		line("2026-10-25T09:31:09.613199435Z", downloadLine),
	}
	out, warnings := mustApply(t, testPack, in)
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	load := markers(out, "vllm.weights.load.end")
	if len(load) != 1 || load[0].Attrs["seconds"] != "0.49" || load[0].Confidence != 1 ||
		!load[0].TS.Equal(ts("2026-10-25T09:31:10.103559029Z")) || load[0].Raw != loadLine || load[0].Source != model.SourceEngineLog {
		t.Errorf("load marker = %+v", load)
	}
	if n := len(markers(out, "vllm.server.listening")); n != 1 {
		t.Errorf("server.listening markers = %d, want 1", n)
	}
	// The download line matches two markers: two events, and no log line left for it.
	if len(markers(out, "x.any.download")) != 1 || len(markers(out, "vllm.weights.fetch.end")) != 1 {
		t.Errorf("download line: want one event per matching marker")
	}
	var lines []string
	for _, e := range out {
		if e.Kind == model.KindLogLine {
			lines = append(lines, e.Raw)
		}
	}
	if len(lines) != 1 || lines[0] != "unrelated" {
		t.Errorf("log lines left = %q, want only the unmatched one", lines)
	}
	if !reflect.DeepEqual(out[0], in[0]) {
		t.Errorf("container start changed: %+v", out[0])
	}
}

// start_from_duration: the end marker's line also yields the start marker, earlier by the
// logged seconds, at confidence 0.9 with the start_derived attribute (owner decision on the
// M1b spike). The end marker itself is unchanged.
func TestStartFromDuration(t *testing.T) {
	out, _ := mustApply(t, testPack, []model.Event{line("2026-10-25T09:31:09.613199435Z", downloadLine)})
	end := markers(out, "vllm.weights.fetch.end")
	start := markers(out, "vllm.weights.fetch.start")
	if len(end) != 1 || len(start) != 1 {
		t.Fatalf("end %v start %v, want one each", end, start)
	}
	if !end[0].TS.Equal(ts("2026-10-25T09:31:09.613199435Z")) || end[0].Confidence != 1 || end[0].Attrs[model.AttrStartDerived] != "" {
		t.Errorf("end = %+v", end[0])
	}
	if want := ts("2026-10-25T09:31:09.613199435Z").Add(-108186023 * time.Microsecond); !start[0].TS.Equal(want) {
		t.Errorf("start TS = %v, want %v", start[0].TS, want)
	}
	if start[0].Confidence != model.DerivedStartConfidence || start[0].Attrs["model"] != "facebook/opt-125m" ||
		start[0].Attrs[model.AttrStartDerived] != "end minus 108.186023 s reported by the engine" || start[0].Raw != downloadLine {
		t.Errorf("start = %+v", start[0])
	}

	// A marker already below 0.9 keeps its lower confidence on the derived start.
	p := testPack
	p.Markers = []model.PackMarker{{ID: "x.weights.fetch.end", Regex: `took (?P<s>[0-9.]+) seconds`, StartFromDuration: "s", Confidence: f32(0.8)}}
	out, _ = mustApply(t, p, []model.Event{line("2026-10-25T09:31:10Z", loadLine)})
	if s := markers(out, "x.weights.fetch.start"); len(s) != 1 || s[0].Confidence != 0.8 {
		t.Errorf("derived start from a 0.8 marker = %+v, want confidence 0.8", s)
	}
}

// With runtime timestamps, the runtime time is the event time and the engine's in-line time
// is only compared with it: a difference over 2 s on any line is a log-inline-skew warning
// (closes DK-PACK-ENH-20261024-per-line-skew). The fixture's own lines differ by under 1 s.
func TestInlineSkew(t *testing.T) {
	in := []model.Event{
		started("2026-10-25T09:28:09Z"),
		line("2026-10-25T09:31:09.613199435Z", downloadLine),
		line("2026-10-25T09:31:10.103559029Z", loadLine),
	}
	out, warnings := mustApply(t, testPack, in)
	if len(warnings) != 0 {
		t.Errorf("fixture lines: warnings = %v, want none", warnings)
	}
	if e := markers(out, "vllm.weights.load.end"); len(e) != 1 || !e[0].TS.Equal(ts("2026-10-25T09:31:10.103559029Z")) {
		t.Errorf("load markers = %v, want one at the runtime time", e)
	}

	// The engine clock 60.2 s ahead on one line (the ticket's case): in-line 09:31:10 against
	// runtime 09:30:09.8. Warned, and the runtime time is still the event time.
	in = append(in, line("2026-10-25T09:30:09.8Z", loadLine))
	out, warnings = mustApply(t, testPack, in)
	want := model.Warning{Code: "log-inline-skew", Text: "the engine's own log clock differs from the runtime's by up to 60.2s; timing uses the runtime timestamps"}
	if len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings = %v, want %v", warnings, want)
	}
	got := map[time.Time]bool{}
	for _, e := range markers(out, "vllm.weights.load.end") {
		got[e.TS] = true
	}
	if len(got) != 2 || !got[ts("2026-10-25T09:31:10.103559029Z")] || !got[ts("2026-10-25T09:30:09.8Z")] {
		t.Errorf("load marker times = %v, want both runtime times", got)
	}
}

// Without runtime timestamps, the in-line time is the event time, read as UTC with the year
// of the container start (the nearest year, so a December start dates a January line in the
// next year), and a log-inline-time warning says so. Lines with no time at all are dropped
// with the log-untimed warning, as the collector used to.
func TestInlineOnly(t *testing.T) {
	in := []model.Event{
		started("2026-10-25T09:28:09Z"),
		line("", loadLine),
		line("", "no time here"),
		line("", readyLine), // uvicorn writes no in-line time
	}
	out, warnings := mustApply(t, testPack, in)
	load := markers(out, "vllm.weights.load.end")
	if len(load) != 1 || !load[0].TS.Equal(ts("2026-10-25T09:31:10Z")) || load[0].Attrs[model.AttrUntimed] != "" {
		t.Errorf("load = %+v, want 2026-10-25T09:31:10Z from the in-line time, untimed cleared", load)
	}
	if n := len(markers(out, "vllm.server.listening")); n != 0 {
		t.Errorf("a line with no time at all became a marker")
	}
	for _, e := range out {
		if e.Source == model.SourceEngineLog && e.TS.IsZero() {
			t.Errorf("an event with no time left Apply: %+v", e)
		}
	}
	want := []model.Warning{
		{Code: "log-untimed", Text: "2 of 3 log lines have no timestamp and were skipped; capture logs with kubectl logs --timestamps"},
		{Code: "log-inline-time", Text: "1 of 3 log lines have no runtime timestamp; their times come from the engine's own clock (1 s resolution, read as UTC, year from the container start); capture logs with kubectl logs --timestamps"},
	}
	if len(warnings) != 2 || warnings[0] != want[0] || warnings[1] != want[1] {
		t.Errorf("warnings = %v\nwant %v", warnings, want)
	}

	out, _ = mustApply(t, testPack, []model.Event{started("2026-12-31T23:59:50Z"), line("", strings.Replace(loadLine, "10-25 09:31:10", "01-01 00:00:05", 1))})
	if load := markers(out, "vllm.weights.load.end"); len(load) != 1 || !load[0].TS.Equal(ts("2027-01-01T00:00:05Z")) {
		t.Errorf("year rollover: %+v, want 2027-01-01T00:00:05Z", load)
	}
}

// A pack with no timestamp (generic) cannot date untimed lines: they are dropped with the
// collector's former warning, word for word, and timed lines pass through untouched.
func TestNoPackTimestamp(t *testing.T) {
	in := []model.Event{line("2026-10-24T10:00:41.5Z", "first"), line("", "no timestamp here")}
	out, warnings := mustApply(t, model.Pack{Name: "generic"}, in)
	if len(out) != 1 || out[0].Raw != "first" || out[0].Kind != model.KindLogLine || out[0].Attrs != nil {
		t.Errorf("out = %+v, want only the timed line, unchanged", out)
	}
	want := model.Warning{Code: "log-untimed", Text: "1 of 2 log lines have no timestamp and were skipped; capture logs with kubectl logs --timestamps"}
	if len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings = %v, want %v", warnings, want)
	}
}

// A fact is read once, from the first matching line, as a manifest event.
func TestFacts(t *testing.T) {
	out, _ := mustApply(t, testPack, []model.Event{
		line("2026-10-25T09:29:06.893981150Z", versionLine),
		line("2026-10-25T09:29:07Z", strings.Replace(versionLine, "0.32.0", "9.9.9", 1)),
	})
	var facts []model.Event
	for _, e := range out {
		if e.Kind == model.KindFact {
			facts = append(facts, e)
		}
	}
	want := map[string]string{model.AttrFactKey: "engine.version", model.AttrFactValue: "0.32.0"}
	if len(facts) != 1 || facts[0].Source != model.SourceManifest || !maps.Equal(facts[0].Attrs, want) ||
		!facts[0].TS.Equal(ts("2026-10-25T09:29:06.893981150Z")) {
		t.Errorf("facts = %+v, want one engine.version 0.32.0", facts)
	}
}

// A marker or fact past byte 512 of a line is still found: the collector cuts only the stored
// Raw, and Apply matches the whole line (DK-COLLECT-BUG-20261025-match-full-log-line). The log
// goes through the real files collector, so a fix on either side alone fails here.
func TestMatchPastRawCap(t *testing.T) {
	pad := func(prefix, rest string) string { return prefix + strings.Repeat(" ", 600-len(prefix)) + rest }
	loadPrefix, _, _ := strings.Cut(loadLine, "Loading")
	versionPrefix, _, _ := strings.Cut(versionLine, "Initializing")
	long := []string{
		pad(loadPrefix, "Loading weights took 0.49 seconds"),
		pad(versionPrefix, "Initializing a V1 LLM engine (v0.32.0) with config"),
	}
	if i := strings.Index(long[0], "Loading"); i != 600 {
		t.Fatalf("marker text at byte %d, want 600", i)
	}
	pod := `{"metadata": {"name": "p"}, "spec": {"containers": [{"name": "c", "image": "i"}]}}`
	log := "2026-10-25T09:31:10Z " + long[0] + "\n" + "2026-10-25T09:29:06Z " + long[1] + "\n"
	collected, _, err := files.Read(strings.NewReader(pod), strings.NewReader(`{"items": []}`), strings.NewReader(log))
	if err != nil {
		t.Fatal(err)
	}
	out, _ := mustApply(t, testPack, collected)

	got := markers(out, "vllm.weights.load.end")
	if len(got) != 1 || got[0].Attrs["seconds"] != "0.49" {
		t.Fatalf("load markers = %+v, want one with seconds 0.49", got)
	}
	if got[0].Raw != model.TruncateRaw(long[0]) {
		t.Errorf("marker Raw has %d bytes, want the line cut to %d", len(got[0].Raw), model.MaxRawBytes)
	}
	var version string
	for _, e := range out {
		if e.Kind == model.KindFact && e.Attrs[model.AttrFactKey] == "engine.version" {
			version = e.Attrs[model.AttrFactValue]
			if len(e.Raw) > model.MaxRawBytes {
				t.Errorf("fact Raw has %d bytes, want at most %d", len(e.Raw), model.MaxRawBytes)
			}
		}
	}
	if version != "0.32.0" {
		t.Errorf("engine.version = %q, want 0.32.0", version)
	}
}

// A pack regex that does not compile, or a timestamp regex without a ts group, is an error
// naming the pack, not a silent miss.
func TestBadPack(t *testing.T) {
	for _, p := range []model.Pack{
		{Name: "x", Markers: []model.PackMarker{{ID: "x.a", Regex: "("}}},
		{Name: "x", Timestamp: &model.PackTimestamp{Regex: `\d+`, Layout: "15"}},
		{Name: "x", Markers: []model.PackMarker{{ID: "x.a.start", Regex: `(?P<s>\d)`, StartFromDuration: "s"}}},
	} {
		if _, _, err := Apply(p, nil); err == nil || !strings.Contains(err.Error(), "pack x") {
			t.Errorf("pack %+v: err = %v, want one naming the pack", p, err)
		}
	}
}
