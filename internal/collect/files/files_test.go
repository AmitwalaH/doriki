package files

import (
	"fmt"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"github.com/AmitwalaH/doriki/internal/model"
)

const fixture = "../../../packs/generic/testdata/kind-nginx/"

func ts(s string) time.Time {
	t, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		panic(err)
	}
	return t
}

type want struct {
	source model.Source
	kind   string
	at     string
}

func countKinds(events []model.Event) map[string]int {
	m := map[string]int{}
	for _, e := range events {
		m[string(e.Source)+"/"+e.Kind]++
	}
	return m
}

func find(events []model.Event, src model.Source, kind string) []model.Event {
	var out []model.Event
	for _, e := range events {
		if e.Source == src && e.Kind == kind {
			out = append(out, e)
		}
	}
	return out
}

// The real kind capture. Expected times were read from the files independently (a Python
// json read of pod.json and events.json, and the first line of container.log), not from
// this collector's output.
func TestLoadKindNginxFixture(t *testing.T) {
	events, warnings, err := Load(fixture+"pod.json", fixture+"events.json", fixture+"container.log")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if len(warnings) != 0 {
		t.Errorf("warnings = %v, want none", warnings)
	}
	for _, w := range []want{
		{model.SourceContainerStatus, model.KindPodCreated, "2026-10-24T14:57:53Z"},
		{model.SourceK8sEvent, model.KindScheduled, "2026-10-24T14:57:53Z"},
		{model.SourceK8sEvent, model.KindPulling, "2026-10-24T14:57:53Z"},
		{model.SourceK8sEvent, model.KindPulled, "2026-10-24T14:58:13Z"},
		{model.SourceContainerStatus, model.KindContainerStarted, "2026-10-24T14:58:13Z"},
		{model.SourceContainerStatus, model.KindPodReady, "2026-10-24T14:58:14Z"},
	} {
		got := find(events, w.source, w.kind)
		if len(got) != 1 || !got[0].TS.Equal(ts(w.at)) {
			t.Errorf("%s/%s: got %v, want one at %s", w.source, w.kind, got, w.at)
		}
	}

	counts := countKinds(events)
	if n := counts["engine-log/"+model.KindLogLine]; n != 32 {
		t.Errorf("log lines = %d, want 32 (wc -l container.log)", n)
	}
	// Six Event objects in events.json; Created, Started and Unhealthy are kept as they are.
	if n := len(find(events, model.SourceK8sEvent, "Unhealthy")); n != 1 {
		t.Errorf("Unhealthy events = %d, want 1", n)
	}
	// The Scheduled event exists, so no fallback from the PodScheduled condition.
	if n := len(find(events, model.SourceContainerStatus, model.KindScheduled)); n != 0 {
		t.Errorf("fallback Scheduled events = %d, want 0", n)
	}

	logs := find(events, model.SourceEngineLog, model.KindLogLine)
	first := logs[0]
	for _, e := range logs {
		if e.TS.Before(first.TS) {
			first = e
		}
	}
	if !first.TS.Equal(ts("2026-10-24T14:58:13.306219429Z")) ||
		first.Raw != "/docker-entrypoint.sh: /docker-entrypoint.d/ is not empty, will attempt to perform configuration" {
		t.Errorf("first log line = %v %q", first.TS, first.Raw)
	}
	started := find(events, model.SourceContainerStatus, model.KindContainerStarted)[0]
	if started.Attrs[model.AttrRestartCount] != "0" {
		t.Errorf("restart_count = %q, want 0", started.Attrs[model.AttrRestartCount])
	}
	for _, e := range events {
		if e.Confidence != 1 || e.TS.Location() != time.UTC {
			t.Errorf("event %s/%s: confidence %v, zone %v; want 1 and UTC", e.Source, e.Kind, e.Confidence, e.TS.Location())
		}
	}
}

const podTmpl = `{
  "apiVersion": "v1", "kind": "Pod",
  "metadata": {"name": "p", "namespace": "ns", "uid": "u1", "creationTimestamp": "2026-10-24T10:00:00Z"},
  "spec": {"containers": [{"name": "main"}, {"name": "sidecar"}]},
  "status": {
    "conditions": [
      {"type": "PodScheduled", "status": "True", "lastTransitionTime": "2026-10-24T10:00:02Z"},
      {"type": "Ready", "status": "%s", "lastTransitionTime": "2026-10-24T10:01:00Z"}
    ],
    "containerStatuses": [
      {"name": "sidecar", "restartCount": 0, "state": {"running": {"startedAt": "2026-10-24T10:00:30Z"}}},
      {"name": "main", "restartCount": %d, "state": %s, "lastState": %s}
    ]
  }
}`

