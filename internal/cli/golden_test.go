package cli

import (
	"bytes"
	"flag"
	"os"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/sebdah/goldie/v2"
)

// goldenFixtures are the CLI goldens (M1b task item 9): every real fixture, by golden name.
var goldenFixtures = map[string]string{
	"kind-nginx":         "../../packs/generic/testdata/kind-nginx/",
	"kind-nginx-warm":    "../../packs/generic/testdata/kind-nginx-warm/",
	"llamacpp-11889":     "../../packs/llamacpp/testdata/11889/",
	"llamacpp-11889-lv4": "../../packs/llamacpp/testdata/11889-lv4/",
	"vllm-0.32":          "../../packs/vllm/testdata/0.32/",
}

// The whole command's output for each fixture directory (--from-files <dir>, which reads
// node.json when present), in text and JSON, against goldens in testdata/. Text goldens are
// recorded with NO_COLOR=1, so they hold no escape codes.
// Regenerate with -update only after approval (METHOD.md Step 11).
func TestCLIGoldens(t *testing.T) {
	if f := flag.Lookup("clean"); f != nil && f.Value.String() == "true" {
		t.Fatal("-clean would delete every file in testdata; run -update without it")
	}
	t.Setenv("NO_COLOR", "1")
	for name, dir := range goldenFixtures {
		for _, format := range []string{"text", "json"} {
			t.Run(name+"/"+format, func(t *testing.T) {
				out, err := run(t, "--from-files", "-o", format, dir)
				if code, msg := Exit(err); code != 0 {
					t.Fatalf("exit %d %q", code, msg)
				}
				if format == "text" {
					checkTerminalText(t, out)
				}
				goldie.New(t, goldie.WithFixtureDir("testdata"), goldie.WithNameSuffix("."+map[string]string{"text": "txt", "json": "json"}[format])).
					Assert(t, name, []byte(out))
			})
		}
	}
}

// checkTerminalText holds DESIGN.md section 7.5 on real output: no line wider than 100
// columns and no escape codes when colour is off.
func checkTerminalText(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(out, "\x1b") {
		t.Errorf("escape codes with NO_COLOR=1")
	}
	for _, l := range strings.Split(out, "\n") {
		if w := lipgloss.Width(l); w > 100 {
			t.Errorf("%d columns: %q", w, l)
		}
	}
}

// Text is the default output (DESIGN.md section 7.2): no -o gives exactly -o text.
func TestTextIsDefault(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	dir := goldenFixtures["vllm-0.32"]
	files := []string{dir + "pod.json", dir + "events.json", dir + "container.log"}
	def, err1 := run(t, append([]string{"--from-files"}, files...)...)
	text, err2 := run(t, append([]string{"--from-files", "-o", "text"}, files...)...)
	if err1 != nil || err2 != nil || def != text || !strings.HasPrefix(def, "Pod ") {
		t.Errorf("default %q (%v) differs from -o text %q (%v)", def, err1, text, err2)
	}
}

// Colour only for a terminal, and never with NO_COLOR set to anything (no-color.org).
// /dev/null is a character device, the same test the CLI uses for a terminal.
func TestColorFor(t *testing.T) {
	devnull, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip(err)
	}
	defer devnull.Close() //nolint:errcheck // read-only file

	t.Setenv("NO_COLOR", "")
	if !colorFor(devnull) {
		t.Errorf("character device, NO_COLOR empty: want colour")
	}
	if colorFor(&bytes.Buffer{}) {
		t.Errorf("a buffer is not a terminal: want no colour")
	}
	t.Setenv("NO_COLOR", "1")
	if colorFor(devnull) {
		t.Errorf("NO_COLOR=1: want no colour")
	}
}
