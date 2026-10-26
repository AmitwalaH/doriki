package cli

import (
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

const restartFixture = "../../packs/generic/testdata/kind-restart/"

// Acceptance 2 of the M2 task, on the real kind capture: the latest attempt (1) starts at the
// first container's end, so the kubelet's cached re-pull is its image-pull, complete and
// cached; scheduling belongs to the pod (decision 0007); the command exits 0. The previous
// container's log is the optional fourth input.
func TestRestartFixture(t *testing.T) {
	files := []string{restartFixture + "pod.json", restartFixture + "events.json", restartFixture + "container.log", restartFixture + "container-previous.log"}
	for _, n := range []int{3, 4} {
		out, err := run(t, append([]string{"--from-files", "-o", "json"}, files[:n]...)...)
		if code, msg := Exit(err); code != 0 {
			t.Fatalf("%d files: exit %d %q\n%s", n, code, msg, out)
		}
		r := decodeReport(t, out)
		if r["attempt"] != float64(1) || r["attempts_total"] != float64(2) {
			t.Errorf("%d files: attempt %v of %v, want 1 of 2", n, r["attempt"], r["attempts_total"])
		}
		states := map[string]string{}
		var pull map[string]any
		for _, p := range r["phases"].([]any) {
			p := p.(map[string]any)
			states[p["name"].(string)], _ = p["state"].(string)
			if p["name"] == string(model.PhaseImagePull) {
				pull = p
			}
		}
		if states[string(model.PhaseScheduling)] != string(model.StateComplete) {
			t.Errorf("%d files: scheduling %s, want complete", n, states[string(model.PhaseScheduling)])
		}
		attrs, _ := pull["attrs"].(map[string]any)
		if pull["state"] != string(model.StateComplete) || attrs[model.AttrCached] != "true" || pull["start"] != "2026-10-26T11:50:45Z" {
			t.Errorf("%d files: image-pull = %v, want complete, cached, at the re-pull 11:50:45", n, pull)
		}
	}
}
