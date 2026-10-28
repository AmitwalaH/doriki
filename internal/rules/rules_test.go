package rules

import (
	"slices"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// fires is a rule that always fires with the given share. It sets a severity of its own,
// which the engine must replace: severity is decided once, from the share, for every rule.
func fires(name string, share float64) rule {
	return func(model.Report) (model.Advice, bool) {
		return model.Advice{Rule: name, Severity: "bogus", Text: name + " text", Fix: name + " fix",
			Evidence: map[string]any{evidenceShare: share}}, true
	}
}

func never(model.Report) (model.Advice, bool) { return model.Advice{Rule: "never"}, false }

// The engine keeps what fired, highest share first (ties by rule name, so the order never
// depends on the rule list), and sets every severity from the one cutoff in DESIGN.md
// section 6.2: high at a share of 40% or more, medium below.
func TestEvaluate(t *testing.T) {
	got := evaluate(model.Report{}, []rule{
		fires("low", 0.2), never, fires("b-tie", 0.5), fires("cutoff", 0.40),
		fires("a-tie", 0.5), fires("under", 0.3999),
	})
	type row struct{ rule, severity string }
	var rows []row
	for _, a := range got {
		rows = append(rows, row{a.Rule, a.Severity})
	}
	want := []row{{"a-tie", "high"}, {"b-tie", "high"}, {"cutoff", "high"}, {"under", "medium"}, {"low", "medium"}}
	if !slices.Equal(rows, want) {
		t.Errorf("advice = %v, want %v", rows, want)
	}
}

// No rule fires: no advice at all, so the JSON report has no advice key (omitempty on nil).
func TestEvaluateNothing(t *testing.T) {
	if got := evaluate(model.Report{}, []rule{never}); got != nil {
		t.Errorf("advice = %#v, want nil", got)
	}
	if got := Evaluate(model.Report{}); got != nil {
		t.Errorf("Evaluate on an empty report = %#v, want nil", got)
	}
}
