package rules_test

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

const restartLoop = "restart-loop"

// kind-restart is the one real restart: restartCount 1 (attempts_total 2), under the
// trigger of 2, so it does not fire there.
func TestRestartLoopFixture(t *testing.T) {
	r := fixture(t, "generic/testdata/kind-restart")
	if r.AttemptsTotal != 2 {
		t.Fatalf("kind-restart attempts_total = %d, want 2", r.AttemptsTotal)
	}
	if a, ok := find(r, restartLoop); ok {
		t.Errorf("restartCount 1, but %s fired: %s", restartLoop, a.Text)
	}
}

// restarted is a minimal report of the latest of attempts boots: earlier seconds spent by the
// previous containers (the "earlier attempts" gap) of a total, or no total when nil.
func restarted(attempts int, earlier float64, total *float64) model.Report {
	r := minimal(0)
	r.Totals.PendingToReadySeconds = total
	r.Attempt, r.AttemptsTotal = attempts-1, attempts
	r.Gaps = []model.Gap{{After: model.PhaseScheduling, Before: model.PhaseImagePull, Seconds: earlier, Reason: model.GapEarlierAttempts}}
	r.Manifest.Pod = &model.ManifestPod{Namespace: "ns", Name: "vllm-0"}
	return r
}

func f64(v float64) *float64 { return &v }

// Fires from restartCount 2 (attempts_total 3). Its share is the proxy the rule's comment
// names, the earlier attempts' seconds over the total: high from 40%. Minimal reports, since no
// fixture restarts twice.
func TestRestartLoopFires(t *testing.T) {
	for _, c := range []struct {
		attempts       int
		earlier, total float64
		want           string
	}{
		{2, 90, 100, ""}, {3, 60, 100, "high"}, {3, 20, 100, "medium"}, {5, 400, 500, "high"},
	} {
		a, ok := find(restarted(c.attempts, c.earlier, f64(c.total)), restartLoop)
		if got := map[bool]string{true: a.Severity}[ok]; got != c.want {
			t.Errorf("attempts %d, %v of %v s earlier: severity %q, want %q", c.attempts, c.earlier, c.total, got, c.want)
			continue
		}
		if ok && (a.Evidence["restarts"] != c.attempts-1 || a.Evidence["share"] != c.earlier/c.total) {
			t.Errorf("attempts %d: evidence %v", c.attempts, a.Evidence)
		}
	}
}

// A crash-looping pod may never be Ready, so it has no total: the rule still fires, medium,
// with no share it cannot compute.
func TestRestartLoopNoTotal(t *testing.T) {
	a, ok := find(restarted(4, 0, nil), restartLoop)
	if !ok || a.Severity != "medium" {
		t.Fatalf("no total: fired %v, severity %q; want medium", ok, a.Severity)
	}
	if _, has := a.Evidence["share"]; has {
		t.Errorf("no total, but evidence has a share: %v", a.Evidence)
	}
}

// The fix points at the crash: the previous container's log, for this pod.
func TestRestartLoopText(t *testing.T) {
	a, _ := find(restarted(3, 60, f64(100)), restartLoop)
	if !strings.Contains(a.Text, "restarted 2 times") || !strings.Contains(a.Fix, "kubectl logs -n ns vllm-0 --previous") {
		t.Errorf("text %q, fix %q", a.Text, a.Fix)
	}
}
