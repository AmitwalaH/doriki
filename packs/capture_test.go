package packs_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/AmitwalaH/doriki/internal/cli"
	"github.com/AmitwalaH/doriki/internal/model"
	renderjson "github.com/AmitwalaH/doriki/internal/render/json"
)

// replayed is the part of a report a replay must reproduce (DESIGN.md section 7.5).
type replayed struct {
	Phases []model.Phase `json:"phases"`
	Gaps   []model.Gap   `json:"gaps,omitempty"`
	Totals model.Totals  `json:"totals,omitzero"`
}

// A fixture made by doriki capture keeps the report.json capture wrote, and --from-files on
// the directory reproduces its phases, gaps and totals byte for byte: what capture records
// and what a replay reports are the same pipeline. report.json is derived, like expected.json:
// a pack change that moves these phases needs it re-recorded with -update, after approval
// (METHOD.md Step 11), and the replay keeps the capture's generated_at.
func TestCaptureReplays(t *testing.T) {
	reports, err := filepath.Glob("*/testdata/*/report.json")
	if err != nil || len(reports) == 0 {
		t.Fatalf("no capture-made fixtures (%v)", err)
	}
	update := flag.Lookup("update") != nil && flag.Lookup("update").Value.String() == "true"
	for _, path := range reports {
		t.Run(filepath.Dir(path), func(t *testing.T) {
			b, err := os.ReadFile(path) // #nosec G304 -- a fixture file
			if err != nil {
				t.Fatal(err)
			}
			var captured model.Report
			if err := json.Unmarshal(b, &captured); err != nil {
				t.Fatal(err)
			}
			r, err := cli.ReportFromDir(filepath.Dir(path), false, captured.GeneratedAt)
			if err != nil {
				t.Fatal(err)
			}
			if update {
				var out bytes.Buffer
				if err := renderjson.Write(&out, r); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(path, out.Bytes(), 0o644); err != nil { // #nosec G306 G703 -- a committed fixture file
					t.Fatal(err)
				}
				return
			}
			got, err := json.MarshalIndent(replayed{r.Phases, r.Gaps, r.Totals}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			want, err := json.MarshalIndent(replayed{captured.Phases, captured.Gaps, captured.Totals}, "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Errorf("--from-files on the bundle gives\n%s\nreport.json says\n%s", got, want)
			}
		})
	}
}

// phase is the named phase of a report.
func phase(t *testing.T, r model.Report, name model.PhaseName) model.Phase {
	t.Helper()
	for _, p := range r.Phases {
		if p.Name == name {
			return p
		}
	}
	t.Fatalf("no %s phase", name)
	return model.Phase{}
}

func at(t *testing.T, s string) time.Time {
	t.Helper()
	ts, err := time.Parse(time.RFC3339Nano, s)
	if err != nil {
		t.Fatal(err)
	}
	return ts
}

// At -lv 4 weight-load ends where the context is built (line 131), kv-alloc runs from there to
// the KV buffer (line 147) with its size, the manifest has the build, and no warning says the
// version is unknown.
func TestLlamacppLv4Report(t *testing.T) {
	r, err := cli.ReportFromDir("llamacpp/testdata/11889-lv4", false, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	context, kv := at(t, "2026-10-29T12:24:35.946615428Z"), at(t, "2026-10-29T12:24:35.946669180Z")
	if p := phase(t, r, model.PhaseWeightLoad); p.State != model.StateComplete || !p.End.Equal(context) {
		t.Errorf("weight-load %s ending %s, want complete ending at line 131 %s", p.State, p.End, context)
	}
	if p := phase(t, r, model.PhaseKVAlloc); p.State != model.StateComplete || !p.Start.Equal(context) || !p.End.Equal(kv) || p.Attrs["mib"] != "1.69" {
		t.Errorf("kv-alloc %+v, want complete from line 131 to line 147 with mib 1.69", p)
	}
	if r.Manifest.Engine.Version != "11889" {
		t.Errorf("engine version %q, want 11889", r.Manifest.Engine.Version)
	}
	for _, w := range r.Warnings {
		t.Errorf("warning %s: %s", w.Code, w.Text)
	}
}

// At the default level the gated markers are dropped, so weight-load still ends at "model
// loaded" (line 14) and kv-alloc is absent with no phase-unobserved warning: the same report
// as before decision 0010.
func TestLlamacppDefaultLevelUnchanged(t *testing.T) {
	r, err := cli.ReportFromDir("llamacpp/testdata/11889", false, time.Date(2000, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatal(err)
	}
	if p := phase(t, r, model.PhaseWeightLoad); !p.End.Equal(at(t, "2026-10-26T13:21:22.861609284Z")) {
		t.Errorf("weight-load ends %s, want line 14, model loaded", p.End)
	}
	if p := phase(t, r, model.PhaseKVAlloc); p.State != model.StateAbsent {
		t.Errorf("kv-alloc %s, want absent", p.State)
	}
	if len(r.Warnings) != 1 || r.Warnings[0].Code != "pack-version-unknown" {
		t.Errorf("warnings %v, want only pack-version-unknown", r.Warnings)
	}
}
