package packs

import (
	"slices"
	"strings"
	"testing"

	"github.com/AmitwalaH/doriki/internal/model"
)

// gatedPack declares one marker and one fact for every level and one of each only from level 4,
// as llama.cpp prints its build, tensor load and KV cache only at -lv 4 (decision 0010).
func gatedPack() model.Pack {
	return model.Pack{PackVersion: 1, Engine: "a", Name: "a/1", Versions: "1",
		Detect: model.PackDetect{ImageRegex: "engine-a", LogLevelRegex: `verbosity = (?P<level>[0-9]+)`},
		Markers: []model.PackMarker{
			{ID: "a.weights.load.end", Regex: "model loaded"},
			{ID: "a.kv.end", Regex: "KV buffer", MinLogLevel: 4},
		},
		Facts: []model.PackFact{
			{Key: "model.ref", Regex: "loading model '(?P<v>[^']+)'"},
			{Key: "engine.version", Regex: "build (?P<v>[0-9]+)", MinLogLevel: 4},
		},
	}
}

func ids(p model.Pack) []string {
	var out []string
	for _, m := range p.Markers {
		out = append(out, m.ID)
	}
	for _, f := range p.Facts {
		out = append(out, f.Key)
	}
	return out
}

// Selection returns the pack with the markers and facts the log's level does not reach
// removed, so they are neither matched nor expected. A log with no level line counts as the
// default level: gated entries are removed. Level 4 and above keep them.
func TestSelectDropsEntriesAboveTheLogLevel(t *testing.T) {
	generic := model.Pack{PackVersion: 1, Engine: Generic, Name: Generic}
	all := []string{"a.weights.load.end", "a.kv.end", "model.ref", "engine.version"}
	ungated := []string{"a.weights.load.end", "model.ref"}
	for log, want := range map[string][]string{
		"verbosity = 3\nmodel loaded": ungated,
		"verbosity = 4\nmodel loaded": all,
		"verbosity = 5\nmodel loaded": all,
		"model loaded":                ungated,
	} {
		p, _, err := selectFrom([]model.Pack{generic, gatedPack()}, "engine-a", strings.Split(log, "\n")...)
		if err != nil {
			t.Fatalf("%q: %v", log, err)
		}
		if got := ids(p); !slices.Equal(got, want) {
			t.Errorf("%q: pack keeps %v, want %v", log, got, want)
		}
	}

	// The level is read from the first 200 lines, as the version is.
	late := append(make([]string, 200), "verbosity = 4")
	p, _, err := selectFrom([]model.Pack{generic, gatedPack()}, "engine-a", late...)
	if err != nil || !slices.Equal(ids(p), ungated) {
		t.Errorf("level on line 201: pack keeps %v (%v), want %v", ids(p), err, ungated)
	}
}

// LogLevel says what it read, and whether it read anything.
func TestLogLevel(t *testing.T) {
	for log, want := range map[string]Level{
		"x\nverbosity = 3": {N: 3, Known: true},
		"verbosity = 12":   {N: 12, Known: true},
		"no level here":    {},
	} {
		got, err := LogLevel(gatedPack(), strings.Split(log, "\n"))
		if err != nil || got != want {
			t.Errorf("%q: %+v %v, want %+v", log, got, err, want)
		}
	}
	for _, c := range []struct {
		l    Level
		min  int
		want bool
	}{{Level{}, 0, true}, {Level{}, 4, false}, {Level{N: 3, Known: true}, 4, false}, {Level{N: 4, Known: true}, 4, true}} {
		if got := c.l.Reaches(c.min); got != c.want {
			t.Errorf("%+v reaches min_log_level %d = %v, want %v", c.l, c.min, got, c.want)
		}
	}
}

// A gated entry with no way to read the level could never be expected, and a level regex
// without the named group reads nothing: both are pack errors, not silent removals.
func TestLogLevelPackErrors(t *testing.T) {
	noRegex := gatedPack()
	noRegex.Detect.LogLevelRegex = ""
	if _, err := LogLevel(noRegex, nil); err == nil || !strings.Contains(err.Error(), "log_level_regex") {
		t.Errorf("min_log_level without log_level_regex: err = %v", err)
	}
	noGroup := gatedPack()
	noGroup.Detect.LogLevelRegex = `verbosity = [0-9]+`
	if _, err := LogLevel(noGroup, nil); err == nil || !strings.Contains(err.Error(), "level") {
		t.Errorf("log_level_regex without a level group: err = %v", err)
	}
	ungated := model.Pack{Name: "b/1", Markers: []model.PackMarker{{ID: "b.kv.end", Regex: "x"}}}
	if l, err := LogLevel(ungated, []string{"verbosity = 4"}); err != nil || l.Known {
		t.Errorf("a pack with no gates reads no level: %+v %v", l, err)
	}
}
