package model

import "time"

// ReportVersion is the report schema version. The report is append-only: fields are added,
// never renamed or removed.
const ReportVersion = 1

// Report is the contract every renderer consumes (DESIGN.md section 4.4). Optional fields
// are omitted when empty and never written as null.
type Report struct {
	ReportVersion int       `json:"report_version"`
	GeneratedAt   time.Time `json:"generated_at,omitzero"`
	DorikiVersion string    `json:"doriki_version,omitempty"`
	Manifest      *Manifest `json:"manifest,omitempty"`

	// The boot attempt the top-level phases describe. Attempt numbers follow the
	// container's restartCount, so the first boot is attempt 0 and is always written.
	Attempt       int `json:"attempt"`
	AttemptsTotal int `json:"attempts_total"`

	Phases   []Phase   `json:"phases,omitempty"`
	Gaps     []Gap     `json:"gaps,omitempty"`
	Totals   Totals    `json:"totals,omitzero"`
	Advice   []Advice  `json:"advice,omitempty"`
	Warnings []Warning `json:"warnings,omitempty"`

	// Every attempt, filled only with --all-attempts.
	Attempts []Attempt `json:"attempts,omitempty"`
	// The input events, filled only with --include-events.
	Events []Event `json:"events,omitempty"`
}

// Attempt is one boot of the container: the first start and each restart.
type Attempt struct {
	Attempt int     `json:"attempt"`
	Phases  []Phase `json:"phases,omitempty"`
	Gaps    []Gap   `json:"gaps,omitempty"`
	Totals  Totals  `json:"totals,omitzero"`
}

// Totals use the wall-clock span, not the sum of phases. A value is nil when its end point
// was not observed; 0 is a real measurement.
type Totals struct {
	PendingToReadySeconds      *float64 `json:"pending_to_ready_seconds,omitempty"`
	PendingToFirstTokenSeconds *float64 `json:"pending_to_first_token_seconds,omitempty"`
}

// Manifest is the deployment manifest (DESIGN.md section 4.3). Its fields land in M3; until
// then a report carries none and the key is omitted.
type Manifest struct{}

// Advice is one rule's finding (DESIGN.md section 6.2). No rules run until M3.
type Advice struct {
	Rule     string         `json:"rule"`
	Severity string         `json:"severity"`
	Text     string         `json:"text"`
	Evidence map[string]any `json:"evidence,omitempty"`
}

// Warning is something the user should know about the inputs, such as clock skew.
type Warning struct {
	Code string `json:"code"`
	Text string `json:"text"`
}
