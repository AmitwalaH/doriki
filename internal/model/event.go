package model

import (
	"cmp"
	"maps"
	"slices"
	"strings"
	"time"
	"unicode/utf8"
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

// Event kinds the phase engine reads (DESIGN.md section 4.2). Collectors emit them; facts
// read from the pod object (creation, container starts, the Ready condition) come from
// SourceContainerStatus, Kubernetes Event objects from SourceK8sEvent.
const (
	KindPodCreated       = "PodCreated" // pod metadata.creationTimestamp
	KindScheduled        = "Scheduled"
	KindPulling          = "Pulling"
	KindPulled           = "Pulled"
	KindContainerStarted = "ContainerStarted" // one per boot attempt; carries AttrRestartCount
	KindPodReady         = "PodReady"         // Ready condition turned True
	KindLogLine          = "log.line"         // any engine log line
	KindLogMarker        = "log.marker"       // a log line a pack matched; Marker holds its id
	KindContainerImage   = "ContainerImage"   // SourceManifest; AttrImage holds the image reference
)

// AttrImage on a KindContainerImage event is the profiled container's image, as written in
// the pod spec. Pack selection reads it.
const AttrImage = "image"

// MaxRawBytes caps Event.Raw (DESIGN.md section 4.1).
const MaxRawBytes = 512

// TruncateRaw cuts s to at most MaxRawBytes without splitting a UTF-8 character.
func TruncateRaw(s string) string {
	if len(s) <= MaxRawBytes {
		return s
	}
	cut := MaxRawBytes
	for cut > 0 && !utf8.RuneStart(s[cut]) {
		cut--
	}
	return s[:cut]
}

// AttrCached is "true" on a KindPulled event whose image was already on the node, and on the
// image-pull phase that event makes (decision 0004).
const AttrCached = "cached"

// AttrRestartCount on a KindContainerStarted event is the container's restartCount at that
// start, which is also its attempt number.
const AttrRestartCount = "restart_count"

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
