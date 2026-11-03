package packs

import (
	"path/filepath"
	"testing"

	"github.com/AmitwalaH/doriki/internal/fixture"
)

// Every captured file of every fixture has the sha256 its SOURCE.md records, and SOURCE.md
// lists no file that is not there. Fixtures are never edited by hand (DESIGN.md section 9.2);
// this makes an edit by an editor or a tool fail the gate instead of passing silently
// (DK-REL-ENH-20261026-editor-rewrites). The check is internal/fixture's, which capture's
// tests also run on a fresh bundle.
func TestFixtureChecksums(t *testing.T) {
	sources, err := filepath.Glob("*/testdata/*/SOURCE.md")
	if err != nil || len(sources) == 0 {
		t.Fatalf("no fixtures found (%v)", err)
	}
	for _, source := range sources {
		if err := fixture.Verify(filepath.Dir(source)); err != nil {
			t.Error(err)
		}
	}
}
