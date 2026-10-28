package cli

import (
	"fmt"
	"io"
	"os"
	"slices"
	"time"

	"github.com/spf13/cobra"
	"k8s.io/cli-runtime/pkg/genericclioptions"

	"github.com/AmitwalaH/doriki/internal/collect/enginelog"
	"github.com/AmitwalaH/doriki/internal/collect/files"
	"github.com/AmitwalaH/doriki/internal/manifest"
	"github.com/AmitwalaH/doriki/internal/model"
	"github.com/AmitwalaH/doriki/internal/normalize"
	"github.com/AmitwalaH/doriki/internal/phase"
	renderjson "github.com/AmitwalaH/doriki/internal/render/json"
	rendertext "github.com/AmitwalaH/doriki/internal/render/text"
	"github.com/AmitwalaH/doriki/internal/rules"
	"github.com/AmitwalaH/doriki/packs"
)

// now is a variable so tests can pin generated_at. cli is the only place that reads the
// clock; the core takes it as a parameter.
var now = time.Now

type profileFlags struct {
	fromFiles     bool
	output        string
	includeEvents bool
	since         time.Duration
	attempt       int
	allAttempts   bool
	follow        bool
	timeout       time.Duration
	noAdvice      bool
	kube          *genericclioptions.ConfigFlags // --kubeconfig, --context, -n and the rest of kubectl's
}

func addProfileFlags(cmd *cobra.Command, f *profileFlags) {
	cmd.Flags().BoolVar(&f.fromFiles, "from-files", false, "build the report from saved files: pod.json events.json container.log [container-previous.log]")
	cmd.Flags().StringVarP(&f.output, "output", "o", "text", "output format: text (the waterfall) or json (the report)")
	cmd.Flags().BoolVar(&f.includeEvents, "include-events", false, "add every input event to the JSON report")
	cmd.Flags().DurationVar(&f.since, "since", 0, "only use events last seen within this long, e.g. 24h (default: every event the cluster still keeps)")
	cmd.Flags().IntVar(&f.attempt, "attempt", 0, "report this boot attempt (the container's restart count) instead of the latest")
	cmd.Flags().BoolVar(&f.allAttempts, "all-attempts", false, "also report every observed boot attempt")
	cmd.Flags().BoolVar(&f.follow, "follow", false, "watch the pod boot, redrawing the waterfall as phases complete, until it is ready or --timeout")
	cmd.Flags().DurationVar(&f.timeout, "timeout", 15*time.Minute, "how long --follow watches before it reports what it has")
	cmd.Flags().BoolVar(&f.noAdvice, "no-advice", false, "run no advice rules: no Advice block, no advice in the JSON report")
	f.kube = genericclioptions.NewConfigFlags(true)
	f.kube.AddFlags(cmd.Flags())
}

// reportOptions are the flags that shape the report rather than the collection.
type reportOptions struct {
	includeEvents bool
	attempt       *int
	allAttempts   bool
	noAdvice      bool
}

// advise is the rules engine, a variable so a test can see what reaches the renderers before
// any rule fires on a fixture.
var advise = rules.Evaluate

