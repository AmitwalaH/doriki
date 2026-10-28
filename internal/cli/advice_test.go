package cli

import (
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// stubAdvice makes the rules return one known advice for this test and counts the calls.
func stubAdvice(t *testing.T) *int {
	t.Helper()
	saved := advise
	t.Cleanup(func() { advise = saved })
	calls := 0
	advise = func(model.Report) []model.Advice {
		calls++
		return []model.Advice{{Rule: "stub-rule", Severity: "high", Text: "Do the stub thing.", Fix: "set --stub",
			Evidence: map[string]any{"share": 0.5}}}
	}
	return &calls
}

// The rules' advice reaches both outputs: the text Advice block as "N. rule: sentence", and
// the JSON advice array with its fix (DESIGN.md sections 2 and 4.4).
func TestAdviceRendered(t *testing.T) {
	stubAdvice(t)
	t.Setenv("NO_COLOR", "1")
	out, _ := run(t, append([]string{"--from-files"}, fixtureArgs...)...)
	if !strings.Contains(out, "\nAdvice\n  1. stub-rule: Do the stub thing.\n") {
		t.Errorf("text has no advice line:\n%s", out)
	}
	out, _ = run(t, append([]string{"--from-files", "-o", "json"}, fixtureArgs...)...)
	advice, _ := decodeReport(t, out)["advice"].([]any)
	if len(advice) != 1 {
		t.Fatalf("JSON advice = %v, want one entry", advice)
	}
	if a, _ := advice[0].(map[string]any); a["rule"] != "stub-rule" || a["fix"] != "set --stub" || a["severity"] != "high" {
		t.Errorf("JSON advice = %v", a)
	}
}

// --no-advice runs no rule, so neither output carries advice.
func TestNoAdvice(t *testing.T) {
	calls := stubAdvice(t)
	t.Setenv("NO_COLOR", "1")
	out, _ := run(t, append([]string{"--from-files", "--no-advice"}, fixtureArgs...)...)
	if strings.Contains(out, "Advice") || strings.Contains(out, "stub-rule") {
		t.Errorf("--no-advice text still has advice:\n%s", out)
	}
	out, _ = run(t, append([]string{"--from-files", "--no-advice", "-o", "json"}, fixtureArgs...)...)
	if _, ok := decodeReport(t, out)["advice"]; ok {
		t.Errorf("--no-advice JSON still has advice:\n%s", out)
	}
	if *calls != 0 {
		t.Errorf("rules ran %d times with --no-advice", *calls)
	}
}
