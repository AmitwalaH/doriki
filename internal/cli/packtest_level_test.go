package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// gatedPackFile is a made-up pack with one marker and one fact for every level and two markers
// and one fact only from level 4 (decision 0010), none of which any fixture log prints.
func gatedPackFile(t *testing.T) string {
	t.Helper()
	pack := filepath.Join(t.TempDir(), "z.yaml")
	body := `pack_version: 1
engine: z
versions: "1"
detect:
  image_regex: "z"
  log_level_regex: 'verbosity = (?P<level>[0-9]+)'
markers:
  - id: z.server.listening
    regex: ": listening on https?://|Application startup complete"
  - id: z.kv.start
    regex: "this line is in no log"
    min_log_level: 4
  - id: z.kv.end
    regex: "nor this one"
    min_log_level: 4
facts:
  - key: model.ref
    regex: "load_model: loading model '(?P<v>[^']+)'"
    untested: true
  - key: engine.version
    regex: "build (?P<v>[0-9]+)"
    min_log_level: 4
`
	if err := os.WriteFile(pack, []byte(body), 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
		t.Fatal(err)
	}
	return pack
}

// Below their level, gated markers and facts are listed as skipped with the level they need,
// not failed, and the output says the level it read: the real llama.cpp default-level log says
// "verbosity = 3".
func TestPackTestBelowLevel(t *testing.T) {
	out, err := run(t, "pack", "test", gatedPackFile(t), "--log", llamacppFixture+"container.log")
	if code, msg := Exit(err); code != 0 {
		t.Fatalf("exit %d %s\n%s", code, msg, out)
	}
	if !strings.Contains(out, "\nlog level 3; 2 markers and 1 fact not checked\n") {
		t.Errorf("output does not say the level and what it did not check:\n%s", out)
	}
	for _, id := range []string{"z.kv.start", "z.kv.end", "engine.version"} {
		if r := row(t, out, id); !strings.HasPrefix(r, "skipped") || !strings.Contains(r, "needs log level 4") {
			t.Errorf("%s row %q", id, r)
		}
	}
	if r := row(t, out, "z.server.listening"); !strings.HasPrefix(r, "ok") {
		t.Errorf("ungated row %q", r)
	}
	checkTerminalText(t, out)
}

// A log with no level line counts as the default level: the vLLM log prints no verbosity.
func TestPackTestLevelUnknown(t *testing.T) {
	out, err := run(t, "pack", "test", gatedPackFile(t), "--log", vllmFixture+"container.log")
	if code, msg := Exit(err); code != 0 {
		t.Fatalf("exit %d %s\n%s", code, msg, out)
	}
	if !strings.Contains(out, "\nlog level unknown; 2 markers and 1 fact not checked\n") {
		t.Errorf("output does not say the level is unknown:\n%s", out)
	}
	if r := row(t, out, "z.kv.end"); !strings.HasPrefix(r, "skipped") {
		t.Errorf("kv row %q", r)
	}
}

// At or above their level, gated entries are checked like any other: one that matches nothing
// fails. The level line is planted in a copy of the real vLLM log, so only the level differs.
func TestPackTestAtLevel(t *testing.T) {
	b, err := os.ReadFile(vllmFixture + "container.log")
	if err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "container.log")
	if err := os.WriteFile(log, append([]byte("verbosity = 4\n"), b...), 0o600); err != nil { // #nosec G703 -- a file in t.TempDir()
		t.Fatal(err)
	}
	out, err := run(t, "pack", "test", gatedPackFile(t), "--log", log)
	if code, _ := Exit(err); code != 1 {
		t.Errorf("exit %d, want 1: gated entries at their level matched nothing\n%s", code, out)
	}
	if !strings.Contains(out, "\nlog level 4\n") || !strings.Contains(out, "2 tested markers matched no line: z.kv.start, z.kv.end") ||
		!strings.Contains(out, "1 tested fact matched no line: engine.version") {
		t.Errorf("output:\n%s", out)
	}
}
