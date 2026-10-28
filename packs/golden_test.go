package packs_test

import (
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sebdah/goldie/v2"

	"github.com/AmitwalaH/doriki/internal/cli"
	"github.com/AmitwalaH/doriki/internal/model"
)

// expected is the part of the report a pack fixture freezes (DESIGN.md section 5.4, owner
// decision Q4): phases, gaps and totals, and the manifest, so a fact that reads the wrong value
// fails here (DK-PACK-ENH-20261026-manifest-in-expected). Version, time, warnings, advice and
// events are left out so wording changes do not force a re-record.
type expected struct {
	Phases   []model.Phase   `json:"phases"`
	Gaps     []model.Gap     `json:"gaps,omitempty"`
	Totals   model.Totals    `json:"totals,omitzero"`
	Manifest *model.Manifest `json:"manifest,omitempty"`
}

// Every fixture under packs/*/testdata/ runs through the full offline pipeline, and the
// result must match its expected.json. Regenerate with -update only after approval
// (METHOD.md Step 11).
func TestFixtures(t *testing.T) {
	// goldie's -clean deletes the whole fixture directory before writing, which here would
	// delete the captured pod.json, events.json and container.log.
	if f := flag.Lookup("clean"); f != nil && f.Value.String() == "true" {
		t.Fatal("-clean (or GOLDIE_CLEAN) would delete fixture captures; run -update without it")
	}

	dirs, err := filepath.Glob("*/testdata/*/pod.json")
	if err != nil || len(dirs) == 0 {
		t.Fatalf("no fixtures found (%v)", err)
	}
	for _, pod := range dirs {
		dir := filepath.Dir(pod)
		t.Run(dir, func(t *testing.T) {
			// A restart fixture also has the previous container's log.
			previous := filepath.Join(dir, "container-previous.log")
			if _, err := os.Stat(previous); err != nil {
				previous = ""
			}
			r, err := cli.ReportFromFiles(pod, filepath.Join(dir, "events.json"), filepath.Join(dir, "container.log"), previous, false,
				time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
			if err != nil {
				t.Fatal(err)
			}
			g := goldie.New(t, goldie.WithFixtureDir(dir), goldie.WithNameSuffix(".json"))
			g.AssertJson(t, "expected", expected{Phases: r.Phases, Gaps: r.Gaps, Totals: r.Totals, Manifest: r.Manifest})
		})
	}
}
