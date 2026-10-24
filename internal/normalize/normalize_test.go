package normalize

import (
	"slices"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

var started = time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)

func ev(src model.Source, kind string, ts time.Time) model.Event {
	return model.Event{TS: ts, Source: src, Kind: kind, Confidence: 1}
}

func secs(f float64) time.Duration { return time.Duration(f * float64(time.Second)) }

// DESIGN.md section 9.3 skew cases, with the outcomes approved for option A. "Behind" means
// the first engine log line is stamped before the container started, which cannot happen,
// so engine-log events shift later until that line sits on `started`. "Ahead" cannot be
// told apart from real container-init time with this anchor, so it is left alone.
func TestClockSkew(t *testing.T) {
	cases := []struct {
		name      string
		firstLog  float64 // seconds relative to container started
		wantShift float64
		wantWarn  string
	}{
		{"0 s", 0, 0, ""},
		{"1.5 s behind", -1.5, 1.5, ""},
		{"2 s behind is not above 2 s", -2, 2, ""},
		{"3 s behind", -3, 3, "engine log clock is 3.0s behind kubelet; engine log times shifted 3.0s later"},
		{"60 s behind", -60, 60, "engine log clock is 60.0s behind kubelet; engine log times shifted 60.0s later"},
		{"3 s ahead", 3, 0, ""},
		{"60 s ahead", 60, 0, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			first := started.Add(secs(c.firstLog))
			in := []model.Event{
				ev(model.SourceK8sEvent, "Pulled", started.Add(-5*time.Second)),
				ev(model.SourceContainerStatus, model.KindContainerStarted, started),
				ev(model.SourceEngineLog, "log.line", first),
				ev(model.SourceEngineLog, "log.line", first.Add(10*time.Second)),
			}
			out, warns := Normalize(in)

			shift := secs(c.wantShift)
			want := []model.Event{
				in[0], in[1],
				ev(model.SourceEngineLog, "log.line", first.Add(shift)),
				ev(model.SourceEngineLog, "log.line", first.Add(10*time.Second+shift)),
			}
			slices.SortFunc(want, model.CompareEvents)
			if !slices.EqualFunc(out, want, eventsEqual) {
				t.Errorf("events:\n got %v\nwant %v", out, want)
			}

			switch {
			case c.wantWarn == "" && len(warns) != 0:
				t.Errorf("warnings = %v, want none", warns)
			case c.wantWarn != "" && (len(warns) != 1 || warns[0] != model.Warning{Code: "clock-skew", Text: c.wantWarn}):
				t.Errorf("warnings = %v, want one clock-skew: %q", warns, c.wantWarn)
			}
		})
	}
}

// With no container start (events expired, DESIGN.md section 9.3) or no log lines, there is
// no anchor: nothing shifts and nothing warns.
func TestClockSkewNoAnchor(t *testing.T) {
	logOnly := []model.Event{ev(model.SourceEngineLog, "log.line", started)}
	startOnly := []model.Event{ev(model.SourceContainerStatus, model.KindContainerStarted, started)}
	for name, in := range map[string][]model.Event{"no start": logOnly, "no log": startOnly} {
		out, warns := Normalize(in)
		if !slices.EqualFunc(out, in, eventsEqual) || len(warns) != 0 {
			t.Errorf("%s: got %v %v, want input unchanged and no warnings", name, out, warns)
		}
	}
}

// Logs come from the current container, so after a restart the latest start is the anchor.
// Anchoring to the first start would see no skew here and leave the lines 3 s too early.
func TestClockSkewAnchorsToLatestStart(t *testing.T) {
	restarted := started.Add(time.Minute)
	in := []model.Event{
		ev(model.SourceContainerStatus, model.KindContainerStarted, started),
		ev(model.SourceContainerStatus, model.KindContainerStarted, restarted),
		ev(model.SourceEngineLog, "log.line", restarted.Add(-3*time.Second)),
	}
	out, warns := Normalize(in)
	if got := out[len(out)-1].TS; !got.Equal(restarted) {
		t.Errorf("log line at %v, want shifted to %v", got, restarted)
	}
	if len(warns) != 1 {
		t.Errorf("warnings = %v, want one clock-skew", warns)
	}
}

// Events delivered out of order come back sorted by time, ties broken by source priority.
func TestOutOfOrderAndTieBreak(t *testing.T) {
	in := []model.Event{
		ev(model.SourceEngineLog, "log.line", started.Add(2*time.Second)),
		ev(model.SourceEngineLog, "log.line", started),
		ev(model.SourceK8sEvent, "Started", started),
		ev(model.SourceK8sEvent, "Scheduled", started.Add(-30*time.Second)),
		ev(model.SourceContainerStatus, model.KindContainerStarted, started),
	}
	out, _ := Normalize(in)
	want := []model.Event{in[3], in[2], in[4], in[1], in[0]}
	if !slices.EqualFunc(out, want, eventsEqual) {
		t.Errorf("got  %v\nwant %v", out, want)
	}
}

// The same event delivered twice (overlapping collectors, a repeated file) appears once.
// Events that differ in any field are both kept.
func TestDuplicates(t *testing.T) {
	a := ev(model.SourceK8sEvent, "Pulled", started)
	a.Attrs = map[string]string{"image": "nginx"}
	dup := a
	dup.Attrs = map[string]string{"image": "nginx"}
	other := a
	other.Raw = "a different message at the same second"
	out, _ := Normalize([]model.Event{a, other, dup})
	if len(out) != 2 {
		t.Errorf("got %d events %v, want 2", len(out), out)
	}
}

// Normalize returns a new slice and never changes the caller's events.
func TestInputNotMutated(t *testing.T) {
	in := []model.Event{
		ev(model.SourceEngineLog, "log.line", started.Add(-3*time.Second)),
		ev(model.SourceContainerStatus, model.KindContainerStarted, started),
	}
	saved := slices.Clone(in)
	Normalize(in)
	if !slices.EqualFunc(in, saved, eventsEqual) {
		t.Errorf("input changed: %v, was %v", in, saved)
	}
}

// Sorting and dedupe must use model.CompareEvents, the single encoding of the order. A
// sentinel order (newest first) must show up in Normalize's output; a private copy of the
// real order would ignore it.
func TestUsesSharedOrder(t *testing.T) {
	saved := compareEvents
	t.Cleanup(func() { compareEvents = saved })
	compareEvents = func(a, b model.Event) int { return -model.CompareEvents(a, b) }

	in := []model.Event{
		ev(model.SourceK8sEvent, "Scheduled", started),
		ev(model.SourceK8sEvent, "Pulling", started.Add(time.Second)),
	}
	out, _ := Normalize(in)
	if out[0].Kind != "Pulling" {
		t.Errorf("got %v, want the sentinel newest-first order", out)
	}
}

func eventsEqual(a, b model.Event) bool { return model.CompareEvents(a, b) == 0 }
