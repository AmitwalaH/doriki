package cli

import (
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/spf13/cobra"

	"github.com/AmitwalaH/doriki/internal/collect/files"
	"github.com/AmitwalaH/doriki/internal/model"
	"github.com/AmitwalaH/doriki/internal/normalize"
	"github.com/AmitwalaH/doriki/internal/phase"
	renderjson "github.com/AmitwalaH/doriki/internal/render/json"
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
	cmd.Flags().BoolVar(&f.fromFiles, "from-files", false, "build the report from saved files: pod.json events.json container.log")
	cmd.Flags().StringVarP(&f.output, "output", "o", "text", "output format: text or json")
	cmd.Flags().BoolVar(&f.includeEvents, "include-events", false, "add every input event to the JSON report")
}

func runProfile(cmd *cobra.Command, args []string, f *profileFlags) error {
	if !f.fromFiles {
		return errors.New("live collection from a cluster is not implemented yet; use --from-files pod.json events.json container.log")
	}
	if len(args) != 3 {
		return fmt.Errorf("--from-files takes 3 files (pod.json events.json container.log), got %d", len(args))
	}
	switch f.output {
	case "json":
	case "text":
		return errors.New("text output is not implemented yet; use -o json")
	default:
		return fmt.Errorf("unknown output %q; use -o json", f.output)
	}

	r, pack, err := reportFromFiles(args[0], args[1], args[2], f.includeEvents, now())
	if err != nil {
		return err
	}
	if err := renderjson.Write(cmd.OutOrStdout(), r); err != nil {
		return err
	}
	if code := exitCode(r, phase.Expected(markerIDs(pack))); code != 0 {
		return &ExitError{Code: code}
	}
	return nil
}

// ReportFromFiles runs the offline pipeline on saved files (DESIGN.md section 3): collect,
// normalize, select a pack by image, assign phases. now becomes generated_at. Live mode will
// share every step after collection.
func ReportFromFiles(podPath, eventsPath, logPath string, includeEvents bool, now time.Time) (model.Report, error) {
	r, _, err := reportFromFiles(podPath, eventsPath, logPath, includeEvents, now)
	return r, err
}

func reportFromFiles(podPath, eventsPath, logPath string, includeEvents bool, now time.Time) (model.Report, model.Pack, error) {
	collected, collectWarnings, err := files.Load(podPath, eventsPath, logPath)
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}
	events, normalizeWarnings := normalize.Normalize(collected)

	pack, packWarnings, err := packs.Select(image(events))
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}
	r, err := phase.Assign(events, phase.Options{Markers: markerIDs(pack)})
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}

	r.GeneratedAt = now.UTC()
	r.DorikiVersion = version
	r.Warnings = slices.Concat(collectWarnings, normalizeWarnings, packWarnings, r.Warnings)
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

func markerIDs(p model.Pack) []string {
	ids := make([]string, len(p.Markers))
	for i, m := range p.Markers {
		ids[i] = m.ID
	}
	return ids
}

// exitCode is DESIGN.md section 7.3 under decision 0003: ExitIncomplete when a phase the
// selected pack can produce (expected, from phase.Expected) is open or absent, else 0.
// Phases the pack cannot produce are listed as absent but do not count.
func exitCode(r model.Report, expected []model.PhaseName) int {
	for _, p := range r.Phases {
		if slices.Contains(expected, p.Name) && (p.State == model.StateOpen || p.State == model.StateAbsent) {
			return ExitIncomplete
		}
	}
	return 0
}