func pod(ready string, restarts int, state, last string) string {
	return fmt.Sprintf(podTmpl, ready, restarts, state, last)
}

const runningMain = `{"running": {"startedAt": "2026-10-24T10:00:40Z"}}`

func read(t *testing.T, podJSON, eventsJSON, log string) ([]model.Event, []model.Warning) {
	t.Helper()
	events, warnings, err := Read(strings.NewReader(podJSON), strings.NewReader(eventsJSON), strings.NewReader(log))
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	return events, warnings
}

// Events expired (DESIGN.md section 9.3): with no Scheduled event, the PodScheduled
// condition stands in, from the pod object. A Ready condition that is not True is no
// PodReady event.
func TestScheduledFallbackAndReadyNotTrue(t *testing.T) {
	events, _ := read(t, pod("False", 0, runningMain, `{}`), `{"kind": "List", "items": []}`, "")
	got := find(events, model.SourceContainerStatus, model.KindScheduled)
	if len(got) != 1 || !got[0].TS.Equal(ts("2026-10-24T10:00:02Z")) {
		t.Errorf("fallback Scheduled = %v, want one at 10:00:02", got)
	}
	if n := len(find(events, model.SourceContainerStatus, model.KindPodReady)); n != 0 {
		t.Errorf("PodReady events = %d, want 0 while Ready is False", n)
	}
}

// Only the first spec container is profiled: the sidecar's start is not a boot attempt.
// After restarts, the previous attempt's start comes from lastState.
func TestContainerStartsAndRestarts(t *testing.T) {
	last := `{"terminated": {"startedAt": "2026-10-24T10:00:35Z", "finishedAt": "2026-10-24T10:00:38Z"}}`
	events, _ := read(t, pod("True", 2, runningMain, last), `{"items": []}`, "")
	got := find(events, model.SourceContainerStatus, model.KindContainerStarted)
	if len(got) != 2 {
		t.Fatalf("ContainerStarted = %v, want 2", got)
	}
	byCount := map[string]time.Time{}
	for _, e := range got {
		byCount[e.Attrs[model.AttrRestartCount]] = e.TS
	}
	if !byCount["2"].Equal(ts("2026-10-24T10:00:40Z")) || !byCount["1"].Equal(ts("2026-10-24T10:00:35Z")) {
		t.Errorf("starts by restart count = %v, want 2 at 10:00:40 and 1 at 10:00:35", byCount)
	}

	// A container that has exited still has a start in state.terminated.
	exited := `{"terminated": {"startedAt": "2026-10-24T10:00:41Z", "finishedAt": "2026-10-24T10:00:50Z"}}`
	events, _ = read(t, pod("False", 0, exited, `{}`), `{"items": []}`, "")
	if got := find(events, model.SourceContainerStatus, model.KindContainerStarted); len(got) != 1 || !got[0].TS.Equal(ts("2026-10-24T10:00:41Z")) {
		t.Errorf("exited container start = %v, want one at 10:00:41", got)
	}

	// Waiting with no start (still pulling): no ContainerStarted.
	events, _ = read(t, pod("False", 0, `{"waiting": {"reason": "ContainerCreating"}}`, `{}`), `{"items": []}`, "")
	if got := find(events, model.SourceContainerStatus, model.KindContainerStarted); len(got) != 0 {
		t.Errorf("waiting container starts = %v, want none", got)
	}
}

