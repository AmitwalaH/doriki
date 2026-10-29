package packs

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

// checksumLine is one entry of a fixture SOURCE.md's "## Checksums" section.
var checksumLine = regexp.MustCompile("^- `([^`]+)`: `sha256:([0-9a-f]{64})`$")

// notCaptures are the files of a fixture directory that are not captured inputs: the notes,
// and the goldens that -update rewrites after review: expected.json, and the report.json a
// fixture made by doriki capture holds.
var notCaptures = []string{"SOURCE.md", "expected.json", "report.json"}

// Every captured file of every fixture has the sha256 its SOURCE.md records, and SOURCE.md
// lists no file that is not there. Fixtures are never edited by hand (DESIGN.md section 9.2);
// this makes an edit by an editor or a tool fail the gate instead of passing silently
// (DK-REL-ENH-20261026-editor-rewrites).
func TestFixtureChecksums(t *testing.T) {
	sources, err := filepath.Glob("*/testdata/*/SOURCE.md")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no fixtures found (%v)", err)
	}
	for _, source := range sources {
		dir := filepath.Dir(source)
		recorded := checksums(t, source)

		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		var captured []string
		for _, e := range entries {
			if !e.IsDir() && !slices.Contains(notCaptures, e.Name()) {
				captured = append(captured, e.Name())
			}
		}
		for _, name := range captured {
			b, err := os.ReadFile(filepath.Join(dir, name)) // #nosec G304 -- a fixture file
			if err != nil {
				t.Fatal(err)
			}
			sum := sha256.Sum256(b)
			got := hex.EncodeToString(sum[:])
			switch want, ok := recorded[name]; {
			case !ok:
				t.Errorf("%s: %s has no checksum in SOURCE.md", dir, name)
			case got != want:
				t.Errorf("%s: %s has sha256 %s, SOURCE.md records %s: the capture changed", dir, name, got, want)
			}
		}
		for name := range recorded {
			if !slices.Contains(captured, name) {
				t.Errorf("%s: SOURCE.md lists %s, which is not in the fixture", dir, name)
			}
		}
	}
}

// checksums reads the "## Checksums" section of a SOURCE.md.
func checksums(t *testing.T, source string) map[string]string {
	t.Helper()
	b, err := os.ReadFile(source) // #nosec G304 -- a fixture's SOURCE.md
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	in := false
	for _, line := range strings.Split(string(b), "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			in = line == "## Checksums"
		case in:
			if m := checksumLine.FindStringSubmatch(line); m != nil {
				out[m[1]] = m[2]
			}
		}
	}
	if len(out) == 0 {
		t.Errorf("%s: no checksums under a \"## Checksums\" heading", source)
	}
	return out
}
