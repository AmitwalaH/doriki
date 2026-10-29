package packs

import (
	"bufio"
	"encoding/json"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	corev1 "k8s.io/api/core/v1"

	"github.com/AmitwalaH/doriki/internal/model"
)

const llamacppLv4Fixture = "llamacpp/testdata/11889-lv4/"

// capturedLines is a fixture's log with the kubectl --timestamps prefix removed, whole: the
// pipeline matches whole lines (DK-COLLECT-BUG-20261025-match-full-log-line).
func capturedLines(t *testing.T, dir string) []string {
	t.Helper()
	f, err := os.Open(dir + "container.log") // #nosec G304 -- a fixture path
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

// atLogLevel is the pack as selection gives it for these lines: without the markers and facts
// their log level does not reach (decision 0010).
func atLogLevel(t *testing.T, p model.Pack, lines []string) model.Pack {
	t.Helper()
	level, err := LogLevel(p, lines)
	if err != nil {
		t.Fatal(err)
	}
	return AtLevel(p, level)
}

// At -lv 4 the log says level 4, so the pack keeps every marker, and each matches exactly the
// lines of the -lv 4 capture its comment in the pack cites. The gated ones are tested
// (confidence 1.0) and are exactly the context and KV cache lines the default level does not
// print.
func TestLlamacppLv4MarkersAgainstFixture(t *testing.T) {
	lines := capturedLines(t, llamacppLv4Fixture)
	full := llamacppPack(t)
	level, err := LogLevel(full, lines)
	if err != nil || level != (Level{N: 4, Known: true}) {
		t.Fatalf("level %+v (%v), want 4", level, err)
	}
	p := AtLevel(full, level)
	want := map[string][]int{
		"llamacpp.engine.init.end":          {6},
		"llamacpp.weights.fetch.start":      {6},
		"llamacpp.weights.fetch.end":        {14},
		"llamacpp.weights.load.start":       {14},
		"llamacpp.weights.load.end":         {186},
		"llamacpp.context.weights.load.end": {131},
		"llamacpp.kv.start":                 {131},
		"llamacpp.kv.end":                   {147},
		"llamacpp.server.listening":         {187},
	}
	if len(p.Markers) != len(want) {
		t.Errorf("pack keeps %d markers at level 4, want %d", len(p.Markers), len(want))
	}
	var gated []string
	for _, m := range p.Markers {
		if got := matching(m.Regex, lines); !slices.Equal(got, want[m.ID]) {
			t.Errorf("%s matches lines %v, want %v", m.ID, got, want[m.ID])
		}
		if m.MinLogLevel != 0 {
			gated = append(gated, m.ID)
			if m.MinLogLevel != 4 || m.Confidence != nil {
				t.Errorf("%s: min_log_level %d, confidence %v; want 4 and tested", m.ID, m.MinLogLevel, m.Confidence)
			}
		}
	}
	if w := []string{"llamacpp.context.weights.load.end", "llamacpp.kv.start", "llamacpp.kv.end"}; !slices.Equal(gated, w) {
		t.Errorf("gated markers %v, want %v", gated, w)
	}
	// The KV buffer size is the kv.end event's mib attribute, which kv-alloc takes.
	re := regexp.MustCompile(p.Markers[slices.IndexFunc(p.Markers, func(m model.PackMarker) bool { return m.ID == "llamacpp.kv.end" })].Regex)
	if m := re.FindStringSubmatch(lines[146]); m == nil || m[re.SubexpIndex("mib")] != "1.69" {
		t.Errorf("kv.end on line 147 reads mib %v, want 1.69", m)
	}
}

// At -lv 4 both facts read a real line: the build and the model.
func TestLlamacppLv4Facts(t *testing.T) {
	lines := capturedLines(t, llamacppLv4Fixture)
	got := map[string]string{}
	for _, f := range atLogLevel(t, llamacppPack(t), lines).Facts {
		re := regexp.MustCompile(f.Regex)
		for _, l := range lines {
			if m := re.FindStringSubmatch(l); m != nil {
				got[f.Key] = m[re.SubexpIndex("v")]
				break
			}
		}
	}
	if want := map[string]string{"engine.version": "11889", "model.ref": "ggml-org/models"}; !maps.Equal(got, want) {
		t.Errorf("facts %v, want %v", got, want)
	}
}

// At -lv 4 the log says the build, so the image's pack is selected by version, with no
// warning and every marker kept.
func TestLlamacppLv4Detection(t *testing.T) {
	lines := capturedLines(t, llamacppLv4Fixture)
	var pod corev1.Pod
	b, err := os.ReadFile(llamacppLv4Fixture + "pod.json")
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(b, &pod); err != nil {
		t.Fatal(err)
	}
	p, warnings, err := Select(pod.Spec.Containers[0].Image, lines...)
	if err != nil || p.Name != "llamacpp/11889" || warnings != nil {
		t.Fatalf("selected %q %v %v, want llamacpp/11889 with no warning", p.Name, warnings, err)
	}
	if len(p.Markers) != len(llamacppPack(t).Markers) {
		t.Errorf("selection kept %d of %d markers at level 4", len(p.Markers), len(llamacppPack(t).Markers))
	}
}
