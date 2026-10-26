package enginelog

import (
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// A marker read from a previous container's line keeps that line's attempt, so phase puts it
// in that attempt even when its time falls past the boundary (finishedAt has 1 s resolution).
// The derived start keeps it too. A current-container line has no tag, and neither do its
// markers.
func TestMarkersKeepAttempt(t *testing.T) {
	prev := line("2026-10-25T09:31:09Z", downloadLine)
	prev.Attrs = map[string]string{model.AttrRestartCount: "0"}
	out, _ := mustApply(t, testPack, []model.Event{prev, line("2026-10-25T09:31:10Z", loadLine)})

	for _, id := range []string{"vllm.weights.fetch.end", "vllm.weights.fetch.start"} {
		got := markers(out, id)
		if len(got) != 1 || got[0].Attrs[model.AttrRestartCount] != "0" {
			t.Errorf("%s = %+v, want one with restart_count 0", id, got)
		}
	}
	if got := markers(out, "vllm.weights.load.end"); len(got) != 1 || got[0].Attrs[model.AttrRestartCount] != "" {
		t.Errorf("current-container marker = %+v, want no restart_count", got)
	}
}
