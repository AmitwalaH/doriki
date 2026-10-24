package model

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"
)

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

// KindContainerStarted is the container-status event for a container start, one per boot
// attempt. Engine-log clock correction anchors to it.
const KindContainerStarted = "ContainerStarted"

// sourcePriority breaks timestamp ties (DESIGN.md section 6.1 step 2). Sources it does not
// list sort after all of these.
var sourcePriority = [...]Source{
	SourceK8sEvent, SourceContainerStatus, SourceCRI, SourceOTelSpan, SourceEngineLog,
}

func sourceRank(s Source) int {
	if i := slices.Index(sourcePriority[:], s); i >= 0 {
		return i
	}
	return len(sourcePriority)
}

// CompareEvents is the single event order: time, then source priority, then every other
// field. Because every field takes part, the order is total and sorting any permutation of
// the same events gives the same slice; it returns 0 only for identical events, which is
// also what dedupe relies on.
func CompareEvents(a, b Event) int {
	return cmp.Or(
		a.TS.Compare(b.TS),
		cmp.Compare(sourceRank(a.Source), sourceRank(b.Source)),
		strings.Compare(string(a.Source), string(b.Source)),
		strings.Compare(a.Kind, b.Kind),
		strings.Compare(a.Marker, b.Marker),
		strings.Compare(a.Raw, b.Raw),
		cmp.Compare(a.Confidence, b.Confidence),
		compareAttrs(a.Attrs, b.Attrs),
	)
}

// compareAttrs orders maps by their sorted key/value pairs.
func compareAttrs(a, b map[string]string) int {
	ka, kb := slices.Sorted(maps.Keys(a)), slices.Sorted(maps.Keys(b))
	for i := range min(len(ka), len(kb)) {
		if c := cmp.Or(strings.Compare(ka[i], kb[i]), strings.Compare(a[ka[i]], b[kb[i]])); c != 0 {
			return c
		}
	}
	return cmp.Compare(len(ka), len(kb))
}
