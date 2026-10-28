package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/AmitwalaH/doriki/internal/collect/enginelog"
	"github.com/AmitwalaH/doriki/internal/collect/files"
	"github.com/AmitwalaH/doriki/internal/manifest"
	"github.com/AmitwalaH/doriki/internal/model"
	"github.com/AmitwalaH/doriki/packs"
)

// ExitPackUntested is pack test's failure: a tested marker (confidence 1.0) or a tested fact
// matched no line, the fixture-first rule packs/vllm_test.go holds the embedded packs to.
// It shares 1 with ExitIncomplete; the two never come from the same command.
const ExitPackUntested = 1

func newPackTestCmd() *cobra.Command {
	var log, pod, events string
	cmd := &cobra.Command{
		Use:   "test <pack.yaml> --log <container.log> [--pod <pod.json> --events <events.json>]",
		Short: "Run a pack against a log without a cluster: every marker and fact with the line it matched",
		Long: "Runs a parser pack against a container log alone, with times from the log's runtime timestamps\n" +
			"or else the engine's own in-line times, and lists every marker and fact with the first line it\n" +
			"matched, or \"no match\". Exits 1 when a tested marker (confidence 1.0) or fact matched nothing. An\n" +
			"untested marker that matches is flagged for promotion (DESIGN.md section 5.4).",
		Args: cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runPackTest(cmd.OutOrStdout(), args[0], log, pod, events)
		},
	}
	cmd.Flags().StringVar(&log, "log", "", "the container log (kubectl logs, --timestamps optional)")
	cmd.Flags().StringVar(&pod, "pod", "", "the pod object (kubectl get pod -o json), with --events, to date lines as a real report does")
	cmd.Flags().StringVar(&events, "events", "", "the pod's events (kubectl get events -o json), with --pod")
	return cmd
}

// packLine is one line of the log, with its time when the pack can date it.
type packLine struct {
	text  string
	at    time.Time
	dated bool
}

func runPackTest(w io.Writer, packPath, logPath, podPath, eventsPath string) error {
	if logPath == "" {
		return errors.New("pack test needs --log <container.log>")
	}
	if (podPath == "") != (eventsPath == "") {
		return errors.New("--pod and --events go together")
	}
	b, err := os.ReadFile(packPath) // #nosec G304 -- the user names the pack on the command line
	if err != nil {
		return err
	}
	pack, err := packs.Parse(b, packName(packPath))
	if err != nil {
		return fmt.Errorf("%s: %w", packPath, err)
	}

	var collected []model.Event
	var anchor string
	if podPath != "" {
		collected, _, err = files.Load(podPath, eventsPath, logPath)
		anchor = "the container start"
	} else {
		collected, err = readLog(logPath)
		// No pod, so no container start: in-line times (no year) take the year nearest today.
		start := model.Event{TS: now().UTC(), Source: model.SourceContainerStatus, Kind: model.KindContainerStarted, Confidence: 1}
		collected = append([]model.Event{start}, collected...)
		anchor = "today"
	}
	if err != nil {
		return err
	}
	// The same redaction a report applies, so pack test never prints a secret either.
	collected = manifest.Sanitize(collected)
	lines, source, err := dateLines(pack, collected, anchor)
	if err != nil {
		return err
	}

	fmt.Fprintf(w, "pack %s against %s: %d lines, times from %s\n\nmarkers\n", pack.Name, filepath.Base(logPath), len(lines), source) //nolint:errcheck // checked at the end
	idWidth := 0
	for _, m := range pack.Markers {
		idWidth = max(idWidth, len(m.ID))
	}
	for _, f := range pack.Facts {
		idWidth = max(idWidth, len(f.Key))
	}
	var failed, failedFacts []string
	for _, m := range pack.Markers {
		re, err := regexp.Compile(m.Regex)
		if err != nil {
			return fmt.Errorf("marker %s: %w", m.ID, err)
		}
		tested := m.Confidence == nil || *m.Confidence >= 1
		conf := "1.0"
		if !tested {
			conf = fmt.Sprintf("%.1f", *m.Confidence)
		}
		first, n := firstMatch(re, lines)
		switch {
		case n == 0 && tested:
			failed = append(failed, m.ID)
			fmt.Fprintf(w, "%s\n", fit(fmt.Sprintf("no match  %-*s  %s  tested", idWidth, m.ID, conf), terminalWidth)) //nolint:errcheck // checked at the end
		case n == 0:
			fmt.Fprintf(w, "%s\n", fit(fmt.Sprintf("no match  %-*s  %s  untested", idWidth, m.ID, conf), terminalWidth)) //nolint:errcheck // checked at the end
		default:
			note := ""
			if !tested {
				note = "untested, matched: promote to tested  "
			}
			fmt.Fprintf(w, "%s\n", fit(fmt.Sprintf("ok        %-*s  %s  %s  %s%s%s", idWidth, m.ID, conf, clock(first), note, more(n), fromMatch(re, first.text)), terminalWidth)) //nolint:errcheck // checked at the end
		}
	}

	fmt.Fprintf(w, "\nfacts\n") //nolint:errcheck // checked at the end
	for _, f := range pack.Facts {
		re, err := regexp.Compile(f.Regex)
		if err != nil {
			return fmt.Errorf("fact %s: %w", f.Key, err)
		}
		label := "tested"
		if f.Untested {
			label = "untested"
		}
		first, n := firstMatch(re, lines)
		if n == 0 {
			if !f.Untested {
				failedFacts = append(failedFacts, f.Key)
			}
			fmt.Fprintf(w, "%s\n", fit(fmt.Sprintf("no match  %-*s  %s", idWidth, f.Key, label), terminalWidth)) //nolint:errcheck // checked at the end
			continue
		}
		v := f.Value
		if v == "" {
			if sub := re.FindStringSubmatch(first.text); sub != nil && re.SubexpIndex("v") >= 0 {
				v = sub[re.SubexpIndex("v")]
			}
		}
		fmt.Fprintf(w, "%s\n", fit(fmt.Sprintf("ok        %-*s  = %s  %s  %s", idWidth, f.Key, v, label, clock(first)), terminalWidth)) //nolint:errcheck // checked at the end
	}

	if len(failed)+len(failedFacts) > 0 {
		fmt.Fprintln(w) //nolint:errcheck // checked below
		for _, f := range []struct {
			noun string
			ids  []string
		}{{"marker", failed}, {"fact", failedFacts}} {
			if len(f.ids) == 0 {
				continue
			}
			noun := f.noun
			if len(f.ids) > 1 {
				noun += "s"
			}
			if _, err := fmt.Fprintf(w, "%d tested %s matched no line: %s\n", len(f.ids), noun, strings.Join(f.ids, ", ")); err != nil {
				return err
			}
		}
		return &ExitError{Code: ExitPackUntested}
	}
	_, err = fmt.Fprintf(w, "\nevery tested marker and fact matched\n")
	return err
}

