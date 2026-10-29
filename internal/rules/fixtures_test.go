package rules_test

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/cli"
	"github.com/AmitwalaH/doriki/internal/model"
	"github.com/AmitwalaH/doriki/internal/rules"
)

// fixture is the report of a real pack fixture through the full offline pipeline, read as a
// capture directory (with node.json when it has one), with no advice yet, so a test sees
// exactly what Evaluate makes of it.
func fixture(t *testing.T, dir string) model.Report {
	t.Helper()
	r, err := cli.ReportFromDir(filepath.Join("../../packs", dir), false, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	r.Advice = nil
	return r
}

// find is the advice of one rule, if it fired.
func find(r model.Report, rule string) (model.Advice, bool) {
	for _, a := range rules.Evaluate(r) {
		if a.Rule == rule {
			return a, true
		}
	}
	return model.Advice{}, false
}

var t0 = time.Date(2026, 10, 25, 9, 0, 0, 0, time.UTC)

// minimal is a synthetic report: a total and the phases a rule reads. Rules read a report,
// not a log, so a hand-made report is a fair input where no real fixture crosses a threshold.
func minimal(total float64, phases ...model.Phase) model.Report {
	return model.Report{ReportVersion: model.ReportVersion, Phases: phases,
		Totals: model.Totals{PendingToReadySeconds: &total}, Manifest: &model.Manifest{ManifestVersion: model.ManifestVersion}}
}

// complete is a complete phase of the given seconds, starting at t0.
func complete(name model.PhaseName, seconds float64) model.Phase {
	return model.Phase{Name: name, Start: t0, End: t0.Add(time.Duration(seconds * float64(time.Second))), Seconds: &seconds,
		State: model.StateComplete}
}
