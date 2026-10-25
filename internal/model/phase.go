package model

import (
	"slices"
	"time"
)

// PhaseName is one of the fixed startup phases.
type PhaseName string

// The phase names, in DESIGN.md section 4.2 order. This file is the single encoding of the
// names and their order: it lives in model, not phase, because renderers may import only
// model (DESIGN.md section 8.3).
const (
	PhaseScheduling    PhaseName = "scheduling"
	PhaseNodeProvision PhaseName = "node-provision"
	PhaseImagePull     PhaseName = "image-pull"
	PhaseContainerInit PhaseName = "container-init"
	PhaseEngineInit    PhaseName = "engine-init" // decision 0006
	PhaseWeightFetch   PhaseName = "weight-fetch"
	PhaseWeightLoad    PhaseName = "weight-load"
	PhaseCompile       PhaseName = "compile"
	PhaseGraphCapture  PhaseName = "graph-capture"
	PhaseKVAlloc       PhaseName = "kv-alloc"
	PhaseServerReady   PhaseName = "server-ready"
	PhaseFirstToken    PhaseName = "first-token"
)

var phaseOrder = [...]PhaseName{
	PhaseScheduling, PhaseNodeProvision, PhaseImagePull, PhaseContainerInit, PhaseEngineInit,
	PhaseWeightFetch, PhaseWeightLoad, PhaseCompile, PhaseGraphCapture, PhaseKVAlloc,
	PhaseServerReady, PhaseFirstToken,
}

// Phases returns every phase in the fixed order. It returns a copy so no caller can reorder
// the phases for the others.
func Phases() []PhaseName {
	return slices.Clone(phaseOrder[:])
}

// PhaseState says how much of a phase was observed (DESIGN.md sections 4.2 and 6.1).
type PhaseState string

const (
	StateComplete PhaseState = "complete" // start and end observed
	StateOpen     PhaseState = "open"     // start observed, no end at report time
	StateAbsent   PhaseState = "absent"   // no start observed
	StateInvalid  PhaseState = "invalid"  // end precedes start by more than 1 s; not in totals
	StateSkipped  PhaseState = "skipped"  // the engine said it skips this phase (decision 0005)
)

var phaseStates = [...]PhaseState{StateComplete, StateOpen, StateAbsent, StateInvalid, StateSkipped}

// PhaseStates returns every phase state, for the report schema's enum. A copy, like Phases.
func PhaseStates() []PhaseState {
	return slices.Clone(phaseStates[:])
}

// Phase is one row of the waterfall. Start, End and Seconds are omitted when not observed;
// Seconds is a pointer because a complete phase of 0 s is real and must still be written.
type Phase struct {
	Name    PhaseName         `json:"name"`
	Start   time.Time         `json:"start,omitzero"`
	End     time.Time         `json:"end,omitzero"`
	Seconds *float64          `json:"seconds,omitempty"`
	State   PhaseState        `json:"state"`
	Attrs   map[string]string `json:"attrs,omitempty"`
	Sources []Source          `json:"sources,omitempty"`
	// Confidence is the lowest confidence of the phase's start and end events, written only
	// when below 1.0: a heuristic marker or a derived start (DESIGN.md section 5.2).
	Confidence *float32 `json:"confidence,omitempty"`
}

// Gap is unattributed time between two consecutive complete phases, reported instead of
// being folded into a neighbour.
type Gap struct {
	After   PhaseName `json:"after"`
	Before  PhaseName `json:"before"`
	Seconds float64   `json:"seconds"`
}
