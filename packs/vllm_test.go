package packs

import (
	"bufio"
	"encoding/json"
	"os"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/manifest"
	"github.com/AmitwalaH/doriki/internal/model"
)

const vllmFixture = "vllm/testdata/0.32/"

// fixtureLines is the vLLM capture's log as the pipeline sees it: the kubectl --timestamps
// prefix removed and each line cut to model.MaxRawBytes, as the collector stores it, so a
// marker past that cut fails here as it would in a real run.
func fixtureLines(t *testing.T) []string {
	t.Helper()
	f, err := os.Open(vllmFixture + "container.log")
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close() //nolint:errcheck // read-only file
	var out []string
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		_, rest, _ := strings.Cut(sc.Text(), " ")
		out = append(out, model.TruncateRaw(rest))
	}
	return out
}

func vllmPack(t *testing.T) model.Pack {
	t.Helper()
	all, err := All()
	if err != nil {
		t.Fatal(err)
	}
	i := slices.IndexFunc(all, func(p model.Pack) bool { return p.Name == "vllm/0.32" })
	if i < 0 {
		t.Fatal("no vllm/0.32 pack")
	}
	return all[i]
}

// The fixture-first rule (M1b task item 4, DESIGN.md section 5.4): a tested marker
// (confidence 1.0) matches at least one real line of the capture; an untested one (below
// 1.0, kept for phases this CPU capture cannot show) matches none, so the label stays true.
// Each fact also reads a real line.
func TestVLLMMarkersAgainstFixture(t *testing.T) {
	p, lines := vllmPack(t), fixtureLines(t)
	count := func(regex string) int {
		re := regexp.MustCompile(regex)
		n := 0
		for _, l := range lines {
			if re.MatchString(l) {
				n++
			}
		}
		return n
	}
	for _, m := range p.Markers {
		n := count(m.Regex)
		tested := m.Confidence == nil || *m.Confidence >= 1
		switch {
		case tested && n == 0:
			t.Errorf("tested marker %s matches no fixture line", m.ID)
		case !tested && n > 0:
			t.Errorf("untested marker %s (confidence %v) matches %d fixture lines; make it tested", m.ID, *m.Confidence, n)
		}
	}
	for _, f := range p.Facts {
		if count(f.Regex) == 0 {
			t.Errorf("fact %s matches no fixture line", f.Key)
		}
		if !slices.Contains(manifest.FactKeys(), f.Key) {
			t.Errorf("fact %s is not a manifest field; known: %v", f.Key, manifest.FactKeys())
		}
	}
}

// Selection on the real capture: the CPU image and upstream's both match, and the version
// line in the first 200 lines picks vllm/0.32.
func TestVLLMSelection(t *testing.T) {
	raw, err := os.ReadFile(vllmFixture + "pod.json")
	if err != nil {
		t.Fatal(err)
	}
	var pod struct {
		Spec struct{ Containers []struct{ Image string } }
	}
	if err := json.Unmarshal(raw, &pod); err != nil {
		t.Fatal(err)
	}
	p, warnings, err := Select(pod.Spec.Containers[0].Image, fixtureLines(t)...)
	if err != nil || p.Name != "vllm/0.32" || warnings != nil {
		t.Errorf("Select = %q %v %v, want vllm/0.32 with no warning", p.Name, warnings, err)
	}
	re := regexp.MustCompile(vllmPack(t).Detect.ImageRegex)
	for _, img := range []string{"vllm/vllm-openai:v0.32.0", "docker.io/vllm/vllm-openai@sha256:00"} {
		if !re.MatchString(img) {
			t.Errorf("image_regex does not match upstream %q", img)
		}
	}
}
