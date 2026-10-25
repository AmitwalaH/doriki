package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/model"
)

const fixture = "../../packs/generic/testdata/kind-nginx/"

var fixtureArgs = []string{fixture + "pod.json", fixture + "events.json", fixture + "container.log"}

// run executes the real root command with args and returns stdout and the error.
func run(t *testing.T, args ...string) (string, error) {
	t.Helper()
	saved := now
	t.Cleanup(func() { now = saved })
	now = func() time.Time { return time.Date(2026, 10, 24, 16, 0, 0, 0, time.FixedZone("x", 3600)) }

	var out, errOut bytes.Buffer
	root := NewRootCmd()
	root.SetOut(&out)
	root.SetErr(&errOut)
	root.SetArgs(args)
	err := root.Execute()
	return out.String(), err
}

func decodeReport(t *testing.T, s string) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		t.Fatalf("not JSON: %v\n%s", err, s)
	}
	return m
}

// The M1a goal command: a report from the real kind capture, generated_at from the clock in
// UTC, the generic pack's warning naming the image, and no events unless asked.
func TestFromFilesJSON(t *testing.T) {
	out, err := run(t, append([]string{"--from-files", "-o", "json"}, fixtureArgs...)...)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	m := decodeReport(t, out)
	if m["report_version"] != float64(1) || m["doriki_version"] != "dev" || m["generated_at"] != "2026-10-24T15:00:00Z" {
		t.Errorf("header: version %v, doriki %v, generated_at %v", m["report_version"], m["doriki_version"], m["generated_at"])
	}
	if phases, _ := m["phases"].([]any); len(phases) != len(model.Phases()) {
		t.Errorf("phases = %d, want %d", len(phases), len(model.Phases()))
	}
	if _, ok := m["events"]; ok {
		t.Errorf("events written without --include-events")
	}
	warnings, _ := m["warnings"].([]any)
	if len(warnings) != 1 || !strings.Contains(warnings[0].(map[string]any)["text"].(string), `"nginx:1.29.3-alpine"`) {
		t.Errorf("warnings = %v, want the no-pack warning naming the image", warnings)
	}
	if !strings.HasSuffix(out, "}\n") {
		t.Errorf("output does not end with a newline")
	}
}

// --include-events adds every normalized input event: 41 from the capture plus the image
// fact (counted in the collector test).
func TestFromFilesIncludeEvents(t *testing.T) {
	out, err := run(t, append([]string{"--from-files", "-o", "json", "--include-events"}, fixtureArgs...)...)
	if err != nil {
		t.Fatalf("error: %v", err)
	}
	if events, _ := decodeReport(t, out)["events"].([]any); len(events) != 42 {
		t.Errorf("events = %d, want 42", len(events))
	}
}

func TestUsageErrors(t *testing.T) {
	cases := map[string]struct {
		args []string
		want string
	}{
		"live mode":      {[]string{"-o", "json", "pod/x"}, "--from-files"},
		"two files":      {[]string{"--from-files", "-o", "json", "a", "b"}, "3 files"},
		"text output":    {append([]string{"--from-files"}, fixtureArgs...), "-o json"},
		"unknown output": {append([]string{"--from-files", "-o", "yaml"}, fixtureArgs...), `"yaml"`},
		"missing file":   {[]string{"--from-files", "-o", "json", "nope.json", fixtureArgs[1], fixtureArgs[2]}, "nope.json"},
	}
	for name, c := range cases {
		out, err := run(t, c.args...)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: err = %v, want one mentioning %s", name, err, c.want)
		}
		if out != "" {
			t.Errorf("%s: wrote output on error: %q", name, out)
		}
	}
}

// pack list reads the same pack list as version; a sentinel list must show up.
func TestPackList(t *testing.T) {
	out, err := run(t, "pack", "list")
	if err != nil || out != "generic\nvllm/0.32\n" {
		t.Errorf("pack list = %q, %v; want generic and vllm/0.32", out, err)
	}
	saved := packNames
	t.Cleanup(func() { packNames = saved })
	packNames = func() []string { return []string{"vllm/0.28", "generic"} }
	if out, _ := run(t, "pack", "list"); out != "vllm/0.28\ngeneric\n" {
		t.Errorf("pack list with sentinel = %q", out)
	}
}

// Decision 0003: exit 1 only when a phase the selected pack can produce is open or absent.
func TestExitCode(t *testing.T) {
	expected := []model.PhaseName{model.PhaseScheduling, model.PhaseImagePull, model.PhaseContainerInit}
	phases := func(states ...model.PhaseState) model.Report {
		var r model.Report
		for i, name := range model.Phases() {
			st := model.StateAbsent
			if i < len(states) {
				st = states[i]
			}
			r.Phases = append(r.Phases, model.Phase{Name: name, State: st})
		}
		return r
	}
	c, a, o, inv := model.StateComplete, model.StateAbsent, model.StateOpen, model.StateInvalid
	cases := map[string]struct {
		r    model.Report
		want int
	}{
		"expected complete, engine phases absent": {phases(c, a, c, c), 0},
		"image-pull open":                         {phases(c, a, o, c), 1},
		"image-pull absent":                       {phases(c, a, a, c), 1},
		"scheduling invalid":                      {phases(inv, a, c, c), 0},
	}
	for name, tc := range cases {
		if got := exitCode(tc.r, expected); got != tc.want {
			t.Errorf("%s: exit %d, want %d", name, got, tc.want)
		}
	}
}

