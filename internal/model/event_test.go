package model

import (
	"slices"
	"strings"
	"testing"
	"time"
)

var t0 = time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)

// Same timestamp: DESIGN.md section 6.1 step 2 ranks k8s-event > container-status > cri >
// otel-span > engine-log. Sources the design does not rank go last.
func TestCompareEventsSourcePriority(t *testing.T) {
	want := []Source{
		SourceK8sEvent, SourceContainerStatus, SourceCRI, SourceOTelSpan, SourceEngineLog,
		SourceManifest,
	}
	events := make([]Event, len(want))
	for i, s := range want {
		events[len(want)-1-i] = Event{TS: t0, Source: s}
	}
	slices.SortFunc(events, CompareEvents)
	for i, e := range events {
		if e.Source != want[i] {
			t.Errorf("position %d: got %s, want %s", i, e.Source, want[i])
		}
	}
}

// Time comes before source: an engine-log line one nanosecond earlier sorts first.
func TestCompareEventsTimeFirst(t *testing.T) {
	early := Event{TS: t0, Source: SourceEngineLog}
	late := Event{TS: t0.Add(time.Nanosecond), Source: SourceK8sEvent}
	if CompareEvents(early, late) >= 0 {
		t.Errorf("earlier engine-log event does not sort before later k8s event")
	}
}

// The order is total: events that differ in any field never compare equal, so sorting any
// permutation of the same events gives the same result. Only identical events compare 0.
func TestCompareEventsTotal(t *testing.T) {
	base := Event{TS: t0, Source: SourceK8sEvent, Kind: "Pulled", Marker: "m", Raw: "r",
		Attrs: map[string]string{"image": "nginx"}, Confidence: 1}
	variants := map[string]Event{}
	v := base
	v.Kind = "Pulling"
	variants["kind"] = v
	v = base
	v.Marker = "n"
	variants["marker"] = v
	v = base
	v.Raw = "s"
	variants["raw"] = v
	v = base
	v.Confidence = 0.5
	variants["confidence"] = v
	v = base
	v.Attrs = map[string]string{"image": "nginx:1"}
	variants["attr value"] = v
	v = base
	v.Attrs = map[string]string{"image": "nginx", "size": "1"}
	variants["extra attr"] = v
	v = base
	v.Attrs = nil
	variants["no attrs"] = v

	for name, other := range variants {
		ab, ba := CompareEvents(base, other), CompareEvents(other, base)
		if ab == 0 || ab != -ba {
			t.Errorf("%s: Compare(base, other)=%d, Compare(other, base)=%d", name, ab, ba)
		}
	}
	same := base
	same.Attrs = map[string]string{"image": "nginx"}
	same.TS = t0.In(time.FixedZone("x", 3600))
	if c := CompareEvents(base, same); c != 0 {
		t.Errorf("identical events (different map instance and zone) compare %d, want 0", c)
	}
}

// Raw keeps at most 512 bytes and backs off to a character boundary.
func TestTruncateRaw(t *testing.T) {
	cases := map[string]struct{ in, want string }{
		"short":         {"abc", "abc"},
		"exactly 512":   {strings.Repeat("a", 512), strings.Repeat("a", 512)},
		"ascii over":    {strings.Repeat("a", 600), strings.Repeat("a", 512)},
		"split 2-byte":  {strings.Repeat("a", 511) + "é", strings.Repeat("a", 511)},
		"split 4-byte":  {strings.Repeat("a", 510) + "😀", strings.Repeat("a", 510)},
		"rune ends 512": {strings.Repeat("a", 510) + "é" + "z", strings.Repeat("a", 510) + "é"},
	}
	for name, c := range cases {
		if got := TruncateRaw(c.in); got != c.want {
			t.Errorf("%s: got %d bytes, want %d", name, len(got), len(c.want))
		}
	}
}

// Text is the whole line when the collector kept it, and Raw for events that have no Line.
func TestText(t *testing.T) {
	if got := (Event{Raw: "cut", Line: "cut and the rest"}).Text(); got != "cut and the rest" {
		t.Errorf("with Line: %q", got)
	}
	if got := (Event{Raw: "message"}).Text(); got != "message" {
		t.Errorf("without Line: %q", got)
	}
}
