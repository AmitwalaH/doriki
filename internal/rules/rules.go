package rules

import (
	"cmp"
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
var all = []rule{}

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
