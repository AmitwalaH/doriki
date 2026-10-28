package rules

import (
	"cmp"
	"fmt"
	"slices"

	"github.com/AmitwalaH/doriki/internal/model"
)

// highShare is the triggering share at and above which advice is high severity, below it
// medium (DESIGN.md section 6.2). It is the one threshold every rule shares, so the engine
// applies it; each rule's own thresholds are in the const block at the top of its file.
const highShare = 0.40

// evidenceShare is the evidence key every rule fills with its triggering share: the share of
// the total its phase took, or the proxy share its file's comment names. Severity and order
// come from it.
const evidenceShare = "share"

// rule is one advice rule: it reads only the report and returns its advice, or false when it
// does not fire. The engine sets the severity.
type rule func(model.Report) (model.Advice, bool)

// all is every rule doriki runs. The order here does not matter: Evaluate sorts.
var all = []rule{pullDominant, fetchRemoteHub, compileMiss, loadSlowStorage, restartLoop}

// Evaluate runs every rule over r and returns the advice that fired, highest share first,
// each with its severity set. Renderers print it in this order and never sort it.
func Evaluate(r model.Report) []model.Advice {
	return evaluate(r, all)
}

func evaluate(r model.Report, rules []rule) []model.Advice {
	var out []model.Advice
	for _, rl := range rules {
		a, ok := rl(r)
		if !ok {
			continue
		}
		a.Severity = "medium"
		if share(a) >= highShare {
			a.Severity = "high"
		}
		out = append(out, a)
	}
	// Equal shares fall back to the rule name, so the order never depends on the list above.
	slices.SortFunc(out, func(a, b model.Advice) int {
		return cmp.Or(cmp.Compare(share(b), share(a)), cmp.Compare(a.Rule, b.Rule))
	})
	return out
}

func share(a model.Advice) float64 {
	s, _ := a.Evidence[evidenceShare].(float64)
	return s
}

// phaseShare is a phase's seconds and its share of the pending-to-ready total, the share
// DESIGN.md section 6.2's triggers name. ok is false unless the phase is complete with a
// duration and the pod has a total: rules never fire on absent, skipped or open phases, or on a
// pod that has not booted.
func phaseShare(r model.Report, name model.PhaseName) (p model.Phase, share float64, ok bool) {
	total := r.Totals.PendingToReadySeconds
	if total == nil || *total <= 0 {
		return p, 0, false
	}
	for _, p := range r.Phases {
		if p.Name == name && p.State == model.StateComplete && p.Seconds != nil {
			return p, *p.Seconds / *total, true
		}
	}
	return p, 0, false
}

// estimated is a phase whose start or end is below confidence 1.0: a derived start or a
// heuristic marker.
func estimated(p model.Phase) bool {
	return p.Confidence != nil && *p.Confidence < 1
}

// evidence is the numbers every phase rule shows: the share, the phase's seconds and the
// total, and estimated when the phase is.
func evidence(r model.Report, p model.Phase, share float64) map[string]any {
	e := map[string]any{evidenceShare: share, "seconds": *p.Seconds, "total_seconds": *r.Totals.PendingToReadySeconds}
	if estimated(p) {
		e["estimated"] = true
	}
	return e
}

// took is "<phase> took 59% of startup (268 s of 456 s)", or "an estimated 24%".
func took(r model.Report, p model.Phase, share float64) string {
	est := ""
	if estimated(p) {
		est = "an estimated "
	}
	return fmt.Sprintf("%s took %s%.0f%% of startup (%.0f s of %.0f s)", p.Name, est, share*100, *p.Seconds, *r.Totals.PendingToReadySeconds)
}
