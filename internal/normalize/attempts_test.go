package normalize

import (
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

func attemptEvent(src model.Source, kind string, ts time.Time, restartCount string) model.Event {
	e := ev(src, kind, ts)
	e.Attrs = map[string]string{model.AttrRestartCount: restartCount}
	return e
}

// A previous container's log lines are measured against that container's start, not the
// latest one: they are earlier than the latest start because they come from an earlier
// container, not because the clock is behind.
func TestPreviousLogNotSkew(t *testing.T) {
	restarted := started.Add(10 * time.Second)
	in := []model.Event{
		attemptEvent(model.SourceContainerStatus, model.KindContainerStarted, started, "0"),
		attemptEvent(model.SourceEngineLog, "log.line", started.Add(time.Second), "0"),
		attemptEvent(model.SourceContainerStatus, model.KindContainerStarted, restarted, "1"),
		ev(model.SourceEngineLog, "log.line", restarted.Add(time.Second)),
	}
	out, warns := Normalize(in)
	if len(warns) != 0 {
		t.Errorf("warnings = %v, want none", warns)
	}
	for _, e := range out {
		if e.Source == model.SourceEngineLog && !e.TS.Equal(started.Add(time.Second)) && !e.TS.Equal(restarted.Add(time.Second)) {
			t.Errorf("log line moved to %v", e.TS)
		}
	}
}

// One node, one log clock: a line before its own container's start, in any attempt, is skew,
// and every engine log line shifts by the largest such difference.
func TestSkewMeasuredPerAttempt(t *testing.T) {
	restarted := started.Add(10 * time.Second)
	in := []model.Event{
		attemptEvent(model.SourceContainerStatus, model.KindContainerStarted, started, "0"),
		attemptEvent(model.SourceEngineLog, "log.line", started.Add(-3*time.Second), "0"),
		attemptEvent(model.SourceContainerStatus, model.KindContainerStarted, restarted, "1"),
		ev(model.SourceEngineLog, "log.line", restarted.Add(time.Second)),
	}
	out, warns := Normalize(in)
	if len(warns) != 1 {
		t.Fatalf("warnings = %v, want one clock-skew", warns)
	}
	var times []time.Time
	for _, e := range out {
		if e.Source == model.SourceEngineLog {
			times = append(times, e.TS)
		}
	}
	if len(times) != 2 || !times[0].Equal(started) || !times[1].Equal(restarted.Add(4*time.Second)) {
		t.Errorf("log times = %v, want both shifted 3 s", times)
	}
}
