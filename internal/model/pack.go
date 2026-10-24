package model

// Pack is a parser pack (DESIGN.md section 5.2): what doriki knows about one engine's logs,
// as data. The generic pack fills in only the version and engine.
type Pack struct {
	PackVersion int            `yaml:"pack_version"`
	Engine      string         `yaml:"engine"`
	Versions    string         `yaml:"versions"` // semver range the pack is tested against
	Detect      PackDetect     `yaml:"detect,omitempty"`
	Timestamp   *PackTimestamp `yaml:"timestamp,omitempty"`
	Markers     []PackMarker   `yaml:"markers,omitempty"`
	Facts       []PackFact     `yaml:"facts,omitempty"`
}

// PackDetect says how a pack is chosen for a pod (DESIGN.md section 5.3).
type PackDetect struct {
	ImageRegex string `yaml:"image_regex,omitempty"`
	LogRegex   string `yaml:"log_regex,omitempty"`
}

// PackTimestamp parses the engine's own timestamp from a log line.
type PackTimestamp struct {
	Regex  string `yaml:"regex"`
	Layout string `yaml:"layout"`
}

// PackMarker turns matching log lines into marker events.
type PackMarker struct {
	ID         string            `yaml:"id"`
	Regex      string            `yaml:"regex"`
	Confidence *float32          `yaml:"confidence,omitempty"` // 1.0 when unset
	Attrs      map[string]string `yaml:"attrs,omitempty"`
}

// PackFact is a one-shot manifest fact read from the log.
type PackFact struct {
	Key   string `yaml:"key"`
	Regex string `yaml:"regex"`
}
