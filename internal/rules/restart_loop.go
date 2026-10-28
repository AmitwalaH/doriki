package rules

import (
	"fmt"

	"github.com/AmitwalaH/doriki/internal/model"
)

// restart-loop: the container restarted at least twice before the boot the report describes
// (DESIGN.md section 6.2). The window is the attempts the report observed: restartCount is
// attempts_total - 1, since attempt numbers follow restartCount from 0.
//
// Its share is a proxy (owner decision, M3 plan Q3): the seconds the earlier attempts took (the
// gaps with reason "earlier attempts", decision 0007) over the pending-to-ready total. A
// crash-looping pod may never be Ready; then there is no total and no share, and the advice
// still fires, medium, because the restarts are the finding.
const (
	// minRestarts is the trigger: a restartCount of 2 or more.
	minRestarts = 2
)

func restartLoop(r model.Report) (model.Advice, bool) {
	restarts := r.AttemptsTotal - 1
	if restarts < minRestarts {
		return model.Advice{}, false
	}
	ev := map[string]any{"restarts": restarts}
	if total := r.Totals.PendingToReadySeconds; total != nil && *total > 0 {
		var earlier float64
		for _, g := range r.Gaps {
			if g.Reason == model.GapEarlierAttempts {
				earlier += g.Seconds
			}
		}
		ev[evidenceShare], ev["earlier_attempts_seconds"], ev["total_seconds"] = earlier / *total, earlier, *total
	}

	logs := "kubectl logs --previous"
	if pod := podOf(r); pod != nil && pod.Name != "" {
		logs = "kubectl logs -n " + pod.Namespace + " " + pod.Name + " --previous"
	}
	text := fmt.Sprintf("the container restarted %d times before this boot, and each restart repeats every startup phase; "+
		"find the crash in the previous container's log (%s) before tuning startup.", restarts, logs)
	fix := "the crash in the previous container's log: " + logs
	return model.Advice{Rule: "restart-loop", Text: text, Evidence: ev, Fix: fix}, true
}

func podOf(r model.Report) *model.ManifestPod {
	if r.Manifest == nil {
		return nil
	}
	return r.Manifest.Pod
}
