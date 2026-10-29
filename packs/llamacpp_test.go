package packs

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

const llamacppFixture = "llamacpp/testdata/11889/"

// llamacppLines is the capture's log with the kubectl --timestamps prefix removed, whole: the
// pipeline matches whole lines (DK-COLLECT-BUG-20261025-match-full-log-line).
func llamacppLines(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(llamacppFixture + "container.log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		_, rest, _ := strings.Cut(sc.Text(), " ")
		out = append(out, rest)
	}
	return out
}

func llamacppPack(t *testing.T) model.Pack {
	t.Helper()
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(p model.Pack) bool { return p.Name == "llamacpp/11889" })
	if i < 0 {
		t.Fatal("no llamacpp/11889 pack")
	}
	return all[i]
}

// matching returns the 1-based numbers of the lines regex matches.
func matching(regex string, lines []string) []int {
	re := regexp.MustCompile(regex)
	var out []int
	for i, l := range lines {
		if re.MatchString(l) {
			out = append(out, i+1)
		}
	}
	return out
}

// At the default level, every marker the pack keeps (decision 0010) matches exactly the
// capture lines its comment in the pack cites, so the pack says nothing the capture does not
// show. The fetch markers are at 0.8 because the download prints nothing; the rest are at 1.0.
func TestLlamacppMarkersAgainstFixture(t *testing.T) {
	lines := llamacppLines(t)
	p := atLogLevel(t, llamacppPack(t), lines)
	want := map[string][]int{
		"llamacpp.engine.init.end":     {4},
		"llamacpp.weights.fetch.start": {4},
		"llamacpp.weights.fetch.end":   {6},
		"llamacpp.weights.load.start":  {6},
		"llamacpp.weights.load.end":    {14},
		"llamacpp.server.listening":    {15},
	}
	if len(p.Markers) != len(want) {
		t.Errorf("pack has %d markers, want %d", len(p.Markers), len(want))
	}
	for _, m := range p.Markers {
		if got := matching(m.Regex, lines); !slices.Equal(got, want[m.ID]) {
			t.Errorf("%s matches lines %v, want %v", m.ID, got, want[m.ID])
		}
		conf := float32(1)
		if m.Confidence != nil {
			conf = *m.Confidence
		}
		if fetch := strings.Contains(m.ID, ".weights.fetch."); fetch && conf != 0.8 || !fetch && conf != 1 {
			t.Errorf("%s confidence %v", m.ID, conf)
		}
	}
	for _, m := range p.Markers {
		if strings.Contains(m.ID, ".kv.") {
			t.Errorf("%s: at the default level this pack keeps no kv markers (decisions 0003, 0010)", m.ID)
		}
	}
}

// The default level prints no version, so log_regex matches nothing here and selection warns
// with the pack's note; the image still selects this pack.
func TestLlamacppDetection(t *testing.T) {
	p, lines := llamacppPack(t), llamacppLines(t)
	if got := matching(p.Detect.LogRegex, lines); len(got) != 0 {
		t.Errorf("log_regex matches lines %v of a default-level capture", got)
	}
	if !regexp.MustCompile(p.Detect.LogRegex).MatchString("I cmn  common_param: common_params_print_info: build 11889 (d854d98ba) with GNU 14.2.0 for Linux x86_64") {
		t.Errorf("log_regex does not read the -lv 4 build line recorded in SOURCE.md")
	}

	var pod corev1.Pod
	b, err := os.ReadFile(llamacppFixture + "pod.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &pod); err != nil {
		t.Fatal(err)
	}
	got, warnings, err := Select(pod.Spec.Containers[0].Image, lines...)
	if err != nil || got.Name != "llamacpp/11889" {
		t.Fatalf("selected %q (%v), want llamacpp/11889", got.Name, err)
	}
	if len(warnings) != 1 || warnings[0].Code != "pack-version-unknown" || !strings.Contains(warnings[0].Text, "-lv 4") {
		t.Errorf("warnings = %v, want pack-version-unknown saying the build shows only at -lv 4", warnings)
	}
}

// At the default level, each fact the pack keeps reads a real line: the model the server
// loaded.
func TestLlamacppFacts(t *testing.T) {
	lines := llamacppLines(t)
	p := atLogLevel(t, llamacppPack(t), lines)
	for _, f := range p.Facts {
		re := regexp.MustCompile(f.Regex)
		var v string
		for _, l := range lines {
			if m := re.FindStringSubmatch(l); m != nil {
				v = m[re.SubexpIndex("v")]
				break
			}
		}
		if f.Key != "model.ref" || v != "ggml-org/models" {
			t.Errorf("fact %s = %q, want model.ref ggml-org/models", f.Key, v)
		}
	}
	if !slices.Equal(p.NotApplicable, []model.PhaseName{model.PhaseCompile, model.PhaseGraphCapture}) {
		t.Errorf("not_applicable = %v", p.NotApplicable)
	}
}
