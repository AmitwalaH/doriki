package model

import (
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

// marshalToMap encodes v and decodes it back into generic JSON so tests check the real
// output, not the struct.
func marshalToMap(t *testing.T, v any) map[string]any {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("unmarshal %s: %v", b, err)
	}
	return m
}

// assertNoNull fails if any value anywhere in decoded JSON is null.
func assertNoNull(t *testing.T, path string, v any) {
	t.Helper()
	switch x := v.(type) {
	case nil:
		t.Errorf("%s is null", path)
	case map[string]any:
		for k, e := range x {
			assertNoNull(t, path+"."+k, e)
		}
	case []any:
		for i, e := range x {
			assertNoNull(t, path+"["+strconv.Itoa(i)+"]", e)
		}
	}
}

func seconds(f float64) *float64 { return &f }

// A report before M3: no manifest, no advice, no gaps, no warnings, no events, totals not
// observed. None of those keys may appear, and nothing may be null.
func TestReportOmitsEmptyOptionalFields(t *testing.T) {
	r := Report{
		ReportVersion: ReportVersion,
		GeneratedAt:   time.Date(2026, 10, 24, 18, 2, 11, 0, time.UTC),
		DorikiVersion: "dev",
		Phases:        []Phase{{Name: PhaseWeightLoad, State: StateAbsent}},
	}
	m := marshalToMap(t, r)

	for _, key := range []string{"manifest", "advice", "gaps", "warnings", "events", "totals", "attempts"} {
		if _, ok := m[key]; ok {
			t.Errorf("key %q present, want omitted: %v", key, m[key])
		}
	}
	if m["report_version"] != float64(1) {
		t.Errorf("report_version = %v, want 1", m["report_version"])
	}
	assertNoNull(t, "report", m)
}

// An absent phase has no start, end, seconds, attrs or sources: writing a zero time or 0 s
// would put a false number on screen.
func TestAbsentPhaseHasOnlyNameAndState(t *testing.T) {
	m := marshalToMap(t, Phase{Name: PhaseCompile, State: StateAbsent})
	want := map[string]any{"name": "compile", "state": "absent"}
	if len(m) != len(want) || m["name"] != want["name"] || m["state"] != want["state"] {
		t.Errorf("got %v, want %v", m, want)
	}
}

// Kubernetes timestamps have 1 s resolution, so a complete 0 s phase is common (scheduling
// on an idle kind cluster). Its seconds must be written, not dropped as empty.
func TestCompleteZeroSecondPhaseWritesSeconds(t *testing.T) {
	ts := time.Date(2026, 10, 24, 18, 0, 0, 0, time.UTC)
	m := marshalToMap(t, Phase{
		Name: PhaseScheduling, Start: ts, End: ts, Seconds: seconds(0),
		State: StateComplete, Sources: []Source{SourceK8sEvent},
	})
	if v, ok := m["seconds"]; !ok || v != float64(0) {
		t.Errorf("seconds = %v (present %v), want 0", v, ok)
	}
	if m["start"] != "2026-10-24T18:00:00Z" {
		t.Errorf("start = %v, want 2026-10-24T18:00:00Z", m["start"])
	}
}

// Totals: an observed value is written even when 0; an unobserved one is omitted.
func TestTotalsOmitUnobserved(t *testing.T) {
	m := marshalToMap(t, Totals{PendingToReadySeconds: seconds(0)})
	if v, ok := m["pending_to_ready_seconds"]; !ok || v != float64(0) {
		t.Errorf("pending_to_ready_seconds = %v (present %v), want 0", v, ok)
	}
	if _, ok := m["pending_to_first_token_seconds"]; ok {
		t.Errorf("pending_to_first_token_seconds present, want omitted")
	}
}

// Attempt 0 is the first boot and is always written; the per-attempt list only appears
// when filled (--all-attempts).
func TestReportAttemptAlwaysWritten(t *testing.T) {
	m := marshalToMap(t, Report{ReportVersion: ReportVersion, Attempt: 0, AttemptsTotal: 1})
	if v, ok := m["attempt"]; !ok || v != float64(0) {
		t.Errorf("attempt = %v (present %v), want 0", v, ok)
	}
	if m["attempts_total"] != float64(1) {
		t.Errorf("attempts_total = %v, want 1", m["attempts_total"])
	}
	if _, ok := m["phases"]; ok {
		t.Errorf("phases present on an empty report, want omitted")
	}
}
