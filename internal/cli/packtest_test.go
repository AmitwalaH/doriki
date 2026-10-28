package cli

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

const vllmPackFile = "../../packs/vllm/0.32.yaml"

// row is the output line for one marker or fact entry, found by its id and regex prefix.
func row(t *testing.T, out, id string) string {
	t.Helper()
	for _, l := range strings.Split(out, "\n") {
		if f := strings.Fields(l); len(f) > 1 && (f[1] == id || (len(f) > 2 && f[0]+" "+f[1] == "no match" && f[2] == id)) {
			return l
		}
	}
	t.Fatalf("no row for %s in:\n%s", id, out)
	return ""
}

// On the vLLM fixture's own log every tested marker matches, so it exits 0; each marker is
// listed with the time and text of the line it matched, the untested GPU ones with "no match"
// (M3 task acceptance 5).
func TestPackTestVLLM(t *testing.T) {
	out, err := run(t, "pack", "test", vllmPackFile, "--log", vllmFixture+"container.log")
	if code, msg := Exit(err); code != 0 {
		t.Fatalf("exit %d %s\n%s", code, msg, out)
	}
	if r := row(t, out, "vllm.weights.load.end"); !strings.HasPrefix(r, "ok") || !strings.Contains(r, "09:31:10") || !strings.Contains(r, "Loading weights took 0.49 seconds") {
		t.Errorf("weight-load row %q", r)
	}
	if r := row(t, out, "vllm.compile.start"); !strings.HasPrefix(r, "no match") || !strings.Contains(r, "untested") {
		t.Errorf("untested compile row %q", r)
	}
	if r := row(t, out, "engine.version"); !strings.Contains(r, "0.32.0") {
		t.Errorf("engine.version fact row %q", r)
	}
	if !strings.Contains(out, "runtime timestamps") {
		t.Errorf("output does not say where the times came from:\n%s", out)
	}
	checkTerminalText(t, out)
}

// stripped writes the fixture log without its runtime timestamps, as a log copied from a
// notebook or `docker logs` would be, and returns its path.
func stripped(t *testing.T, log string) string {
	t.Helper()
	b, err := os.ReadFile(log) // #nosec G304 -- a fixture path
	if err != nil {
		t.Fatal(err)
	}
	ts := regexp.MustCompile(`(?m)^\S+Z `)
	path := filepath.Join(t.TempDir(), "container.log")
	if err := os.WriteFile(path, []byte(ts.ReplaceAllString(string(b), "")), 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
		t.Fatal(err)
	}
	return path
}

// A log with no runtime timestamps is dated from the engine's in-line time (vLLM prints
// "10-25 09:31:10", with no year), and the output says the year is a guess from today.
func TestPackTestInlineTimes(t *testing.T) {
	out, err := run(t, "pack", "test", vllmPackFile, "--log", stripped(t, vllmFixture+"container.log"))
	if code, msg := Exit(err); code != 0 {
		t.Fatalf("exit %d %s\n%s", code, msg, out)
	}
	if r := row(t, out, "vllm.weights.load.end"); !strings.Contains(r, "09:31:10") {
		t.Errorf("weight-load row %q, want the in-line time 09:31:10", r)
	}
	if !strings.Contains(out, "in-line") {
		t.Errorf("output does not say the times are the engine's in-line times:\n%s", out)
	}
}

// llama.cpp prints no wall-clock time of its own: without runtime timestamps a matching line
// still counts as matched, and the row says it could not be dated.
func TestPackTestUndatable(t *testing.T) {
	out, err := run(t, "pack", "test", "../../packs/llamacpp/11889.yaml", "--log", stripped(t, llamacppFixture+"container.log"))
	if code, msg := Exit(err); code != 0 {
		t.Fatalf("exit %d %s\n%s", code, msg, out)
	}
	if r := row(t, out, "llamacpp.weights.load.end"); !strings.HasPrefix(r, "ok") || !strings.Contains(r, "no time") {
		t.Errorf("load row %q, want matched with no time", r)
	}
}

// A tested (confidence 1.0) marker that matches nothing exits 1 and is named; an untested
// one that matches is flagged for promotion. A made-up pack against the real vLLM log.
func TestPackTestExit(t *testing.T) {
	pack := filepath.Join(t.TempDir(), "x.yaml")
	body := `pack_version: 1
engine: x
versions: "1"
detect:
  image_regex: "x"
markers:
  - id: x.weights.load.end
    regex: "Loading weights took (?P<seconds>[0-9.]+) seconds"
    confidence: 0.8
  - id: x.graph.end
    regex: "this line is in no log"
`
	if err := os.WriteFile(pack, []byte(body), 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
		t.Fatal(err)
	}
	out, err := run(t, "pack", "test", pack, "--log", vllmFixture+"container.log")
	if code, _ := Exit(err); code != 1 {
		t.Errorf("exit %d, want 1 for a tested marker with no match\n%s", code, out)
	}
	if r := row(t, out, "x.graph.end"); !strings.HasPrefix(r, "no match") {
		t.Errorf("graph row %q", r)
	}
	if r := row(t, out, "x.weights.load.end"); !strings.Contains(r, "promote") {
		t.Errorf("an untested marker that matched is not flagged: %q", r)
	}
	if !strings.Contains(out, "x.graph.end") || !strings.Contains(out, "1 tested marker matched no line") {
		t.Errorf("summary does not name the failure:\n%s", out)
	}
}

// With --pod and --events, times come from the full collector, as in a real report.
func TestPackTestWithPod(t *testing.T) {
	out, err := run(t, "pack", "test", vllmPackFile, "--log", vllmFixture+"container.log",
		"--pod", vllmFixture+"pod.json", "--events", vllmFixture+"events.json")
	if code, msg := Exit(err); code != 0 {
		t.Fatalf("exit %d %s\n%s", code, msg, out)
	}
	if r := row(t, out, "vllm.weights.load.end"); !strings.Contains(r, "09:31:10") {
		t.Errorf("weight-load row %q", r)
	}
	if _, err := run(t, "pack", "test", vllmPackFile, "--log", vllmFixture+"container.log", "--pod", vllmFixture+"pod.json"); err == nil {
		t.Errorf("--pod without --events accepted")
	}
}

// A tested fact that matches nothing fails too (the vllm_test.go rule); an untested one does
// not. A made-up pack against the real vLLM log, whose markers all match.
func TestPackTestFactExit(t *testing.T) {
	pack := filepath.Join(t.TempDir(), "y.yaml")
	body := `pack_version: 1
engine: y
versions: "1"
detect:
  image_regex: "y"
markers:
  - id: y.weights.load.end
    regex: "Loading weights took (?P<seconds>[0-9.]+) seconds"
facts:
  - key: engine.version
    regex: "this line is in no log (?P<v>x)"
  - key: model.loaded_gib
    regex: "nor this one (?P<v>[0-9.]+)"
    untested: true
`
	if err := os.WriteFile(pack, []byte(body), 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
		t.Fatal(err)
	}
	out, err := run(t, "pack", "test", pack, "--log", vllmFixture+"container.log")
	if code, _ := Exit(err); code != 1 {
		t.Errorf("exit %d, want 1 for a tested fact with no match\n%s", code, out)
	}
	if !strings.Contains(out, "1 tested fact matched no line: engine.version") || strings.Contains(out, "matched no line: model.loaded_gib") {
		t.Errorf("summary:\n%s", out)
	}
}
