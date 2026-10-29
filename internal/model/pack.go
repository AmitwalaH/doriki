package model

import "strings"

// Pack is a parser pack (DESIGN.md section 5.2): what doriki knows about one engine's logs,
// as data. The generic pack fills in only the version and engine.
type Pack struct {
	// Name is "<engine>/<version>" or "<engine>", from the pack's file path; not in the YAML.
	Name        string         `yaml:"-"`
	PackVersion int            `yaml:"pack_version"`
	Engine      string         `yaml:"engine"`
	Versions    string         `yaml:"versions"` // semver range the pack is tested against
	Detect      PackDetect     `yaml:"detect,omitempty"`
	Timestamp   *PackTimestamp `yaml:"timestamp,omitempty"`
	Markers     []PackMarker   `yaml:"markers,omitempty"`
	Facts       []PackFact     `yaml:"facts,omitempty"`
	// NotApplicable lists phases this engine never has (decision 0008). One the log does not
	// show is reported as skipped, "not applicable to this engine", instead of absent. It is
	// a label: such phases are not expected anyway, since the pack declares no markers for
	// them (decision 0003).
	NotApplicable []PhaseName `yaml:"not_applicable,omitempty"`
}

// PackDetect says how a pack is chosen for a pod (DESIGN.md section 5.3).
type PackDetect struct {
	ImageRegex string `yaml:"image_regex,omitempty"`
	LogRegex   string `yaml:"log_regex,omitempty"`
	// VersionNote is added to the warning when log_regex finds no version, to say why: for
	// example, that the engine prints its version only at a higher log level.
	VersionNote string `yaml:"version_note,omitempty"`
	// LogLevelRegex reads the engine's log level, the named group "level" as a number, from
	// the first matching line in the first 200. Markers and facts with a MinLogLevel the level
	// does not reach are dropped for that log; a log with no level line counts as the
	// engine's default level (decision 0010).
	LogLevelRegex string `yaml:"log_level_regex,omitempty"`
}

// PackTimestamp parses the engine's own timestamp from a log line: Regex has a named group
// "ts" and Layout is its Go time layout, without a year (the year comes from the container
// start). With runtime timestamps (kubectl logs --timestamps) it is only compared with them.
type PackTimestamp struct {
	Regex  string `yaml:"regex"`
	Layout string `yaml:"layout"`
}

// PackMarker turns matching log lines into marker events; named groups become the event's
// attributes. Several markers may share an ID (alternative wordings), and one line may match
// several markers.
type PackMarker struct {
	ID         string            `yaml:"id"`
	Regex      string            `yaml:"regex"`
	Confidence *float32          `yaml:"confidence,omitempty"` // 1.0 when unset
	Attrs      map[string]string `yaml:"attrs,omitempty"`
	// StartFromDuration names a group holding a duration in seconds, on a marker whose ID ends
	// in ".end" for an engine that logs only how long a step took when it finishes. The
	// matching line also yields the ".start" marker, stamped that many seconds earlier, with
	// confidence at most DerivedStartConfidence and the attribute AttrStartDerived.
	StartFromDuration string `yaml:"start_from_duration,omitempty"`
	// MinLogLevel is the log level, above the engine's default, from which the engine prints
	// this line. Below it the marker is neither matched nor expected (decision 0010).
	MinLogLevel int `yaml:"min_log_level,omitempty"`
}

// DerivedStartID is the ".start" marker a StartFromDuration marker also yields: "x.y.end"
// gives "x.y.start". Empty for any other marker.
func (m PackMarker) DerivedStartID() string {
	if base, ok := strings.CutSuffix(m.ID, ".end"); ok && m.StartFromDuration != "" {
		return base + ".start"
	}
	return ""
}

// DerivedStartConfidence caps the confidence of a start derived from a logged duration: the
// time is computed, not observed.
const DerivedStartConfidence float32 = 0.9

// PackFact is a one-shot manifest fact read from the log: the named group "v" of the first
// matching line, or Value when set, for a line that means a value without printing it (a cache
// hit). Key is a manifest field such as engine.version or model.ref; several facts may share
// one. Untested marks a fact written from the engine's source for a line the pack's fixture
// cannot show, as confidence below 1.0 marks an untested marker.
type PackFact struct {
	Key      string `yaml:"key"`
	Regex    string `yaml:"regex"`
	Value    string `yaml:"value,omitempty"`
	Untested bool   `yaml:"untested,omitempty"`
	// MinLogLevel gates a fact as it gates a marker (decision 0010).
	MinLogLevel int `yaml:"min_log_level,omitempty"`
}
