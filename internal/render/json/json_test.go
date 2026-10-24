package json

import (
	"bytes"
	stdjson "encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

func decode(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := stdjson.Unmarshal(b, &m); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, b)
	}
	return m
}

func walkNoNull(t *testing.T, path string, v any) {
	t.Helper()
	switch x := v.(type) {
	case nil:
		t.Errorf("%s is null", path)
	case map[string]any:
		for k, e := range x {
			walkNoNull(t, path+"."+k, e)
		}
	case []any:
		for _, e := range x {
			walkNoNull(t, path+"[]", e)
		}
	}
}

func report() model.Report {
	ts := time.Date(2026, 10, 24, 14, 57, 53, 0, time.UTC)
	s := 0.0
	return model.Report{
		ReportVersion: model.ReportVersion,
		GeneratedAt:   time.Date(2026, 10, 24, 15, 0, 0, 0, time.UTC),
		DorikiVersion: "dev",
		AttemptsTotal: 1,
		Phases: []model.Phase{
			{Name: model.PhaseScheduling, Start: ts, End: ts, Seconds: &s, State: model.StateComplete, Sources: []model.Source{model.SourceK8sEvent}},
			{Name: model.PhaseNodeProvision, State: model.StateAbsent},
		},
		Warnings: []model.Warning{{Code: "no-pack", Text: `no parser pack matches image "a<b>&c"`}},
	}
}

// DESIGN.md section 4.4: report_version 1, absent optional fields omitted, never null, one
// object per output followed by a newline.
func TestWriteShape(t *testing.T) {
	var buf bytes.Buffer
	if err := Write(&buf, report()); err != nil {
		t.Fatal(err)
	}
	out := buf.Bytes()
	m := decode(t, out)
	walkNoNull(t, "report", m)
	if m["report_version"] != float64(1) || m["generated_at"] != "2026-10-24T15:00:00Z" {
		t.Errorf("report_version %v, generated_at %v", m["report_version"], m["generated_at"])
	}
	for _, k := range []string{"manifest", "advice", "gaps", "events", "attempts", "totals"} {
		if _, ok := m[k]; ok {
			t.Errorf("empty %q written", k)
		}
	}
	if !bytes.HasSuffix(out, []byte("}\n")) || bytes.Count(out, []byte("\n}\n")) != 1 {
		t.Errorf("want one indented object and a trailing newline, got:\n%s", out)
	}
	// Indented for people reading it in a terminal or a diff.
	if !strings.Contains(string(out), "\n  \"report_version\": 1,\n") {
		t.Errorf("not indented by two spaces:\n%s", out)
	}
	// Log lines and image names are shown as written, not as JSON unicode escapes.
	if !strings.Contains(string(out), `a<b>&c`) {
		t.Errorf("HTML characters escaped:\n%s", out)
	}
}

// --include-events fills Report.Events; the renderer writes whatever the report holds.
func TestWriteEvents(t *testing.T) {
	r := report()
	r.Events = []model.Event{{TS: time.Date(2026, 10, 24, 14, 58, 13, 306219429, time.UTC), Source: model.SourceEngineLog, Kind: model.KindLogLine, Raw: "x", Confidence: 1}}
	var buf bytes.Buffer
	if err := Write(&buf, r); err != nil {
		t.Fatal(err)
	}
	m := decode(t, buf.Bytes())
	ev, ok := m["events"].([]any)
	if !ok || len(ev) != 1 || ev[0].(map[string]any)["ts"] != "2026-10-24T14:58:13.306219429Z" {
		t.Errorf("events = %v", m["events"])
	}
	walkNoNull(t, "report", m)
}

type failWriter struct{}

func (failWriter) Write([]byte) (int, error) { return 0, bytes.ErrTooLarge }

func TestWriteError(t *testing.T) {
	if err := Write(failWriter{}, report()); err == nil {
		t.Errorf("write error swallowed")
	}
}
