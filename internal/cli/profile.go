package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/AmitwalaH/doriki/internal/collect/enginelog"
	"github.com/AmitwalaH/doriki/internal/collect/files"
	"github.com/AmitwalaH/doriki/internal/manifest"
	"github.com/AmitwalaH/doriki/internal/model"
	"github.com/AmitwalaH/doriki/internal/normalize"
	"github.com/AmitwalaH/doriki/internal/phase"
	renderjson "github.com/AmitwalaH/doriki/internal/render/json"
	rendertext "github.com/AmitwalaH/doriki/internal/render/text"
	"github.com/AmitwalaH/doriki/packs"
)

// now is a variable so tests can pin generated_at. cli is the only place that reads the
// clock; the core takes it as a parameter.
var now = time.Now

type profileFlags struct {
	fromFiles     bool
	output        string
	includeEvents bool
}

func addProfileFlags(cmd *cobra.Command, f *profileFlags) {
	cmd.Flags().BoolVar(&f.fromFiles, "from-files", false, "build the report from saved files: pod.json events.json container.log [container-previous.log]")
	cmd.Flags().StringVarP(&f.output, "output", "o", "text", "output format: text (the waterfall) or json (the report)")
	cmd.Flags().BoolVar(&f.includeEvents, "include-events", false, "add every input event to the JSON report")
}

func runProfile(cmd *cobra.Command, args []string, f *profileFlags) error {
	if !f.fromFiles {
		return errors.New("live collection from a cluster is not implemented yet; use --from-files pod.json events.json container.log")
	}
	if len(args) != 3 && len(args) != 4 {
		return fmt.Errorf("--from-files takes 3 files (pod.json events.json container.log) and, after a restart, optionally container-previous.log; got %d", len(args))
	}
	if f.output != "text" && f.output != "json" {
		return fmt.Errorf("unknown output %q; use -o text or -o json", f.output)
	}

	previous := ""
	if len(args) == 4 {
		previous = args[3]
	}
	r, pack, err := reportFromFiles(args[0], args[1], args[2], previous, f.includeEvents, now())
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	if f.output == "json" {
		err = renderjson.Write(out, r)
	} else {
		err = rendertext.Write(out, r, rendertext.Options{Color: colorFor(out)})
	}
	if err != nil {
		return err
	}
	if code := exitCode(r, phase.Expected(markerIDs(pack))); code != 0 {
		return &ExitError{Code: code}
	}
	return nil
}

// ReportFromFiles runs the offline pipeline on saved files (DESIGN.md section 3): collect,
// select a pack by image and log, apply it to the log, normalize, assign phases, build the
// manifest. previousLogPath, when not empty, is the previous container's log. now becomes
// generated_at. Live mode will share every step after collection.
func ReportFromFiles(podPath, eventsPath, logPath, previousLogPath string, includeEvents bool, now time.Time) (model.Report, error) {
	r, _, err := reportFromFiles(podPath, eventsPath, logPath, previousLogPath, includeEvents, now)
	return r, err
}

func reportFromFiles(podPath, eventsPath, logPath, previousLogPath string, includeEvents bool, now time.Time) (model.Report, model.Pack, error) {
	var previous []string
	if previousLogPath != "" {
		previous = append(previous, previousLogPath)
	}
	collected, collectWarnings, err := files.Load(podPath, eventsPath, logPath, previous...)
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}
	// Env capture rules apply before anything else reads the events, so no secret reaches
	// a report, --include-events or a warning (DESIGN.md section 10.3).
	collected = manifest.Sanitize(collected)
	pack, packWarnings, err := packs.Select(image(collected), logText(collected)...)
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}
	applied, logWarnings, err := enginelog.Apply(pack, collected)
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}
	events, normalizeWarnings := normalize.Normalize(applied)

	r, err := phase.Assign(events, phase.Options{Markers: markerIDs(pack), Pack: pack.Name})
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}

	r.GeneratedAt = now.UTC()
	r.DorikiVersion = version
	r.Manifest = manifest.Build(events, pack)
	r.Manifest.Fingerprint = manifest.Fingerprint(r.Manifest)
	r.Warnings = slices.Concat(collectWarnings, packWarnings, logWarnings, normalizeWarnings, r.Warnings)
	if includeEvents {
		r.Events = events
	}
	return r, pack, nil
}

func image(events []model.Event) string {
	for _, e := range events {
		if e.Source == model.SourceManifest && e.Kind == model.KindContainerImage {
			return e.Attrs[model.AttrImage]
		}
	}
	return ""
}

// logText is the engine log lines in file order, whole, for pack selection by log_regex.
func logText(events []model.Event) []string {
	var out []string
	for _, e := range events {
		if e.Source == model.SourceEngineLog && e.Kind == model.KindLogLine {
			out = append(out, e.Text())
		}
	}
	return out
}

// markerIDs are the marker ids the pack can produce: each declared id, and the start a
// start_from_duration marker derives.
func markerIDs(p model.Pack) []string {
	var ids []string
	for _, m := range p.Markers {
		ids = append(ids, m.ID)
		if d := m.DerivedStartID(); d != "" {
			ids = append(ids, d)
		}
	}
	return ids
}

// exitCode is DESIGN.md section 7.3 under decisions 0003 and 0005: ExitIncomplete when a
// phase the selected pack can produce (expected, from phase.Expected) is open, or absent
// without the phase engine's phase-unobserved warning, else 0. Phases the pack cannot
// produce, skipped phases, and engine phases still absent once the pod was Ready are listed
// but do not count; which phases are excused is decided in phase, not here.
func exitCode(r model.Report, expected []model.PhaseName) int {
	excused := func(name model.PhaseName) bool {
		return slices.ContainsFunc(r.Warnings, func(w model.Warning) bool {
			return w.Code == model.WarnPhaseUnobserved && w.Phase == name
		})
	}
	for _, p := range r.Phases {
		if !slices.Contains(expected, p.Name) {
			continue
		}
		if p.State == model.StateOpen || p.State == model.StateAbsent && !excused(p.Name) {
			return ExitIncomplete
		}
	}
	return 0
}

// colorFor reports whether text output to w is coloured: only for a terminal (a character
// device), and never when NO_COLOR is set to anything non-empty (DESIGN.md section 7.5,
// no-color.org). cli decides this so the renderer reads no environment.
func colorFor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}
