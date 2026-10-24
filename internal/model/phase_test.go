package model

import (
	"slices"
	"testing"
)

// The fixed order is DESIGN.md section 4.2's table, top to bottom.
func TestPhasesOrder(t *testing.T) {
	want := []PhaseName{
		"scheduling", "node-provision", "image-pull", "container-init", "weight-fetch",
		"weight-load", "compile", "graph-capture", "kv-alloc", "server-ready", "first-token",
	}
	if got := Phases(); !slices.Equal(got, want) {
		t.Errorf("Phases() = %v, want %v", got, want)
	}
}

// Callers get a copy, so no caller can reorder the phases for everyone else.
func TestPhasesReturnsCopy(t *testing.T) {
	first := Phases()
	first[0] = "sentinel"
	if got := Phases()[0]; got != PhaseScheduling {
		t.Errorf("after mutating a returned slice, Phases()[0] = %q, want %q", got, PhaseScheduling)
	}
}
