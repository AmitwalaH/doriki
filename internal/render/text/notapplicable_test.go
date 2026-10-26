package text

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// Decision 0008 is a label: phases skipped because they do not apply to the engine get their
// own line, apart from phases the engine said it skipped.
func TestNotApplicableLine(t *testing.T) {
	na := map[string]string{model.AttrNotApplicable: "true"}
	r := model.Report{
		Manifest: &model.Manifest{Engine: &model.ManifestEngine{ParserPack: "x/1"}},
		Phases: []model.Phase{
			complete(model.PhaseScheduling, 0, 1),
			{Name: model.PhaseCompile, State: model.StateSkipped, Attrs: na},
			{Name: model.PhaseGraphCapture, State: model.StateSkipped, Attrs: na},
			{Name: model.PhaseKVAlloc, State: model.StateSkipped, Attrs: map[string]string{"skipped_by": "x.kv.skipped"}},
		},
	}
	out := render(t, r, false)
	for _, want := range []string{
		"skipped: compile, graph-capture (not applicable to this engine)",
		"skipped: kv-alloc (the engine said it skips them)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("no line %q in:\n%s", want, out)
		}
	}
}