func runProfile(cmd *cobra.Command, args []string, f *profileFlags) error {
	if f.output != "text" && f.output != "json" {
		return fmt.Errorf("unknown output %q; use -o text or -o json", f.output)
	}
	if f.since < 0 {
		return fmt.Errorf("--since must be a positive duration, got %s", f.since)
	}
	opts := reportOptions{includeEvents: f.includeEvents, allAttempts: f.allAttempts, noAdvice: f.noAdvice}
	if cmd.Flags().Changed("attempt") {
		opts.attempt = &f.attempt
	}
	at := now()
	out := cmd.OutOrStdout()
	write := func(w io.Writer, r model.Report) error {
		switch {
		case f.output == "json":
			return renderjson.Write(w, r)
		case opts.allAttempts:
			return writeAttempts(w, r, func(w io.Writer, r model.Report) error {
				return rendertext.Write(w, r, rendertext.Options{Color: colorFor(out)})
			})
		case opts.attempt != nil:
			return writeAttempt(w, r, func(w io.Writer, r model.Report) error {
				return rendertext.Write(w, r, rendertext.Options{Color: colorFor(out)})
			})
		}
		return rendertext.Write(w, r, rendertext.Options{Color: colorFor(out)})
	}
	if f.follow {
		return runFollow(cmd.Context(), f, args, opts, out, write)
	}

	var collected []model.Event
	var collectWarnings []model.Warning
	var err error
	switch {
	case f.fromFiles:
		if len(args) != 3 && len(args) != 4 {
			return fmt.Errorf("--from-files takes 3 files (pod.json events.json container.log) and, after a restart, optionally container-previous.log; got %d", len(args))
		}
		previous := ""
		if len(args) == 4 {
			previous = args[3]
		}
		collected, collectWarnings, err = loadFiles(args[0], args[1], args[2], previous)
	case len(args) == 0:
		return fmt.Errorf("name the pod to profile: %s (or --from-files pod.json events.json container.log)", targetUsage)
	case len(args) > 1:
		return fmt.Errorf("doriki profiles one pod at a time; got %d names", len(args))
	default:
		collected, collectWarnings, err = collectLive(cmd.Context(), f, args[0], at)
	}
	if err != nil {
		return err
	}

	r, pack, err := buildReport(collected, collectWarnings, opts, at)
	if err != nil {
		return err
	}
	if err := write(out, r); err != nil {
		return err
	}
	return exitFor(r, pack)
}

// exitFor is the exit code of a finished report (DESIGN.md section 7.3).
func exitFor(r model.Report, pack model.Pack) error {
	if code := exitCode(r, phase.Expected(markerIDs(pack))); code != 0 {
		return &ExitError{Code: code}
	}
	return nil
}

// ReportFromFiles runs the offline pipeline on saved files (DESIGN.md section 3): collect,
// select a pack by image and log, apply it to the log, normalize, assign phases, build the
// manifest. previousLogPath, when not empty, is the previous container's log. now becomes
// generated_at. Live mode shares every step after collection.
func ReportFromFiles(podPath, eventsPath, logPath, previousLogPath string, includeEvents bool, now time.Time) (model.Report, error) {
	collected, warnings, err := loadFiles(podPath, eventsPath, logPath, previousLogPath)
	if err != nil {
		return model.Report{}, err
	}
	r, _, err := buildReport(collected, warnings, reportOptions{includeEvents: includeEvents}, now)
	return r, err
}

func loadFiles(podPath, eventsPath, logPath, previousLogPath string) ([]model.Event, []model.Warning, error) {
	var previous []string
	if previousLogPath != "" {
		previous = append(previous, previousLogPath)
	}
	return files.Load(podPath, eventsPath, logPath, previous...)
}

// buildReport is every step after collection, the same for files and a live cluster.
func buildReport(collected []model.Event, collectWarnings []model.Warning, opts reportOptions, now time.Time) (model.Report, model.Pack, error) {
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

	r, err := phase.Assign(events, phase.Options{Markers: markerIDs(pack), Pack: pack.Name, NotApplicable: pack.NotApplicable,
		Attempt: opts.attempt, AllAttempts: opts.allAttempts})
	if err != nil {
		return model.Report{}, model.Pack{}, err
	}

	r.GeneratedAt = now.UTC()
	r.DorikiVersion = version
	r.Manifest = manifest.Build(events, pack)
	r.Manifest.Fingerprint = manifest.Fingerprint(r.Manifest)
	r.Warnings = slices.Concat(collectWarnings, packWarnings, logWarnings, normalizeWarnings, r.Warnings)
	// Rules read the finished report and nothing else (DESIGN.md section 6.2), so they run
	// last, before the events are attached.
	if !opts.noAdvice {
		r.Advice = advise(r)
	}
	if opts.includeEvents {
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

// isTerminal reports whether w is a terminal (a character device). A variable so tests can
// pretend.
var isTerminal = func(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	info, err := f.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

// colorFor reports whether text output to w is coloured: only for a terminal (a character
// device), and never when NO_COLOR is set to anything non-empty (DESIGN.md section 7.5,
// no-color.org). cli decides this so the renderer reads no environment.
func colorFor(w io.Writer) bool {
	return os.Getenv("NO_COLOR") == "" && isTerminal(w)
}
