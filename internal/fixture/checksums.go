// Package fixture checks the "## Checksums" section of a fixture's SOURCE.md: the sha256 of
// every captured file, so a fixture edited by an editor or a tool fails the gate instead of
// passing silently (DESIGN.md section 9.2, DK-REL-ENH-20261026-editor-rewrites).
package fixture

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// Heading starts a Checksums section. A SOURCE.md may hold several, and every one counts.
const Heading = "## Checksums"

// checksumLine is one entry of a Checksums section.
var checksumLine = regexp.MustCompile("^- `([^`]+)`: `sha256:([0-9a-f]{64})`$")

// notCaptures are the files of a fixture directory that are not captured inputs: the notes,
// and the goldens that -update rewrites after review: expected.json, and the report.json a
// fixture made by doriki capture holds.
var notCaptures = []string{"SOURCE.md", "expected.json", "report.json"}

// Verify checks that every captured file in dir has the sha256 its SOURCE.md records, and that
// SOURCE.md lists no file that is not there. The error names every file that fails.
func Verify(dir string) error {
	source := filepath.Join(dir, "SOURCE.md")
	b, err := os.ReadFile(source) // #nosec G304 -- a fixture's SOURCE.md
	if err != nil {
		return err
	}
	recorded := checksums(string(b))
	var errs []error
	if len(recorded) == 0 {
		errs = append(errs, fmt.Errorf("%s: no checksums under a %q heading", source, Heading))
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
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
			errs = append(errs, err)
			continue
		}
		sum := sha256.Sum256(b)
		got := hex.EncodeToString(sum[:])
		switch want, ok := recorded[name]; {
		case !ok:
			errs = append(errs, fmt.Errorf("%s: %s has no checksum in SOURCE.md", dir, name))
		case got != want:
			errs = append(errs, fmt.Errorf("%s: %s has sha256 %s, SOURCE.md records %s: the capture changed", dir, name, got, want))
		}
	}
	for name := range recorded {
		if !slices.Contains(captured, name) {
			errs = append(errs, fmt.Errorf("%s: SOURCE.md lists %s, which is not in the fixture", dir, name))
		}
	}
	return errors.Join(errs...)
}

// checksums reads every Checksums section of a SOURCE.md: a fixture made with doriki capture
// has the hand-written part's, for files the contributor added, and capture's own. A section
// ends at the next "## " heading; a "# " heading does not end it.
func checksums(source string) map[string]string {
	out := map[string]string{}
	in := false
	for _, line := range strings.Split(source, "\n") {
		switch {
		case strings.HasPrefix(line, "## "):
			in = line == Heading
		case in:
			if m := checksumLine.FindStringSubmatch(line); m != nil {
				out[m[1]] = m[2]
			}
		}
	}
	return out
}