// terminalWidth is DESIGN.md section 7.5's 100 columns.
const terminalWidth = 100

func readLog(path string) ([]model.Event, error) {
	f, err := os.Open(path) // #nosec G304 -- the user names the log on the command line
	if err != nil {
		return nil, err
	}
	defer f.Close() //nolint:errcheck // read-only file
	return files.ReadLog(f)
}

// packName is "<engine dir>/<file>" as the embedded packs are named, e.g. vllm/0.32.
func packName(path string) string {
	return filepath.Base(filepath.Dir(path)) + "/" + strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
}

// dateLines is every engine log line with the time enginelog.Apply gives it: its runtime
// timestamp, or else the pack's in-line time read in the year nearest anchor. Apply drops a
// line it cannot date; such a line still counts for matching, without a time. It also says
// where the times came from. Apply runs with the pack's timestamp block only, so every line
// comes back as a line.
func dateLines(pack model.Pack, events []model.Event, anchor string) ([]packLine, string, error) {
	if _, _, err := enginelog.Apply(pack, nil); err != nil {
		return nil, "", err // a bad regex or start_from_duration, as a real run would report
	}
	dated, _, err := enginelog.Apply(model.Pack{Name: pack.Name, Timestamp: pack.Timestamp}, events)
	if err != nil {
		return nil, "", err
	}
	isLine := func(e model.Event) bool { return e.Source == model.SourceEngineLog && e.Kind == model.KindLogLine }
	var out []packLine
	runtime, inline := 0, 0
	j := 0
	for _, e := range events {
		if !isLine(e) {
			continue
		}
		l := packLine{text: e.Text()}
		for j < len(dated) && !isLine(dated[j]) {
			j++
		}
		if j < len(dated) && dated[j].Raw == e.Raw {
			l.at, l.dated = dated[j].TS, true
			if e.Attrs[model.AttrUntimed] == "true" {
				inline++
			} else {
				runtime++
			}
			j++
		}
		out = append(out, l)
	}
	source := "nowhere: no line has a runtime timestamp or an in-line time this pack reads"
	switch {
	case runtime > 0 && inline > 0:
		source = fmt.Sprintf("runtime timestamps, and the engine's in-line times on %d lines (year nearest %s)", inline, anchor)
	case runtime > 0:
		source = "runtime timestamps"
	case inline > 0:
		source = "the engine's in-line times (year nearest " + anchor + ")"
	}
	return out, source, nil
}

func firstMatch(re *regexp.Regexp, lines []packLine) (packLine, int) {
	var first packLine
	n := 0
	for _, l := range lines {
		if re.MatchString(l.text) {
			if n == 0 {
				first = l
			}
			n++
		}
	}
	return first, n
}

// fromMatch is the line from where re matched, so the matched words fit in the row even after a
// long logger prefix; "…" marks the part left out.
func fromMatch(re *regexp.Regexp, text string) string {
	if loc := re.FindStringIndex(text); loc != nil && loc[0] > 0 {
		return "…" + text[loc[0]:]
	}
	return text
}

func clock(l packLine) string {
	if !l.dated {
		return "no time "
	}
	return l.at.UTC().Format("15:04:05")
}

func more(n int) string {
	if n <= 1 {
		return ""
	}
	return fmt.Sprintf("(%d lines) ", n)
}

// fit cuts s to width columns, marking the cut.
func fit(s string, width int) string {
	r := []rune(s)
	if len(r) <= width {
		return s
	}
	return string(r[:width-1]) + "…"
}
