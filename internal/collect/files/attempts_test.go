package files

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// The previous container's end, from lastState.terminated.finishedAt, is a ContainerTerminated
// event carrying the restart count of the attempt it ends, so phase can split attempts there
// (DK-PHASE-BUG-20261024-attempt-boundary-repull). With no previous container there is none.
func TestContainerTerminated(t *testing.T) {
	last := `{"terminated": {"startedAt": "2026-10-24T10:00:35Z", "finishedAt": "2026-10-24T10:00:38Z", "exitCode": 1}}`
	events, _ := read(t, pod("True", 2, runningMain, last), `{"items": []}`, "")
	got := find(events, model.SourceContainerStatus, model.KindContainerTerminated)
	if len(got) != 1 || !got[0].TS.Equal(ts("2026-10-24T10:00:38Z")) || got[0].Attrs[model.AttrRestartCount] != "1" {
		t.Fatalf("ContainerTerminated = %+v, want one at 10:00:38 with restart_count 1", got)
	}

	events, _ = read(t, pod("True", 0, runningMain, `{}`), `{"items": []}`, "")
	if got := find(events, model.SourceContainerStatus, model.KindContainerTerminated); len(got) != 0 {
		t.Errorf("ContainerTerminated without a previous container = %+v, want none", got)
	}
}

// The kubelet merges repeated events into one with a count. For the image-pull events, a
// count above 1 with a later lastTimestamp is two events, at the first and the last time, so
// the latest attempt gets its own (re-)pull. Other repeated events keep one event, and so
// does a count whose times are equal.
func TestAggregatedPullEvents(t *testing.T) {
	obj := `"involvedObject": {"kind": "Pod", "name": "p", "namespace": "ns", "uid": "u1", "fieldPath": "spec.containers{main}"}`
	eventsJSON := `{"items": [
	  {"reason": "Pulling", "message": "Pulling image", ` + obj + `, "count": 2,
	   "firstTimestamp": "2026-10-24T10:00:03Z", "lastTimestamp": "2026-10-24T10:00:39Z"},
	  {"reason": "Pulled", "message": "Container image \"x\" already present on machine and can be accessed by the pod", ` + obj + `, "count": 2,
	   "firstTimestamp": "2026-10-24T10:00:04Z", "lastTimestamp": "2026-10-24T10:00:40Z"},
	  {"reason": "Unhealthy", "message": "Readiness probe failed", ` + obj + `, "count": 25,
	   "firstTimestamp": "2026-10-24T10:00:41Z", "lastTimestamp": "2026-10-24T10:00:55Z"},
	  {"reason": "Created", "message": "Container created", ` + obj + `, "count": 2,
	   "firstTimestamp": "2026-10-24T10:00:05Z", "lastTimestamp": "2026-10-24T10:00:05Z"}
	]}`
	events, _ := read(t, pod("True", 1, runningMain, `{}`), eventsJSON, "")

	for kind, want := range map[string][]string{
		model.KindPulling: {"2026-10-24T10:00:03Z", "2026-10-24T10:00:39Z"},
		model.KindPulled:  {"2026-10-24T10:00:04Z", "2026-10-24T10:00:40Z"},
		"Unhealthy":       {"2026-10-24T10:00:41Z"},
		"Created":         {"2026-10-24T10:00:05Z"},
	} {
		got := find(events, model.SourceK8sEvent, kind)
		if len(got) != len(want) {
			t.Errorf("%s: %d events, want %d", kind, len(got), len(want))
			continue
		}
		for i, w := range want {
			if !got[i].TS.Equal(ts(w)) {
				t.Errorf("%s event %d at %v, want %s", kind, i, got[i].TS, w)
			}
		}
	}
	for _, e := range find(events, model.SourceK8sEvent, model.KindPulled) {
		if e.Attrs[model.AttrCached] != "true" {
			t.Errorf("Pulled at %v not tagged cached", e.TS)
		}
	}
}

// The previous container's log (kubectl logs --previous) is an optional input. Its lines are
// tagged with the attempt they came from, timed or not; the current log's lines stay untagged,
// meaning the current container.
func TestPreviousLog(t *testing.T) {
	last := `{"terminated": {"startedAt": "2026-10-24T10:00:35Z", "finishedAt": "2026-10-24T10:00:38Z"}}`
	events, _, err := Read(strings.NewReader(pod("True", 1, runningMain, last)), strings.NewReader(`{"items": []}`),
		strings.NewReader("2026-10-24T10:00:41Z current\n"),
		strings.NewReader("2026-10-24T10:00:36Z previous\nprevious untimed\n"))
	if err != nil {
		t.Fatal(err)
	}
	lines := find(events, model.SourceEngineLog, model.KindLogLine)
	if len(lines) != 3 {
		t.Fatalf("log lines = %+v, want 3", lines)
	}
	byText := map[string]model.Event{}
	for _, l := range lines {
		byText[l.Raw] = l
	}
	if a := byText["current"].Attrs; a[model.AttrRestartCount] != "" {
		t.Errorf("current line attrs = %v, want no restart_count", a)
	}
	if a := byText["previous"].Attrs; a[model.AttrRestartCount] != "0" || !byText["previous"].TS.Equal(ts("2026-10-24T10:00:36Z")) {
		t.Errorf("previous line = %+v, want restart_count 0 at 10:00:36", byText["previous"])
	}
	if a := byText["previous untimed"].Attrs; a[model.AttrRestartCount] != "0" || a[model.AttrUntimed] != "true" {
		t.Errorf("previous untimed line attrs = %v, want restart_count 0 and untimed", a)
	}
}

// A previous log needs a container that restarted, and there is only one previous log.
func TestPreviousLogErrors(t *testing.T) {
	r := strings.NewReader
	if _, _, err := Read(r(pod("True", 0, runningMain, `{}`)), r(`{"items": []}`), r(""), r("x\n")); err == nil {
		t.Errorf("previous log for a container that never restarted: want an error")
	}
	last := `{"terminated": {"startedAt": "2026-10-24T10:00:35Z", "finishedAt": "2026-10-24T10:00:38Z"}}`
	if _, _, err := Read(r(pod("True", 1, runningMain, last)), r(`{"items": []}`), r(""), r(""), r("")); err == nil {
		t.Errorf("two previous logs: want an error")
	}
}