// A report with an expected phase open still prints in full, then the command fails with exit
// code 1 and no message (DESIGN.md section 7.3, decision 0003). The input is the real fixture
// with its Pulled event removed, written to a temporary file; the fixture is not touched.
func TestIncompleteReportExitsOne(t *testing.T) {
	raw, err := os.ReadFile(fixture + "events.json")
	if err != nil {
		t.Fatal(err)
	}
	var list map[string]any
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	var kept []any
	for _, e := range list["items"].([]any) {
		if e.(map[string]any)["reason"] != "Pulled" {
			kept = append(kept, e)
		}
	}
	list["items"] = kept
	b, _ := json.Marshal(list)
	events := filepath.Join(t.TempDir(), "events.json")
	if err := os.WriteFile(events, b, 0o600); err != nil {
		t.Fatal(err)
	}

	out, err := run(t, "--from-files", "-o", "json", fixtureArgs[0], events, fixtureArgs[2])
	if code, msg := Exit(err); code != 1 || msg != "" {
		t.Errorf("Exit = %d %q, want 1 and no message", code, msg)
	}
	if !strings.Contains(out, `"state": "open"`) {
		t.Errorf("report not written before exiting 1:\n%s", out)
	}

	// The unmodified fixture is complete: no error, exit 0.
	_, err = run(t, append([]string{"--from-files", "-o", "json"}, fixtureArgs...)...)
	if code, msg := Exit(err); code != 0 || msg != "" {
		t.Errorf("complete fixture: Exit = %d %q, want 0", code, msg)
	}
}

// Exit maps Execute's error to the process exit code and the message main prints.
func TestExit(t *testing.T) {
	cases := map[string]struct {
		err  error
		code int
		msg  string
	}{
		"success":         {nil, 0, ""},
		"usage":           {errors.New("bad flag"), ExitUsage, "bad flag"},
		"incomplete":      {&ExitError{Code: ExitIncomplete}, 1, ""},
		"wrapped":         {fmt.Errorf("x: %w", &ExitError{Code: ExitIncomplete}), 1, ""},
		"code and reason": {&ExitError{Code: 2, Err: errors.New("pod not found")}, 2, "pod not found"},
	}
	for name, c := range cases {
		if code, msg := Exit(c.err); code != c.code || msg != c.msg {
			t.Errorf("%s: Exit = %d %q, want %d %q", name, code, msg, c.code, c.msg)
		}
	}
}

// DK-PHASE-ENH-20261024-cached-image-pull: a pod that booted on a node already holding its
// image exits 0, with image-pull complete and cached (decision 0004), not 1 as if booting.
func TestWarmNodeExitsZero(t *testing.T) {
	warm := "../../packs/generic/testdata/kind-nginx-warm/"
	out, err := run(t, "--from-files", "-o", "json", warm+"pod.json", warm+"events.json", warm+"container.log")
	if code, msg := Exit(err); code != 0 || msg != "" {
		t.Errorf("Exit = %d %q, want 0", code, msg)
	}
	var r model.Report
	if err := json.Unmarshal([]byte(out), &r); err != nil {
		t.Fatal(err)
	}
	for _, p := range r.Phases {
		if p.Name == model.PhaseImagePull && (p.State != model.StateComplete || p.Attrs[model.AttrCached] != "true") {
			t.Errorf("image-pull = %+v, want complete and cached", p)
		}
	}
}

// Decision 0005: an expected phase the phase engine excused with a phase-unobserved warning
// (an engine phase still absent once the pod was Ready) and a skipped phase do not hold the
// exit code at 1; the same phase absent without that warning still does.
func TestExitCodeAfterReady(t *testing.T) {
	expected := []model.PhaseName{model.PhaseCompile, model.PhaseGraphCapture}
	report := func(compile, graph model.PhaseState, warnings ...model.Warning) model.Report {
		return model.Report{
			Phases:   []model.Phase{{Name: model.PhaseCompile, State: compile}, {Name: model.PhaseGraphCapture, State: graph}},
			Warnings: warnings,
		}
	}
	unobserved := model.Warning{Code: model.WarnPhaseUnobserved, Phase: model.PhaseGraphCapture}
	cases := map[string]struct {
		r    model.Report
		want int
	}{
		"skipped and excused":              {report(model.StateSkipped, model.StateAbsent, unobserved), 0},
		"absent, not excused":              {report(model.StateSkipped, model.StateAbsent), 1},
		"warning names another phase":      {report(model.StateAbsent, model.StateComplete, unobserved), 1},
		"other warning code for the phase": {report(model.StateSkipped, model.StateAbsent, model.Warning{Code: "x", Phase: model.PhaseGraphCapture}), 1},
		"open still counts":                {report(model.StateSkipped, model.StateOpen, unobserved), 1},
	}
	for name, tc := range cases {
		if got := exitCode(tc.r, expected); got != tc.want {
			t.Errorf("%s: exit %d, want %d", name, got, tc.want)
		}
	}
}

// A start_from_duration marker produces its ".start" marker too, so the pack can produce the
// phase: markerIDs must list the derived id, or the phase engine treats weight-fetch as one
// the pack cannot produce and leaves it absent without a warning.
func TestMarkerIDsIncludeDerivedStart(t *testing.T) {
	p := model.Pack{Markers: []model.PackMarker{
		{ID: "x.weights.fetch.end", StartFromDuration: "s"},
		{ID: "x.kv.end"},
	}}
	if got, want := markerIDs(p), []string{"x.weights.fetch.end", "x.weights.fetch.start", "x.kv.end"}; !slices.Equal(got, want) {
		t.Errorf("markerIDs = %v, want %v", got, want)
	}
}