// Events about other pods or other containers are dropped. eventTime, when set, wins over
// firstTimestamp because it has microseconds. Both EventList and List are accepted.
func TestEventSelection(t *testing.T) {
	eventsJSON := `{"kind": "EventList", "items": [
	  {"reason": "Scheduled", "message": "assigned", "involvedObject": {"kind": "Pod", "name": "p", "namespace": "ns", "uid": "u1"},
	   "eventTime": "2026-10-24T10:00:01.250000Z", "firstTimestamp": null},
	  {"reason": "Pulling", "message": "pull main", "involvedObject": {"kind": "Pod", "name": "p", "namespace": "ns", "uid": "u1", "fieldPath": "spec.containers{main}"},
	   "firstTimestamp": "2026-10-24T10:00:03Z", "lastTimestamp": "2026-10-24T10:00:09Z"},
	  {"reason": "Pulling", "message": "pull sidecar", "involvedObject": {"kind": "Pod", "name": "p", "namespace": "ns", "uid": "u1", "fieldPath": "spec.containers{sidecar}"},
	   "firstTimestamp": "2026-10-24T10:00:04Z"},
	  {"reason": "Pulling", "message": "other pod", "involvedObject": {"kind": "Pod", "name": "q", "namespace": "ns", "uid": "u2"},
	   "firstTimestamp": "2026-10-24T10:00:05Z"},
	  {"reason": "Pulling", "message": "older pod with the same name", "involvedObject": {"kind": "Pod", "name": "p", "namespace": "ns", "uid": "u0"},
	   "firstTimestamp": "2026-10-24T09:00:05Z"},
	  {"reason": "Pulled", "message": "no time at all", "involvedObject": {"kind": "Pod", "name": "p", "namespace": "ns", "uid": "u1"}}
	]}`
	events, _ := read(t, pod("True", 0, runningMain, `{}`), eventsJSON, "")
	k8s := find(events, model.SourceK8sEvent, model.KindScheduled)
	if len(k8s) != 1 || !k8s[0].TS.Equal(ts("2026-10-24T10:00:01.25Z")) {
		t.Errorf("Scheduled = %v, want one at 10:00:01.25 from eventTime", k8s)
	}
	pulls := find(events, model.SourceK8sEvent, model.KindPulling)
	if len(pulls) != 1 || pulls[0].Raw != "pull main" || !pulls[0].TS.Equal(ts("2026-10-24T10:00:03Z")) {
		t.Errorf("Pulling = %v, want only the main container's, at its firstTimestamp", pulls)
	}
	if n := len(find(events, model.SourceK8sEvent, model.KindPulled)); n != 0 {
		t.Errorf("an event with no timestamp was kept")
	}
	if n := len(find(events, model.SourceContainerStatus, model.KindScheduled)); n != 0 {
		t.Errorf("fallback Scheduled emitted although the event exists")
	}
}

// Decision d1: lines without a timestamp prefix become no event and are counted in a
// warning. A wholly untimed log gives no log events at all.
func TestUntimedLog(t *testing.T) {
	log := "2026-10-24T10:00:41.5Z first\nno timestamp here\n\n2026-10-24T10:00:42Z  second with two spaces\r\nalso untimed\n"
	events, warnings := read(t, pod("True", 0, runningMain, `{}`), `{"items": []}`, log)
	lines := find(events, model.SourceEngineLog, model.KindLogLine)
	if len(lines) != 2 || lines[0].Raw != "first" || lines[1].Raw != " second with two spaces" {
		t.Errorf("log events = %+v, want the two timed lines with the prefix and one space removed", lines)
	}
	want := model.Warning{Code: "log-untimed", Text: "2 of 4 log lines have no timestamp and were skipped; capture logs with kubectl logs --timestamps"}
	if len(warnings) != 1 || warnings[0] != want {
		t.Errorf("warnings = %v, want %v", warnings, want)
	}

	events, warnings = read(t, pod("True", 0, runningMain, `{}`), `{"items": []}`, "plain\nlog\n")
	if n := len(find(events, model.SourceEngineLog, model.KindLogLine)); n != 0 || len(warnings) != 1 {
		t.Errorf("untimed log: %d log events, warnings %v; want 0 and one warning", n, warnings)
	}
}

// Raw keeps at most 512 bytes and never splits a UTF-8 character.
func TestRawTruncated(t *testing.T) {
	long := strings.Repeat("a", 511) + "é" + strings.Repeat("b", 100)
	events, _ := read(t, pod("True", 0, runningMain, `{}`), `{"items": []}`, "2026-10-24T10:00:41Z "+long+"\n")
	raw := find(events, model.SourceEngineLog, model.KindLogLine)[0].Raw
	if len(raw) > 512 || !utf8.ValidString(raw) || raw != strings.Repeat("a", 511) {
		t.Errorf("raw has %d bytes, valid %v", len(raw), utf8.ValidString(raw))
	}
}

func TestReadErrors(t *testing.T) {
	cases := map[string][3]string{
		"bad pod json":    {`{`, `{"items": []}`, ""},
		"bad events json": {pod("True", 0, runningMain, `{}`), `[`, ""},
		"no containers":   {`{"metadata": {"name": "p"}, "spec": {"containers": []}}`, `{"items": []}`, ""},
	}
	for name, c := range cases {
		if _, _, err := Read(strings.NewReader(c[0]), strings.NewReader(c[1]), strings.NewReader(c[2])); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
	if _, _, err := Load(fixture+"missing.json", fixture+"events.json", fixture+"container.log"); err == nil || !strings.Contains(err.Error(), "missing.json") {
		t.Errorf("missing file error = %v, want one naming the file", err)
	}
}
