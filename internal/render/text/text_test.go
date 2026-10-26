package text

import (
	"bytes"
	"strings"
	"testing"
	"time"
	"unicode/utf8"

	"charm.land/lipgloss/v2"

	"github.com/AmitwalaH/doriki/internal/model"
)

var t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

func at(s float64) time.Time { return t0.Add(time.Duration(s * float64(time.Second))) }

func f64(f float64) *float64 { return &f }

func f32(f float32) *float32 { return &f }

func complete(name model.PhaseName, from, to float64) model.Phase {
	return model.Phase{Name: name, Start: at(from), End: at(to), Seconds: f64(to - from), State: model.StateComplete}
}

func render(t *testing.T, r model.Report, color bool) string {
	t.Helper()
	var b bytes.Buffer
	if err := Write(&b, r, Options{Color: color}); err != nil {
		t.Fatalf("Write: %v", err)
	}
	return b.String()
}

// row returns the line for a phase or gap name.
func row(t *testing.T, out, name string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, name+" ") {
			return l
		}
	}
	t.Fatalf("no row %q in:\n%s", name, out)
	return ""
}

// bar returns the first and last track cells (0-based) a row's bar occupies.
func bar(t *testing.T, line string) (first, last int) {
	t.Helper()
	track := []rune(line)[nameWidth : nameWidth+trackWidth]
	first, last = -1, -1
	for i, r := range track {
		if r == '█' || r == '░' || r == '│' {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	return first, last
}

// A 100 s boot: the track's 44 cells span pod creation (scheduling's start) to Ready, so each
// cell is 100/44 s. Bars start where the phase starts, not at the left edge, and their length
// is the phase's share of the span: a bar covers cells round(start) to round(end)-1 in track
// units, so neighbours touch without overlapping.
func TestWaterfallGeometry(t *testing.T) {
	r := model.Report{
		Phases: []model.Phase{
			complete(model.PhaseScheduling, 0, 0),
			complete(model.PhaseImagePull, 1, 51),
			complete(model.PhaseWeightFetch, 60, 82),
		},
		Gaps:   []model.Gap{{After: model.PhaseImagePull, Before: model.PhaseWeightFetch, Seconds: 9}},
		Totals: model.Totals{PendingToReadySeconds: f64(100)},
	}
	out := render(t, r, false)
	for name, want := range map[string][2]int{
		"scheduling":   {0, 0},   // 0 s: one marker cell at its start
		"image-pull":   {0, 21},  // 1 s rounds to cell 0; 51 s is 22.4 cells; 50 s = 22 cells
		"weight-fetch": {26, 35}, // starts at 60 s (cell 26), 22 s = 10 cells
		"gap":          {22, 25}, // 51 s to 60 s
	} {
		if first, last := bar(t, row(t, out, name)); first != want[0] || last != want[1] {
			t.Errorf("%s: bar cells %d..%d, want %d..%d\n%s", name, first, last, want[0], want[1], out)
		}
	}
	if !strings.Contains(row(t, out, "scheduling"), "│") {
		t.Errorf("0 s phase drawn without its marker: %q", row(t, out, "scheduling"))
	}
	// The gap row follows the phase it comes after.
	lines := strings.Split(out, "\n")
	for i, l := range lines {
		if strings.HasPrefix(l, "image-pull ") && !strings.HasPrefix(lines[i+1], "gap ") {
			t.Errorf("gap row not after image-pull:\n%s", out)
		}
	}
}

// Durations are m:ss, h:mm:ss past an hour, and one decimal under a second (DESIGN.md 7.5).
func TestDuration(t *testing.T) {
	for in, want := range map[float64]string{
		0: "0.0s", 0.0024: "0.0s", 0.367: "0.4s", 0.49: "0.5s", 1.474: "0:01", 70.586: "1:11",
		108.186: "1:48", 268: "4:28", 456: "7:36", 3599.6: "1:00:00", 3725: "1:02:05",
	} {
		if got := duration(in); got != want {
			t.Errorf("duration(%v) = %q, want %q", in, got, want)
		}
	}
}

// A phase below confidence 1.0 has "~" before its duration and a one-line legend explains it;
// with no such phase there is no legend.
func TestConfidenceTilde(t *testing.T) {
	p := complete(model.PhaseWeightFetch, 10, 118)
	p.Confidence = f32(0.9)
	r := model.Report{Phases: []model.Phase{complete(model.PhaseScheduling, 0, 0), p, complete(model.PhaseWeightLoad, 118, 119)},
		Totals: model.Totals{PendingToReadySeconds: f64(120)}}
	out := render(t, r, false)
	if !strings.Contains(row(t, out, "weight-fetch"), "~1:48") || strings.Contains(row(t, out, "weight-load"), "~") {
		t.Errorf("tilde on the wrong rows:\n%s", out)
	}
	if !strings.Contains(out, "\n~ ") {
		t.Errorf("no legend for ~:\n%s", out)
	}
	r.Phases[1].Confidence = nil
	if strings.Contains(render(t, r, false), "~") {
		t.Errorf("legend or tilde with every phase at 1.0")
	}
}

// Absent and skipped phases are not rows: one line each, in the fixed order, the absent line
// naming the pack from the manifest. Open runs to the end of the track; invalid has no bar.
func TestStatesAndSummaryLines(t *testing.T) {
	r := model.Report{
		Manifest: &model.Manifest{Engine: &model.ManifestEngine{ParserPack: "vllm/0.32"}},
		Phases: []model.Phase{
			complete(model.PhaseScheduling, 0, 0),
			{Name: model.PhaseNodeProvision, State: model.StateAbsent},
			{Name: model.PhaseImagePull, Start: at(5), End: at(2), State: model.StateInvalid},
			{Name: model.PhaseContainerInit, Start: at(10), State: model.StateOpen},
			{Name: model.PhaseCompile, State: model.StateSkipped},
			{Name: model.PhaseGraphCapture, State: model.StateSkipped},
			{Name: model.PhaseFirstToken, State: model.StateAbsent},
		},
		Totals: model.Totals{PendingToReadySeconds: f64(44)},
	}
	out := render(t, r, false)
	for _, want := range []string{
		"\nskipped: compile, graph-capture (the engine said it skips them)\n",
		"\nabsent: node-provision, first-token (not seen in the events or the log; pack vllm/0.32)\n",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("missing %q in:\n%s", want, out)
		}
	}
	for _, name := range []string{"node-provision ", "compile ", "first-token "} {
		if strings.Contains(out, "\n"+name) {
			t.Errorf("%q has a row", name)
		}
	}
	if first, last := bar(t, row(t, out, "container-init")); first != 10 || last != trackWidth-1 || !strings.Contains(row(t, out, "container-init"), "open") {
		t.Errorf("open phase: cells %d..%d, row %q; want 10..%d and 'open'", first, last, row(t, out, "container-init"), trackWidth-1)
	}
	if first, _ := bar(t, row(t, out, "image-pull")); first != -1 || !strings.Contains(row(t, out, "image-pull"), "invalid") {
		t.Errorf("invalid phase drawn or unlabelled: %q", row(t, out, "image-pull"))
	}
}

// The header names the pod, engine, node and total; selected attributes appear after the
// duration. Nothing is wider than 100 columns, however long the names, attributes or warnings.
func TestHeaderAttrsAndWidth(t *testing.T) {
	pull := complete(model.PhaseImagePull, 1, 30)
	pull.Attrs = map[string]string{model.AttrCached: "true"}
	fetch := complete(model.PhaseWeightFetch, 30, 60)
	fetch.Attrs = map[string]string{"model": "org/" + strings.Repeat("very-long-model-name-", 5), "seconds": "30"}
	kv := complete(model.PhaseKVAlloc, 60, 61)
	kv.Attrs = map[string]string{"gib": "1.0", "tokens": "29,056"}
	r := model.Report{
		Manifest: &model.Manifest{
			Pod:    &model.ManifestPod{Namespace: "default", Name: "vllm-785fb8997d-bvsz4" + strings.Repeat("x", 60)},
			Node:   &model.ManifestNode{Name: "doriki-control-plane"},
			Engine: &model.ManifestEngine{Name: "vllm", Version: "0.32.0", ParserPack: "vllm/0.32"},
		},
		Phases:   []model.Phase{complete(model.PhaseScheduling, 0, 1), pull, fetch, kv},
		Totals:   model.Totals{PendingToReadySeconds: f64(456)},
		Warnings: []model.Warning{{Code: "no-pack", Text: strings.Repeat("a long warning text ", 12)}},
	}
	out := render(t, r, false)
	if !strings.HasPrefix(out, "Pod default/vllm-785fb8997d-bvsz4") || !strings.Contains(out, "total 7:36") {
		t.Errorf("header: %q", strings.SplitN(out, "\n", 2)[0])
	}
	if !strings.Contains(row(t, out, "image-pull"), "image already on node") ||
		!strings.Contains(row(t, out, "kv-alloc"), "1.0 GiB, 29,056 tokens") ||
		!strings.Contains(row(t, out, "weight-fetch"), "org/very-long") || strings.Contains(row(t, out, "weight-fetch"), "seconds") {
		t.Errorf("attributes:\n%s", out)
	}
	if !strings.Contains(out, "\nWarnings\n  no-pack: a long warning") {
		t.Errorf("warnings block:\n%s", out)
	}
	for _, l := range strings.Split(out, "\n") {
		if w := lipgloss.Width(l); w > Width {
			t.Errorf("line is %d columns: %q", w, l)
		}
		if !utf8.ValidString(l) {
			t.Errorf("invalid UTF-8: %q", l)
		}
	}
	// The engine is unknown under the generic pack: the header names the pack instead.
	r.Manifest.Engine = &model.ManifestEngine{ParserPack: "generic"}
	if h := strings.SplitN(render(t, r, false), "\n", 2)[0]; !strings.Contains(h, "pack generic") || strings.Contains(h, "engine") {
		t.Errorf("generic header: %q", h)
	}
}

// Colour only when asked for; without it the output has no escape codes at all.
func TestColor(t *testing.T) {
	r := model.Report{Phases: []model.Phase{complete(model.PhaseScheduling, 0, 3)}, Totals: model.Totals{PendingToReadySeconds: f64(3)},
		Warnings: []model.Warning{{Code: "x", Text: "y"}}}
	if out := render(t, r, false); strings.Contains(out, "\x1b") {
		t.Errorf("escape codes with colour off: %q", out)
	}
	if out := render(t, r, true); !strings.Contains(out, "\x1b[") {
		t.Errorf("no escape codes with colour on: %q", out)
	}
}

// At most three advice lines (DESIGN.md section 2); none, no Advice block.
func TestAdvice(t *testing.T) {
	r := model.Report{Phases: []model.Phase{complete(model.PhaseScheduling, 0, 3)}, Totals: model.Totals{PendingToReadySeconds: f64(3)}}
	if strings.Contains(render(t, r, false), "Advice") {
		t.Errorf("Advice block with no advice")
	}
	for i := range 4 {
		r.Advice = append(r.Advice, model.Advice{Rule: "r", Text: "advice " + string(rune('A'+i))})
	}
	out := render(t, r, false)
	if !strings.Contains(out, "\nAdvice\n  1. advice A\n  2. advice B\n  3. advice C\n") || strings.Contains(out, "advice D") {
		t.Errorf("advice block:\n%s", out)
	}
}

// The duration shown is the report's seconds, not end minus start: the phase engine clamps an
// end up to 1 s before the start to 0 s (timestamp resolution), and the text must agree with
// the JSON rather than print "-0.0s". Seen on the vLLM fixture's server-ready.
func TestDurationIsReportSeconds(t *testing.T) {
	p := model.Phase{Name: model.PhaseServerReady, Start: at(10.039), End: at(10), Seconds: f64(0), State: model.StateComplete}
	out := render(t, model.Report{Phases: []model.Phase{complete(model.PhaseScheduling, 0, 0), p}, Totals: model.Totals{PendingToReadySeconds: f64(10)}}, false)
	if r := row(t, out, "server-ready"); !strings.HasSuffix(r, " 0.0s") || strings.Contains(r, "-0") {
		t.Errorf("server-ready row %q, want 0.0s", r)
	}
}

// A gap with a reason is labelled with it instead of "gap" (decision 0007), and gaps after the
// same phase are drawn one after the other, so a split interval is not drawn twice over.
func TestGapReason(t *testing.T) {
	r := model.Report{
		Phases: []model.Phase{
			complete(model.PhaseScheduling, 0, 0),
			complete(model.PhaseImagePull, 44, 88),
		},
		Gaps: []model.Gap{
			{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: 22, Reason: model.GapEarlierAttempts},
			{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: 22},
		},
	}
	out := render(t, r, false)
	// The label fills the name column, so its row is found by prefix alone.
	var earlier string
	for _, l := range strings.Split(out, "\n") {
		if strings.HasPrefix(l, model.GapEarlierAttempts) {
			earlier = l
		}
	}
	if earlier == "" {
		t.Fatalf("no %q row in:\n%s", model.GapEarlierAttempts, out)
	}
	ef, el := bar(t, earlier)
	gf, gl := bar(t, row(t, out, "gap"))
	if ef != 0 || el != 10 || gf != 11 || gl != 21 {
		t.Errorf("earlier attempts cells %d-%d, gap %d-%d; want 0-10 then 11-21:\n%s", ef, el, gf, gl, out)
	}
}
