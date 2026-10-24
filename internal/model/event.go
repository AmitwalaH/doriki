package model

import "time"

// Source says which collector produced an event.
type Source string

const (
	SourceK8sEvent        Source = "k8s-event"
	SourceContainerStatus Source = "container-status"
	SourceCRI             Source = "cri"
	SourceEngineLog       Source = "engine-log"
	SourceOTelSpan        Source = "otel-span"
	SourceManifest        Source = "manifest"
)

// Event is the one normalized type every collector produces (DESIGN.md section 4.1).
type Event struct {
	TS         time.Time         `json:"ts"` // UTC, clock-corrected by normalize
	Source     Source            `json:"source"`
	Kind       string            `json:"kind"`             // e.g. "Scheduled", "Pulling", "log.marker"
	Marker     string            `json:"marker,omitempty"` // parser-pack marker id
	Attrs      map[string]string `json:"attrs,omitempty"`  // raw key/values from the source
	Raw        string            `json:"raw,omitempty"`    // original line or message, at most 512 bytes
	Confidence float32           `json:"confidence"`       // 1.0 for K8s and CRI; packs may set lower
}
